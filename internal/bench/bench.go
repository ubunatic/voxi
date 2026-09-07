// Package bench measures Whisper transcription speed (RTF, real-time
// factor) per model and per compute backend (CPU vs GPU), driving voxtype
// the same way eager mode's whisper engine does. It is voxtype/whisper-only:
// since issue 074 the model registry can also carry non-whisper engines
// (e.g. cohere-transcribe), which voxtype cannot run, so a default run
// benches whisper-engine models only and an explicitly named non-whisper
// model is reported as skipped rather than mis-invoked (see issue 077). GPU
// is forced off by setting
// GGML_VK_VISIBLE_DEVICES="" — confirmed against whisper.cpp/ggml-vulkan's
// device-enumeration fallback: with no visible devices it logs
// "whisper_backend_init_gpu: no GPU found" and runs on CPU. voxtype does
// not expose a --cpu/--gpu flag, and its `setup gpu --enable/--disable`
// only works for symlink-based binary variants, which this env does not
// have installed, so the env var is the only reliable per-run toggle.
//
// By default the benchmark audio is a fixed reference clip (whisper.cpp's
// own JFK sample) downloaded on demand into the user cache directory — it
// is never committed to the repo. This keeps results comparable across
// machines and runs; pass Record or WavFile to bench live/local audio
// instead.
package bench

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"ubunatic.com/voxi/internal/asr"
	"ubunatic.com/voxi/internal/audio"
	"ubunatic.com/voxi/internal/deps"
	spec "ubunatic.com/voxi/spec"
)

// Options configures a CPU vs GPU model benchmark run.
type Options struct {
	Models       []string // model names to bench; empty = every model in spec/models.yaml
	Backends     []string // "cpu", "gpu"; empty = auto-detect via DefaultOptions
	WavFile      string   // reuse an existing 16kHz mono WAV instead of the reference clip
	Record       bool     // record DurationSecs of live mic audio instead of the reference clip
	DurationSecs int      // seconds of mic audio to record when Record is set
	Threads      int      // --threads passed to voxtype
}

// referenceClipURL points at whisper.cpp's own canonical example clip (JFK's
// "ask not what your country can do for you" excerpt, a US federal
// government work and so public domain), pinned to a specific commit so the
// content — and its checksum — never changes under us.
const referenceClipURL = "https://raw.githubusercontent.com/ggml-org/whisper.cpp/b0a11594aec50892a02cd8d129eee2dfe93a8bb8/samples/jfk.wav"

// referenceClipSHA256 is the expected checksum of the downloaded clip;
// downloads that don't match are rejected rather than used.
const referenceClipSHA256 = "59dfb9a4acb36fe2a2affc14bacbee2920ff435cb13cc314a08c13f66ba7860e"

// referenceClipMaxBytes bounds the download; the real clip is ~350KB.
const referenceClipMaxBytes = 32 << 20

// DefaultOptions returns every configured model, gpu+cpu if a GPU render
// node is present (else cpu only), benching against the downloaded
// reference clip (see referenceClipURL) rather than a live recording.
func DefaultOptions() Options {
	backends := []string{"cpu"}
	if _, err := os.Stat("/dev/dri/renderD128"); err == nil {
		backends = []string{"gpu", "cpu"}
	}
	return Options{DurationSecs: 8, Backends: backends, Threads: 6}
}

// Result is one model x backend transcription pass.
type Result struct {
	Model           string  `json:"model"`
	Backend         string  `json:"backend"`          // requested: "cpu" or "gpu"
	DetectedBackend string  `json:"detected_backend"` // what voxtype actually reported using
	AudioSecs       float64 `json:"audio_secs"`
	TranscribeSecs  float64 `json:"transcribe_secs"`
	RTF             float64 `json:"rtf"`
	Speedup         float64 `json:"speedup"`
	Text            string  `json:"text"`
	Error           string  `json:"error,omitempty"`
}

// Report is the full output of a bench run.
type Report struct {
	GeneratedAt time.Time `json:"generated_at"`
	AudioSecs   float64   `json:"audio_secs"`
	AudioSource string    `json:"audio_source"` // "recorded" or the WAV path used
	Results     []Result  `json:"results"`
}

var backendLineRe = regexp.MustCompile(`whisper_backend_init_gpu: (using (\S+) backend|no GPU found)`)

func detectBackend(output string) string {
	m := backendLineRe.FindStringSubmatch(output)
	switch {
	case m == nil:
		return "unknown"
	case m[2] != "":
		return "gpu:" + m[2]
	default:
		return "cpu"
	}
}

