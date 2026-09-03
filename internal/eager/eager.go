package eager

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"ubunatic.com/voxi/internal/asr"
	"ubunatic.com/voxi/internal/audio"
	"ubunatic.com/voxi/internal/chunks"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/feedback"
	"ubunatic.com/voxi/internal/history"
	"ubunatic.com/voxi/internal/speechcontext"
	"ubunatic.com/voxi/internal/typing"
	spec "ubunatic.com/voxi/spec"
)

// EagerOptions holds configuration parameters for Continuous Eager Sentence Streaming dictation.
type EagerOptions struct {
	ThresholdRMS  int
	SilenceMs     int
	PreRollMs     int
	MinSpeechMs   int
	MaxWindowMs   int
	TypeOutput    bool
	RecordHistory bool
	Daemon        bool
	Model         string
	SpeechContext bool
	Vocabulary    []string
}

// DefaultEagerOptions returns standard defaults for eager sentence streaming dictation.
// The default model name comes from spec/models.yaml, not a hardcoded value.
func DefaultEagerOptions() EagerOptions {
	s, err := spec.LoadModels()
	if err != nil {
		// spec/models.yaml is embedded at build time and schema-checked by
		// spec's own tests; a load failure here means a broken build.
		panic(err)
	}
	return EagerOptions{
		ThresholdRMS:  150,
		SilenceMs:     800,
		PreRollMs:     500,
		MinSpeechMs:   200,
		MaxWindowMs:   8000,
		TypeOutput:    true,
		RecordHistory: true,
		Daemon:        false,
		Model:         s.DefaultModel,
		SpeechContext: true,
	}
}

// transcribeTimeout bounds a single `voxtype transcribe` subprocess so a
// stalled model or backend can never linger as a zombie process after
// recording stops; well above realistic transcription time even for the
// longest MaxWindowMs utterance on a slow CPU backend.
const transcribeTimeout = 30 * time.Second

// UtteranceStat records timing, speed, and text for one transcribed phrase.
type UtteranceStat struct {
	Index          int       `json:"index"`
	AudioSecs      float64   `json:"audio_secs"`
	TranscribeSecs float64   `json:"transcribe_secs"`
	RTF            float64   `json:"rtf"`
	Text           string    `json:"text"`
	Timestamp      time.Time `json:"timestamp"`
}

// EagerMetrics holds aggregated throughput and recent sentence history.
type EagerMetrics struct {
	TotalChunks         int             `json:"total_chunks"`
	TotalAudioSecs      float64         `json:"total_audio_secs"`
	TotalTranscribeSecs float64         `json:"total_transcribe_secs"`
	AvgRTF              float64         `json:"avg_rtf"`
	LastUtterance       *UtteranceStat  `json:"last_utterance,omitempty"`
	Recent              []UtteranceStat `json:"recent"`
}

var (
	eagerMetricsLock sync.Mutex
	eagerMetrics     EagerMetrics
)

func recordEagerStat(stat UtteranceStat) {
	eagerMetricsLock.Lock()
	defer eagerMetricsLock.Unlock()

	eagerMetrics.TotalChunks++
	eagerMetrics.TotalAudioSecs += stat.AudioSecs
	eagerMetrics.TotalTranscribeSecs += stat.TranscribeSecs
	if eagerMetrics.TotalAudioSecs > 0 {
		eagerMetrics.AvgRTF = eagerMetrics.TotalTranscribeSecs / eagerMetrics.TotalAudioSecs
	}
	eagerMetrics.LastUtterance = &stat
	eagerMetrics.Recent = append([]UtteranceStat{stat}, eagerMetrics.Recent...)
	if len(eagerMetrics.Recent) > 10 {
		eagerMetrics.Recent = eagerMetrics.Recent[:10]
	}

	data, err := json.MarshalIndent(eagerMetrics, "", "  ")
	if err == nil {
		runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
		if runtimeDir == "" {
			runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
		}
		voxiDir := filepath.Join(runtimeDir, "voxi")
		_ = os.MkdirAll(voxiDir, 0755)
		_ = os.WriteFile(filepath.Join(voxiDir, "eager-metrics.json"), data, 0644)
	}
}

