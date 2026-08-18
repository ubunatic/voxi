package debug

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"ubunatic.com/voxi/internal/audio"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/typing"
	spec "ubunatic.com/voxi/spec"
)

// CanaryOptions holds tuning parameters for Voxtype streaming canary.
type CanaryOptions struct {
	ChunkSecs        float64
	LeftContextSecs  float64
	RightContextSecs float64
	VAD              bool
	VADThreshold     float64
	VADBackend       string
}

// DefaultCanaryOptions returns standard Parakeet streaming parameters.
func DefaultCanaryOptions() CanaryOptions {
	return CanaryOptions{
		ChunkSecs:        0.48,
		LeftContextSecs:  1.60,
		RightContextSecs: 0.48,
		VAD:              false,
		VADThreshold:     0.5,
		VADBackend:       "energy",
	}
}

// ValidateMelMultiple checks if duration in seconds is a valid multiple of 0.08s (8 mel frames @ 100fps).
func ValidateMelMultiple(name string, val float64) error {
	frames := math.Round(val * 100.0)
	if int(frames)%8 != 0 {
		return fmt.Errorf("%s (%.2fs = %.0f mel frames) must be a multiple of 0.08s (8 mel frames @ 100fps) for Parakeet ONNX", name, val, frames)
	}
	return nil
}

// TokenRecord holds a single captured token event.
type TokenRecord struct {
	Elapsed float64
	Delta   float64
	Text    string
	IsKey   bool
}