// Run transcribes the same audio through every requested model on every
// requested backend and records timing. d.Stdout receives progress notes
// (e.g. the "speak now" recording prompt); per-result output is the
// caller's responsibility.
func Run(ctx context.Context, d deps.Dependencies, opts Options) (*Report, error) {
	voxtypePath, err := d.LookPath("voxtype")
	if err != nil {
		return nil, fmt.Errorf("voxtype not found on PATH: %w", err)
	}

	modelSpec, err := spec.LoadModels()
	if err != nil {
		return nil, fmt.Errorf("load model spec: %w", err)
	}
	models := opts.Models
	if len(models) == 0 {
		// bench drives voxtype exclusively (see the package doc comment), so
		// an auto-selected default run must stick to whisper-engine models;
		// since issue 074 the registry can carry non-whisper entries (e.g.
		// cohere-transcribe), which voxtype cannot run. A model explicitly
		// named via opts.Models still goes through -- it is caught and
		// reported per-result below instead of silently mis-invoked. See
		// issue 077.
		for _, name := range modelSpec.Names() {
			if modelSpec.IsWhisperEngine(name) {
				models = append(models, name)
			}
		}
		sort.Strings(models)
	}
	backends := opts.Backends
	if len(backends) == 0 {
		backends = DefaultOptions().Backends
	}
	threads := opts.Threads
	if threads <= 0 {
		threads = 6
	}

	wavPath := opts.WavFile
	audioSource := wavPath
	switch {
	case wavPath != "":
		// use the caller-supplied file as-is
	case opts.Record:
		duration := opts.DurationSecs
		if duration <= 0 {
			duration = 8
		}
		tmpDir, err := os.MkdirTemp("", "voxi-bench-*")
		if err != nil {
			return nil, fmt.Errorf("create temp dir: %w", err)
		}
		defer os.RemoveAll(tmpDir)
		wavPath = filepath.Join(tmpDir, "bench.wav")
		fmt.Fprintf(d.Stdout, "Recording %ds of audio for benchmark — speak now...\n", duration)
		if err := recordFixedDuration(ctx, d, wavPath, duration); err != nil {
			return nil, fmt.Errorf("record benchmark audio: %w", err)
		}
		audioSource = "recorded"
	default:
		clipPath, err := ensureReferenceClip(ctx, d)
		if err != nil {
			return nil, fmt.Errorf("fetch reference clip: %w", err)
		}
		wavPath = clipPath
		audioSource = referenceClipURL
	}

	audioSecs, err := wavDurationSecs(wavPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", wavPath, err)
	}
	if audioSecs <= 0 {
		return nil, fmt.Errorf("%s contains no audio", wavPath)
	}

	report := &Report{GeneratedAt: time.Now(), AudioSecs: audioSecs, AudioSource: audioSource}
	for _, backend := range backends {
		for _, model := range models {
			if !modelSpec.IsWhisperEngine(model) {
				report.Results = append(report.Results, Result{
					Model: model, Backend: backend, AudioSecs: audioSecs,
					Error: fmt.Sprintf("skipped: model %q uses a non-whisper engine; bench only drives voxtype (see spec/models.yaml)", model),
				})
				continue
			}
			if backend == "cpu" && !modelSpec.AllowsCPU(model) {
				report.Results = append(report.Results, Result{
					Model: model, Backend: backend, AudioSecs: audioSecs,
					Error: "skipped: model requires_gpu (see spec/models.yaml)",
				})
				continue
			}
			report.Results = append(report.Results, runOne(ctx, voxtypePath, model, backend, threads, wavPath, audioSecs, modelSpec.StopWords(model)))
		}
	}
	return report, nil
}

func runOne(ctx context.Context, voxtypePath, model, backend string, threads int, wavPath string, audioSecs float64, stopWords []string) Result {
	result := Result{Model: model, Backend: backend, AudioSecs: audioSecs}

	cmdArgs := []string{"--model", model, "--threads", fmt.Sprintf("%d", threads), "-v", "transcribe", wavPath}
	cmd := exec.CommandContext(ctx, voxtypePath, cmdArgs...)
	cmd.Env = os.Environ()
	if backend == "cpu" {
		// Empties the Vulkan device list ggml enumerates, so whisper.cpp
		// falls back to CPU. See package doc comment.
		cmd.Env = append(cmd.Env, "GGML_VK_VISIBLE_DEVICES=")
	}
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	start := time.Now()
	err := cmd.Run()
	result.TranscribeSecs = time.Since(start).Seconds()
	output := outBuf.String()
	result.DetectedBackend = detectBackend(output)

	if err != nil {
		result.Error = firstErrorLine(output, err)
		return result
	}
	result.Text = asr.CleanWhisperTranscript(output, stopWords)
	result.RTF = result.TranscribeSecs / audioSecs
	if result.RTF > 0 {
		result.Speedup = 1.0 / result.RTF
	}
	return result
}

