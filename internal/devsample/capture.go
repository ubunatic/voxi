package devsample

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"

	"ubunatic.com/voxi/internal/deps"
)

// SampleRate is the fixed capture rate, matching internal/eager's pipeline
// (16kHz mono S16LE), so recordings are directly usable by voxtype without
// resampling.
const SampleRate = 16000

// bytesPerSample is 16-bit mono PCM.
const bytesPerSample = 2

// resolveCaptureCommand picks the microphone capture tool the same way
// internal/eager does (pw-record preferred, arecord as fallback), but stops
// there: it does not pull in eager's VAD segmentation, transcription worker,
// or typing pipeline, and it does not depend on internal/record's
// already-running-daemon remote control. This is the smallest integration
// for a single, manually-triggered, one-shot capture.
func resolveCaptureCommand(d deps.Dependencies) (name string, args []string, err error) {
	if _, lookErr := d.LookPath("pw-record"); lookErr == nil {
		return "pw-record", []string{"--rate", fmt.Sprint(SampleRate), "--channels", "1", "--format", "s16", "-"}, nil
	}
	if _, lookErr := d.LookPath("arecord"); lookErr == nil {
		return "arecord", []string{"-r", fmt.Sprint(SampleRate), "-c", "1", "-f", "S16_LE", "-t", "raw", "-q", "-"}, nil
	}
	return "", nil, fmt.Errorf("neither pw-record nor arecord found on PATH; install PipeWire or ALSA utilities to record")
}

// CaptureUtterance records one manually-triggered utterance: it starts the
// microphone capture and stops on the first of (a) the user pressing Enter
// on d.Stdin, or (b) ctx being canceled, returning the raw 16kHz mono S16LE
// PCM captured in between.
func CaptureUtterance(ctx context.Context, d deps.Dependencies) ([]byte, time.Duration, error) {
	var in *bufio.Reader
	if d.Stdin != nil {
		in = bufio.NewReader(d.Stdin)
	}
	return captureUtteranceWithReader(ctx, d, in)
}

// captureUtteranceWithReader is CaptureUtterance's implementation, taking an
// explicit *bufio.Reader over d.Stdin. Callers (like Record) that also read
// other prompts from the same stdin must share one *bufio.Reader across all
// of them: a fresh bufio.Reader over an already-partially-consumed io.Reader
// silently drops whatever the previous reader had already buffered ahead.
func captureUtteranceWithReader(ctx context.Context, d deps.Dependencies, in *bufio.Reader) ([]byte, time.Duration, error) {
	name, args, err := resolveCaptureCommand(d)
	if err != nil {
		return nil, 0, err
	}
	if d.Stdout != nil {
		fmt.Fprintf(d.Stdout, "Recording with %s... speak now, then press Enter to stop.\n", name)
	}
	return captureFromCommand(ctx, name, args, in)
}

func captureFromCommand(ctx context.Context, name string, args []string, in *bufio.Reader) ([]byte, time.Duration, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, 0, fmt.Errorf("audio capture pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, 0, fmt.Errorf("start audio capture (%s): %w", name, err)
	}

	pcmCh := make(chan []byte, 1)
	go func() {
		buf, _ := io.ReadAll(stdout)
		pcmCh <- buf
	}()

	stopCh := make(chan struct{})
	go func() {
		defer close(stopCh)
		if in == nil {
			return
		}
		_, _ = in.ReadString('\n')
	}()

	select {
	case <-stopCh:
	case <-ctx.Done():
	}

	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	// Read the fully captured PCM before Wait: os/exec documents that Wait
	// must not be called until all reads from a StdoutPipe have completed.
	pcm := <-pcmCh
	_ = cmd.Wait()

	if len(pcm) == 0 {
		return nil, 0, fmt.Errorf("no audio captured; check that a microphone/audio device is available")
	}
	duration := time.Duration(float64(len(pcm)) / float64(SampleRate*bytesPerSample) * float64(time.Second))
	return pcm, duration, nil
}