// RunStreamingCanary runs the live canary loop with an isolated voxtype daemon and clean terminal I/O.
func RunStreamingCanary(ctx context.Context, d deps.Dependencies, opts CanaryOptions) error {
	if err := ValidateMelMultiple("chunk", opts.ChunkSecs); err != nil {
		fmt.Fprintf(d.Stdout, "Warning: %v\n", err)
	}
	if err := ValidateMelMultiple("left-context", opts.LeftContextSecs); err != nil {
		fmt.Fprintf(d.Stdout, "Warning: %v\n", err)
	}
	if err := ValidateMelMultiple("right-context", opts.RightContextSecs); err != nil {
		fmt.Fprintf(d.Stdout, "Warning: %v\n", err)
	}

	voxtypePath, err := d.LookPath("voxtype")
	if err != nil {
		return fmt.Errorf("voxtype not found on PATH: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "voxi-canary-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	dotoolPipe := filepath.Join(tmpDir, "dotool-pipe")
	if err := syscall.Mkfifo(dotoolPipe, 0600); err != nil {
		return fmt.Errorf("create fifo %s: %w", dotoolPipe, err)
	}

	configPath := filepath.Join(tmpDir, "config.toml")
	configContent := fmt.Sprintf(`[input]
device = "default"

[output]
driver = "dotool"
type_delay_ms = 0

[streaming]
model = "parakeet-unified-en-0.6b"
chunk_duration_s = %.2f
left_context_s = %.2f
right_context_s = %.2f
blank_penalty = 1.5
temperature = 0.0
vad = %t
vad_threshold = %.2f
vad_backend = "%s"
`, opts.ChunkSecs, opts.LeftContextSecs, opts.RightContextSecs, opts.VAD, opts.VADThreshold, opts.VADBackend)

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		return fmt.Errorf("write config %s: %w", configPath, err)
	}

	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Fprintln(d.Stdout, "── Voxtype Real-Time Streaming Canary ──────────────────────────")
	fmt.Fprintf(d.Stdout, "  Chunk: %.2fs | Left Context: %.2fs | Right Context: %.2fs\n",
		opts.ChunkSecs, opts.LeftContextSecs, opts.RightContextSecs)
	fmt.Fprintf(d.Stdout, "  VAD: %t (threshold=%.2f, backend=%s)\n", opts.VAD, opts.VADThreshold, opts.VADBackend)
	fmt.Fprintln(d.Stdout, "────────────────────────────────────────────────────────────────")

	fifoFile, err := os.OpenFile(dotoolPipe, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open fifo %s: %w", dotoolPipe, err)
	}
	defer fifoFile.Close()

	cmd := exec.CommandContext(sigCtx, voxtypePath, "-c", configPath, "daemon", "--streaming")
	cmd.Env = append(os.Environ(), "DOTOOL_PIPE="+dotoolPipe)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start voxtype daemon: %w", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	var tokens []TokenRecord
	var tokensLock sync.Mutex
	recordingStart := time.Now()
	var lastEventTime time.Time

	go func() {
		scanner := bufio.NewScanner(fifoFile)
		for scanner.Scan() {
			line := scanner.Text()
			now := time.Now()
			elapsed := now.Sub(recordingStart).Seconds()
			delta := 0.0
			if !lastEventTime.IsZero() {
				delta = now.Sub(lastEventTime).Seconds()
			}
			lastEventTime = now

			rec := TokenRecord{Elapsed: elapsed, Delta: delta}
			if strings.HasPrefix(line, "type ") {
				rec.Text = strings.TrimPrefix(line, "type ")
				rec.IsKey = false
			} else if strings.HasPrefix(line, "key ") {
				rec.Text = "[" + strings.TrimPrefix(line, "key ") + "]"
				rec.IsKey = true
			} else {
				continue
			}

			tokensLock.Lock()
			tokens = append(tokens, rec)
			tokensLock.Unlock()

			if rec.IsKey {
				fmt.Fprintf(d.Stdout, "\x1b[90m%s\x1b[0m", rec.Text)
			} else {
				if delta > 0.8 && len(tokens) > 1 {
					fmt.Fprintf(d.Stdout, " \x1b[33m(pause %.2fs)\x1b[0m \x1b[32m%s\x1b[0m", delta, rec.Text)
				} else {
					fmt.Fprintf(d.Stdout, "\x1b[32m%s\x1b[0m", rec.Text)
				}
			}
		}
	}()

	fmt.Fprintln(d.Stdout, "\nReady. Press Enter to START recording...")
	bufio.NewReader(d.Stdin).ReadString('\n')

	toggleCmd := exec.CommandContext(sigCtx, voxtypePath, "-c", configPath, "record", "toggle")
	_ = toggleCmd.Run()
	recordingStart = time.Now()
	lastEventTime = time.Now()

	fmt.Fprintln(d.Stdout, "🔴 Recording! Speak naturally with pauses. Press Enter to STOP...")
	bufio.NewReader(d.Stdin).ReadString('\n')

	stopCmd := exec.CommandContext(context.Background(), voxtypePath, "-c", configPath, "record", "stop")
	_ = stopCmd.Run()
	time.Sleep(1 * time.Second)

	fmt.Fprintln(d.Stdout, "\n\n── Canary Summary ──────────────────────────────────────────────")
	tokensLock.Lock()
	fmt.Fprintf(d.Stdout, "Total Tokens Delivered: %d\n", len(tokens))
	tokensLock.Unlock()
	fmt.Fprintln(d.Stdout, "────────────────────────────────────────────────────────────────")

	return nil
}

// VADProbeOptions holds configuration parameters for VAD segmentation probe.
type VADProbeOptions struct {
	ThresholdRMS int
	SilenceMs    int
	PreRollMs    int
	MinSpeechMs  int
	TypeOutput   bool
}

// DefaultVADProbeOptions returns standard defaults for VAD segmentation.
func DefaultVADProbeOptions() VADProbeOptions {
	return VADProbeOptions{
		ThresholdRMS: 250,
		SilenceMs:    700,
		PreRollMs:    300,
		MinSpeechMs:  250,
		TypeOutput:   false,
	}
}

// RunVADProbe runs the interactive VAD segmentation dictation probe.
func RunVADProbe(ctx context.Context, d deps.Dependencies, opts VADProbeOptions) error {
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

	tmpDir, err := os.MkdirTemp("", "voxi-vad-probe-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	fmt.Fprintln(d.Stdout, "── VAD Sentence-by-Sentence Dictation Probe ────────────────────")
	fmt.Fprintf(d.Stdout, "  Audio Source:          %s (16kHz mono S16_LE)\n", recCmdName)
	fmt.Fprintf(d.Stdout, "  RMS Threshold:         %d\n", opts.ThresholdRMS)
	fmt.Fprintf(d.Stdout, "  Silence Cutoff:        %d ms\n", opts.SilenceMs)
	fmt.Fprintf(d.Stdout, "  Pre-roll Buffer:       %d ms (preserves initial consonants)\n", opts.PreRollMs)
	fmt.Fprintf(d.Stdout, "  Type into window:      %t\n", opts.TypeOutput)
	fmt.Fprintln(d.Stdout, "────────────────────────────────────────────────────────────────")

	segOpts := audio.SegmenterOptions{
		ThresholdRMS: opts.ThresholdRMS,
		SilenceMs:    opts.SilenceMs,
		PreRollMs:    opts.PreRollMs,
		MinSpeechMs:  opts.MinSpeechMs,
		MaxWindowMs:  8000,
	}
	segmenter := audio.NewAudioSegmenter(segOpts)

	modelSpec, err := spec.LoadModels()
	if err != nil {
		return fmt.Errorf("load model spec: %w", err)
	}

	recCmd := exec.CommandContext(ctx, recCmdName, recArgs...)
	audioOut, err := recCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	recCmd.Stderr = io.Discard
	if err := recCmd.Start(); err != nil {
		return fmt.Errorf("start audio capture: %w", err)
	}
	defer func() {
		if recCmd.Process != nil {
			_ = recCmd.Process.Kill()
			_ = recCmd.Wait()
		}
	}()

	buf := make([]byte, 640)
	uttCount := 0

	for {
		if ctx.Err() != nil {
			break
		}
		n, err := io.ReadFull(audioOut, buf)
		if err != nil {
			break
		}
		if n < 640 {
			continue
		}

		segment, _, _ := segmenter.ProcessFrame(buf)
		if len(segment) > 0 {
			uttCount++
			wavPath := filepath.Join(tmpDir, fmt.Sprintf("utt_%03d.wav", uttCount))
			_ = audio.WriteWAVAudio(wavPath, segment, 16000)

			cmd := exec.CommandContext(ctx, voxtypePath, "--model", modelSpec.DefaultModel, "-q", "transcribe", wavPath)
			out, err := cmd.Output()
			_ = os.Remove(wavPath)
			if err == nil {
				text := strings.TrimSpace(string(out))
				fmt.Fprintf(d.Stdout, "\n  #%d -> %s\n", uttCount, text)
				if opts.TypeOutput {
					_ = typing.TypeText(ctx, d, text+" ")
				}
			}
		}
	}
	return nil
}