func firstErrorLine(output string, fallback error) string {
	sc := bufio.NewScanner(strings.NewReader(output))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "Error:") {
			return line
		}
	}
	return fallback.Error()
}

// ensureReferenceClip returns the local path to whisper.cpp's canonical
// JFK example clip, downloading it into the user cache directory if it
// isn't already there. It is never written into the repo or committed —
// only cached under os.UserCacheDir().
func ensureReferenceClip(ctx context.Context, d deps.Dependencies) (string, error) {
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache dir: %w", err)
	}
	dir := filepath.Join(cacheRoot, "voxi", "bench")
	path := filepath.Join(dir, "jfk-reference.wav")
	return fetchAndCacheClip(ctx, d, referenceClipURL, referenceClipSHA256, path)
}

// fetchAndCacheClip returns path if it already holds content matching
// wantSHA256, otherwise downloads url and installs it at path. A checksum
// mismatch (corrupt cache entry or tampered/unexpected download) is an
// error, never silently accepted.
func fetchAndCacheClip(ctx context.Context, d deps.Dependencies, url, wantSHA256, path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil && sha256Hex(data) == wantSHA256 {
		return path, nil
	}

	fmt.Fprintf(d.Stdout, "Downloading reference clip: %s\n", url)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: unexpected status %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, referenceClipMaxBytes+1))
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}
	if len(data) > referenceClipMaxBytes {
		return "", fmt.Errorf("downloaded clip exceeds %d bytes; refusing to cache it", referenceClipMaxBytes)
	}
	if got := sha256Hex(data); got != wantSHA256 {
		return "", fmt.Errorf("checksum mismatch for reference clip: got %s, want %s", got, wantSHA256)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create cache dir %s: %w", dir, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return "", fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", fmt.Errorf("install %s: %w", path, err)
	}
	return path, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func recordFixedDuration(ctx context.Context, d deps.Dependencies, wavPath string, durationSecs int) error {
	const sampleRate = 16000

	recCmdName := ""
	var recArgs []string
	if _, err := d.LookPath("pw-record"); err == nil {
		recCmdName = "pw-record"
		recArgs = []string{"--rate", "16000", "--channels", "1", "--format", "s16", "-"}
	} else if _, err := d.LookPath("arecord"); err == nil {
		recCmdName = "arecord"
		recArgs = []string{"-r", "16000", "-c", "1", "-f", "S16_LE", "-t", "raw", "-q", "-"}
	} else {
		return fmt.Errorf("neither pw-record nor arecord found on PATH")
	}

	recCtx, cancel := context.WithTimeout(ctx, time.Duration(durationSecs)*time.Second)
	defer cancel()

	recCmd := exec.CommandContext(recCtx, recCmdName, recArgs...)
	audioOut, err := recCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	recCmd.Stderr = io.Discard
	if err := recCmd.Start(); err != nil {
		return fmt.Errorf("start audio capture: %w", err)
	}

	pcm, readErr := io.ReadAll(audioOut)
	_ = recCmd.Wait()
	if readErr != nil && len(pcm) == 0 {
		return fmt.Errorf("read audio: %w", readErr)
	}
	if len(pcm) == 0 {
		return fmt.Errorf("no audio captured")
	}
	return audio.WriteWAVAudio(wavPath, pcm, sampleRate)
}

// wavDurationSecs computes audio duration from a WAV file's fmt/data
// chunks, without assuming a fixed header size (unlike our own
// audio.WriteWAVAudio output, a caller-supplied --file may carry extra
// chunks).
func wavDurationSecs(path string) (float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return 0, fmt.Errorf("not a RIFF/WAVE file")
	}

	var sampleRate, dataSize uint32
	var channels, bitsPerSample uint16
	pos := 12
	for pos+8 <= len(data) {
		chunkID := string(data[pos : pos+4])
		chunkSize := binary.LittleEndian.Uint32(data[pos+4 : pos+8])
		body := pos + 8
		switch chunkID {
		case "fmt ":
			if body+16 > len(data) {
				return 0, fmt.Errorf("truncated fmt chunk")
			}
			channels = binary.LittleEndian.Uint16(data[body+2 : body+4])
			sampleRate = binary.LittleEndian.Uint32(data[body+4 : body+8])
			bitsPerSample = binary.LittleEndian.Uint16(data[body+14 : body+16])
		case "data":
			dataSize = chunkSize
		}
		pos = body + int(chunkSize)
		if chunkSize%2 == 1 { // chunks are word-aligned
			pos++
		}
	}
	if sampleRate == 0 || channels == 0 || bitsPerSample == 0 {
		return 0, fmt.Errorf("incomplete fmt chunk")
	}
	bytesPerSec := float64(sampleRate) * float64(channels) * float64(bitsPerSample) / 8
	return float64(dataSize) / bytesPerSec, nil
}