func writeVoxtypeState(state string) {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	stateDir := filepath.Join(runtimeDir, "voxtype")
	_ = os.MkdirAll(stateDir, 0755)
	stateFile := filepath.Join(stateDir, "state")
	_ = os.WriteFile(stateFile, []byte(state+"\n"), 0644)

	// Write to voxi state
	voxiDir := filepath.Join(runtimeDir, "voxi")
	_ = os.MkdirAll(voxiDir, 0755)
	_ = os.WriteFile(filepath.Join(voxiDir, "voice-state"), []byte(state+"\n"), 0644)
}

// gpuAvailable reports whether a GPU render node is present for Vulkan
// acceleration (see internal/monitor's equivalent check).
func gpuAvailable() bool {
	_, err := os.Stat("/dev/dri/renderD128")
	return err == nil
}

// RunEagerDictation orchestrates continuous audio capture, rolling phrase segmentation,
// Whisper transcription, instant text typing via dotool, and history appending.
func RunEagerDictation(ctx context.Context, d deps.Dependencies, opts EagerOptions) error {
	if opts.Daemon {
		return runEagerDaemon(ctx, d, opts)
	}

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

	voxtypePath, err := d.LookPath("voxtype")
	if err != nil {
		return fmt.Errorf("voxtype not found on PATH: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "voxi-eager-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	fmt.Fprintln(d.Stdout, "── Continuous Eager Sentence Streaming Dictation ───────────────")
	fmt.Fprintf(d.Stdout, "  Audio Source:          %s (16kHz mono S16_LE)\n", recCmdName)
	fmt.Fprintf(d.Stdout, "  RMS Threshold:         %d\n", opts.ThresholdRMS)
	fmt.Fprintf(d.Stdout, "  Silence Cutoff:        %d ms\n", opts.SilenceMs)
	fmt.Fprintf(d.Stdout, "  Pre-roll Buffer:       %d ms (preserves initial phonemes)\n", opts.PreRollMs)
	fmt.Fprintf(d.Stdout, "  Type into window:      %t\n", opts.TypeOutput)
	fmt.Fprintf(d.Stdout, "  Record history:        %t\n", opts.RecordHistory)
	fmt.Fprintln(d.Stdout, "────────────────────────────────────────────────────────────────")
	fmt.Fprintln(d.Stdout, "Instructions:")
	fmt.Fprintln(d.Stdout, "  Speak naturally with conversational pauses. Sentences will transcribe")
	fmt.Fprintln(d.Stdout, "  and type immediately upon each pause with zero dropped words.")
	fmt.Fprintln(d.Stdout, "  Press Ctrl-C (or cancel context) to stop dictation.")
	fmt.Fprintln(d.Stdout, "")

	return runEagerCaptureSession(ctx, d, opts, tmpDir, voxtypePath, recCmdName, recArgs, false)
}

func runEagerCaptureSession(ctx context.Context, d deps.Dependencies, opts EagerOptions, tmpDir string, voxtypePath string, recCmdName string, recArgs []string, isDaemon bool) error {
	const (
		sampleRate  = 16000
		bytesPerSec = sampleRate * 2
		frameMs     = 20
		frameBytes  = (sampleRate * frameMs / 1000) * 2 // 640 bytes
	)

	segOpts := audio.SegmenterOptions{
		ThresholdRMS: opts.ThresholdRMS,
		SilenceMs:    opts.SilenceMs,
		PreRollMs:    opts.PreRollMs,
		MinSpeechMs:  opts.MinSpeechMs,
		MaxWindowMs:  opts.MaxWindowMs,
	}
	segmenter := audio.NewAudioSegmenter(segOpts)
	historyPath := history.HistoryPath(d.Getenv("HOME"))

	type TranscribeJob struct {
		Index           int
		Audio           []byte
		Duration        float64
		Plausible       bool
		RejectionReason string
	}

	modelSpec, err := spec.LoadModels()
	if err != nil {
		return fmt.Errorf("eager: load model spec: %w", err)
	}
	modelName := opts.Model
	if modelName == "" {
		modelName = modelSpec.DefaultModel
	}
	resolvedModel, usedFallback, err := modelSpec.ResolveModel(modelName, gpuAvailable())
	if err != nil {
		return fmt.Errorf("eager: %w", err)
	}
	if usedFallback {
		fmt.Fprintf(d.Stdout, "Model %q requires GPU acceleration; no GPU render node found, falling back to %q\n", modelName, resolvedModel)
	}
	modelName = resolvedModel
	stopWords := modelSpec.StopWords(modelName)
	initialPrompt := ""
	if shouldUseSpeechContext(modelName, opts.SpeechContext) {
		explicit := append([]string(nil), opts.Vocabulary...)
		vocabularyPath := speechcontext.VocabularyPath(d.Getenv("HOME"))
		if vocabularyPath != "" && d.ReadFile != nil {
			if data, readErr := d.ReadFile(vocabularyPath); readErr == nil {
				explicit = append(explicit, speechcontext.ParseVocabulary(data)...)
			} else if !os.IsNotExist(readErr) {
				fmt.Fprintf(d.Stdout, "Warning: cannot load local speech vocabulary: %v\n", readErr)
			}
		}
		cwd, _ := os.Getwd()
		initialPrompt = speechcontext.Build(speechcontext.Options{
			Enabled:      true,
			PromptPrefix: modelSpec.SpeechContext.PromptPrefix,
			MaxTerms:     modelSpec.SpeechContext.MaxTerms,
			MaxChars:     modelSpec.SpeechContext.MaxChars,
			MaxTermChars: modelSpec.SpeechContext.MaxTermChars,
		}, speechcontext.Sources{
			Explicit:   explicit,
			Static:     modelSpec.SpeechContext.Terms,
			Repository: speechcontext.DiscoverRepositoryTerms(ctx, cwd, 12),
		})
	}
	var silenceArtifacts []string
	if overrides, loadErr := feedback.Load(feedback.Path(d.Getenv("HOME"))); loadErr != nil {
		fmt.Fprintf(d.Stdout, "Warning: cannot load local stop-word feedback; using built-ins: %v\n", loadErr)
	} else {
		stopWords = feedback.ActivePatterns(modelSpec.BuiltinStopWords(modelName), overrides)
		silenceArtifacts = overrides.SilenceArtifacts
	}

	jobChan := make(chan TranscribeJob, 10)
	var transWg sync.WaitGroup
	var fullTranscript strings.Builder
	var transLock sync.Mutex
	utteranceCount := 0

	writeVoxtypeState("recording")
	defer writeVoxtypeState("idle")

	chunkBuf := chunks.NewBuffer(chunks.StorageDir(d.Getenv("XDG_RUNTIME_DIR"), d.Getenv("HOME")), chunks.DefaultBufferSize)

	// Start sequential transcription worker
	transWg.Add(1)
	go func() {
		defer transWg.Done()
		for job := range jobChan {
			transStart := time.Now()
			wavPath := filepath.Join(tmpDir, fmt.Sprintf("utt_%03d.wav", job.Index))
			if err := audio.WriteWAVAudio(wavPath, job.Audio, sampleRate); err != nil {
				if !isDaemon {
					fmt.Fprintf(d.Stdout, "Error writing utterance audio: %v\n", err)
				}
				continue
			}

			if !job.Plausible {
				// Acoustically unvoiced transient or click: do not waste GPU/CPU on voxtype
				rejReason := job.RejectionReason
				if rejReason == "" {
					rejReason = "low_energy_transient"
				}
				chunkMeta := chunks.Chunk{
					Index:                 job.Index,
					Timestamp:             transStart,
					AudioDurationSecs:     job.Duration,
					TranscribeDurationSec: 0,
					RTF:                   0,
					RawTranscript:         "",
					CleanedTranscript:     "",
					Accepted:              false,
					RejectionReason:       rejReason,
				}
				_, _ = chunkBuf.AddExistingWAV(chunkMeta, wavPath, true)
				_ = os.Remove(wavPath)
				continue
			}

			writeVoxtypeState("transcribing")
			cmdArgs := voxtypeTranscribeArgs(modelName, wavPath, initialPrompt)
			transcribeCtx, cancelTranscribe := context.WithTimeout(context.Background(), transcribeTimeout)
			cmd := exec.CommandContext(transcribeCtx, voxtypePath, cmdArgs...)
			cmd.Env = append(os.Environ(), "NO_COLOR=1", "RUST_LOG=error")
			var outBuf bytes.Buffer
			cmd.Stdout = &outBuf
			cmd.Stderr = io.Discard
			err := cmd.Run()
			cancelTranscribe()
			transDuration := time.Since(transStart).Seconds()
			writeVoxtypeState("recording")

			rawText := outBuf.String()
			text := asr.CleanWhisperTranscript(rawText, stopWords)
			accepted := acceptTranscript(err, text, stopWords, silenceArtifacts)
			rejReason := ""
			if !accepted {
				rejReason = rejectionReason(err, rawText, text, stopWords, silenceArtifacts)
			}

			rtf := 0.0
			if job.Duration > 0 {
				rtf = transDuration / job.Duration
			}

			// Store chunk audio and metadata into the bounded ring buffer
			chunkMeta := chunks.Chunk{
				Index:                 job.Index,
				Timestamp:             transStart,
				AudioDurationSecs:     job.Duration,
				TranscribeDurationSec: transDuration,
				RTF:                   rtf,
				RawTranscript:         strings.TrimSpace(rawText),
				CleanedTranscript:     text,
				Accepted:              accepted,
				RejectionReason:       rejReason,
			}
			_, _ = chunkBuf.AddExistingWAV(chunkMeta, wavPath, true)
			_ = os.Remove(wavPath) // Ensure removal if AddExistingWAV didn't move it

			if accepted {
				transLock.Lock()
				if fullTranscript.Len() > 0 {
					fullTranscript.WriteString(" ")
				}
				fullTranscript.WriteString(text)
				transLock.Unlock()

				if !isDaemon {
					fmt.Fprintf(d.Stdout, "  #%d [Audio: %.1fs, Transcribe: %.2fs] -> \x1b[32;1m%q\x1b[0m\n",
						job.Index, job.Duration, transDuration, text)
				}

				if opts.TypeOutput {
					_ = typing.TypeText(context.Background(), d, text+" ")
				}

				if historyPath != "" {
					_, _ = history.AppendHistory(historyPath, text, history.DefaultHistoryLimit, time.Now())
				}

				recordEagerStat(UtteranceStat{
					Index:          job.Index,
					AudioSecs:      job.Duration,
					TranscribeSecs: transDuration,
					RTF:            rtf,
					Text:           text,
					Timestamp:      time.Now(),
				})
			}
		}
	}()

	recCmd := exec.CommandContext(ctx, recCmdName, recArgs...)
	audioOut, err := recCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("audio pipe: %w", err)
	}
	recCmd.Stderr = io.Discard

	if err := recCmd.Start(); err != nil {
		return fmt.Errorf("start audio capture: %w", err)
	}

	killDone := make(chan struct{})
	defer close(killDone)
	go func() {
		select {
		case <-ctx.Done():
			if recCmd.Process != nil {
				_ = recCmd.Process.Kill()
			}
		case <-killDone:
		}
	}()

	defer func() {
		if recCmd.Process != nil {
			_ = recCmd.Process.Kill()
			_ = recCmd.Wait()
		}
	}()

	buf := make([]byte, frameBytes)
	frameIndex := 0

	if !isDaemon {
		fmt.Fprintln(d.Stdout, "🟢 Listening for voice activity...")
	}

	for {
		if ctx.Err() != nil {
			break
		}

		n, err := io.ReadFull(audioOut, buf)
		if err != nil {
			break
		}
		if n < frameBytes {
			continue
		}

		candidate, speechStarted, isSpeaking := segmenter.ProcessFrame(buf)

		frameIndex++
		if !isDaemon {
			if speechStarted {
				fmt.Fprintf(d.Stdout, "\r🎙️  [Speaking... buffering]                                  \n")
			} else if frameIndex%10 == 0 && !isSpeaking {
				rms := audio.ComputeAudioRMS(buf)
				meter := audio.RenderAudioLevelMeter(rms, opts.ThresholdRMS)
				fmt.Fprintf(d.Stdout, "\r  Level: %s RMS: %4d   ", meter, rms)
			}
		}

		if len(candidate.Audio) > 0 {
			utteranceCount++
			audioDur := float64(len(candidate.Audio)) / float64(bytesPerSec)
			jobChan <- TranscribeJob{
				Index:           utteranceCount,
				Audio:           candidate.Audio,
				Duration:        audioDur,
				Plausible:       candidate.Plausible,
				RejectionReason: candidate.RejectionReason,
			}
		}
	}

	// Flush remaining speech upon exit
	if finalCandidate := segmenter.Flush(); len(finalCandidate.Audio) > 0 {
		utteranceCount++
		audioDur := float64(len(finalCandidate.Audio)) / float64(bytesPerSec)
		jobChan <- TranscribeJob{
			Index:           utteranceCount,
			Audio:           finalCandidate.Audio,
			Duration:        audioDur,
			Plausible:       finalCandidate.Plausible,
			RejectionReason: finalCandidate.RejectionReason,
		}
	}

	close(jobChan)
	transWg.Wait()

	if !isDaemon {
		fmt.Fprintln(d.Stdout, "\n\n── Dictation Complete ──────────────────────────────────────────")
		fmt.Fprintf(d.Stdout, "Total Utterances Transcribed: %d\n", utteranceCount)
		fmt.Fprintf(d.Stdout, "Full Consolidated Transcript:\n\n\x1b[1m%s\x1b[0m\n", fullTranscript.String())
		fmt.Fprintln(d.Stdout, "────────────────────────────────────────────────────────────────")
	}

	return nil
}

func voxtypeTranscribeArgs(modelName, wavPath, initialPrompt string) []string {
	args := []string{"--model", modelName, "--threads", "6"}
	if initialPrompt != "" {
		args = append(args, "--initial-prompt", initialPrompt)
	}
	return append(args, "-q", "transcribe", wavPath)
}

func shouldUseSpeechContext(modelName string, enabled bool) bool {
	return enabled && modelName == "small.en"
}

// acceptTranscript is the single gate before an eager result can affect either
// the focused application or local history.
func acceptTranscript(err error, text string, stopWords, silenceArtifacts []string) bool {
	return err == nil && text != "" && asr.IsSafeToType(text, stopWords) && !feedback.IsSilenceArtifact(text, silenceArtifacts)
}

// rejectionReason returns the explanation if a chunk was rejected, or empty string if accepted.
func rejectionReason(err error, rawText, cleanedText string, stopWords, silenceArtifacts []string) string {
	if err != nil {
		return fmt.Sprintf("transcribe_error: %v", err)
	}
	if strings.TrimSpace(rawText) == "" {
		return "empty"
	}
	if feedback.IsSilenceArtifact(cleanedText, silenceArtifacts) {
		return "silence_artifact"
	}
	if !asr.IsSafeToType(cleanedText, stopWords) {
		return "stop_word"
	}
	if cleanedText == "" {
		return "empty"
	}
	return ""
}

func runEagerDaemon(ctx context.Context, d deps.Dependencies, opts EagerOptions) error {
	runtimeDir := d.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	sockDir := filepath.Join(runtimeDir, "voxi")
	_ = os.MkdirAll(sockDir, 0755)
	sockPath := filepath.Join(sockDir, "eager.sock")
	_ = os.Remove(sockPath)

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("listen unix socket %s: %w", sockPath, err)
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(sockPath)
	}()

	recCmdName := ""
	var recArgs []string
	if _, err := d.LookPath("pw-record"); err == nil {
		recCmdName = "pw-record"
		recArgs = []string{"--rate", "16000", "--channels", "1", "--format", "s16", "-"}
	} else if _, err := d.LookPath("arecord"); err == nil {
		recCmdName = "arecord"
		recArgs = []string{"-r", "16000", "-c", "1", "-f", "S16_LE", "-t", "raw", "-q", "-"}
	} else {
		return fmt.Errorf("audio capture tool (pw-record or arecord) missing")
	}

	voxtypePath, err := d.LookPath("voxtype")
	if err != nil {
		return fmt.Errorf("voxtype missing: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "voxi-eager-daemon-*")
	if err != nil {
		return fmt.Errorf("temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	writeVoxtypeState("idle")
	defer writeVoxtypeState("inactive")

	var mu sync.Mutex
	var activeCancel context.CancelFunc
	var sessWg sync.WaitGroup
	isRecording := false

	stopCurrent := func() {
		mu.Lock()
		if activeCancel != nil {
			activeCancel()
			activeCancel = nil
		}
		isRecording = false
		mu.Unlock()
		sessWg.Wait()
		writeVoxtypeState("idle")
	}

	startRecording := func() {
		stopCurrent()

		mu.Lock()
		sessCtx, cancel := context.WithCancel(ctx)
		activeCancel = cancel
		isRecording = true
		sessWg.Add(1)
		mu.Unlock()

		go func() {
			defer sessWg.Done()
			_ = runEagerCaptureSession(sessCtx, d, opts, tmpDir, voxtypePath, recCmdName, recArgs, true)
			mu.Lock()
			if activeCancel != nil {
				activeCancel = nil
			}
			isRecording = false
			mu.Unlock()
			writeVoxtypeState("idle")
		}()
	}

	toggleRecording := func() string {
		mu.Lock()
		rec := isRecording
		mu.Unlock()
		if rec {
			stopCurrent()
			return "Recording stopped"
		}
		startRecording()
		return "Recording started"
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGUSR1, syscall.SIGUSR2)
	go func() {
		for sig := range sigChan {
			if sig == syscall.SIGUSR1 {
				toggleRecording()
			} else if sig == syscall.SIGUSR2 {
				stopCurrent()
			}
		}
	}()

	go func() {
		<-ctx.Done()
		_ = listener.Close()
		stopCurrent()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			scanner := bufio.NewScanner(c)
			if scanner.Scan() {
				cmd := strings.TrimSpace(scanner.Text())
				switch cmd {
				case "toggle":
					msg := toggleRecording()
					_, _ = fmt.Fprintln(c, msg)
				case "start":
					startRecording()
					_, _ = fmt.Fprintln(c, "Recording started")
				case "stop":
					stopCurrent()
					_, _ = fmt.Fprintln(c, "Recording stopped")
				case "status":
					mu.Lock()
					rec := isRecording
					mu.Unlock()
					if rec {
						_, _ = fmt.Fprintln(c, "recording")
					} else {
						_, _ = fmt.Fprintln(c, "idle")
					}
				default:
					_, _ = fmt.Fprintln(c, "unknown command")
				}
			}
		}(conn)
	}

	return nil
}

// ControlEagerDaemon sends recording control actions to the eager daemon socket.
func ControlEagerDaemon(ctx context.Context, d deps.Dependencies, action string) error {
	runtimeDir := d.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	sockPath := filepath.Join(runtimeDir, "voxi", "eager.sock")
	if _, err := os.Stat(sockPath); err != nil {
		// Try legacy path
		legacySock := filepath.Join(runtimeDir, "harnez", "eager.sock")
		if _, err := os.Stat(legacySock); err == nil {
			sockPath = legacySock
		}
	}

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return fmt.Errorf("eager daemon not reachable at %s (is voxi-eager.service active?): %w", sockPath, err)
	}
	defer conn.Close()

	_, err = fmt.Fprintf(conn, "%s\n", action)
	if err != nil {
		return fmt.Errorf("send %s to eager daemon: %w", action, err)
	}

	res, _ := bufio.NewReader(conn).ReadString('\n')
	if strings.TrimSpace(res) != "" {
		fmt.Fprintln(d.Stdout, strings.TrimSpace(res))
	}
	return nil
}

// GetEagerRecordingStatus queries current recording/idle status from the eager daemon.
func GetEagerRecordingStatus(ctx context.Context, d deps.Dependencies) (string, error) {
	runtimeDir := d.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	sockPath := filepath.Join(runtimeDir, "voxi", "eager.sock")
	if _, err := os.Stat(sockPath); err != nil {
		legacySock := filepath.Join(runtimeDir, "harnez", "eager.sock")
		if _, err := os.Stat(legacySock); err == nil {
			sockPath = legacySock
		}
	}

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return "inactive", nil
	}
	defer conn.Close()

	_, _ = fmt.Fprintln(conn, "status")
	res, _ := bufio.NewReader(conn).ReadString('\n')
	status := strings.TrimSpace(res)
	if status == "" {
		return "idle", nil
	}
	return status, nil
}
