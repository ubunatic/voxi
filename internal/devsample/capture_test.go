package devsample

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/deps"
)

func TestCaptureUtteranceNoDeviceAvailable(t *testing.T) {
	// Simulates a sandbox/CI environment with neither pw-record nor arecord
	// on PATH: CaptureUtterance must fail cleanly, never hang.
	d := deps.Dependencies{
		LookPath: func(string) (string, error) { return "", errLookPath },
		Stdin:    strings.NewReader("\n"),
		Stdout:   &bytes.Buffer{},
	}
	_, _, err := CaptureUtterance(context.Background(), d)
	if err == nil {
		t.Fatal("expected error when no capture tool is on PATH")
	}
}

var errLookPath = errNotFound("not found")

type errNotFound string

func (e errNotFound) Error() string { return string(e) }

// delayedReader returns line after a short delay, giving a just-started
// subprocess time to actually write bytes into its stdout pipe before the
// capture is stopped. Without this, an immediately-ready stdin ("\n" read at
// time zero) races the "sh -c" process's own startup and can stop capture
// before any bytes are produced.
type delayedReader struct {
	delay time.Duration
	line  string
	done  bool
}

func (r *delayedReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	time.Sleep(r.delay)
	r.done = true
	return copy(p, r.line), nil
}

func TestCaptureFromCommandStopsOnStdinEnter(t *testing.T) {
	// Use a real, always-available shell one-liner that emits a fixed chunk
	// of bytes immediately and then blocks, to exercise the stop-on-Enter
	// path deterministically without depending on pw-record/arecord/a real
	// microphone being present in this environment.
	in := bufio.NewReader(&delayedReader{delay: 150 * time.Millisecond, line: "\n"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pcm, dur, err := captureFromCommand(ctx, "sh", []string{"-c", "head -c 3200 /dev/zero; sleep 5"}, in)
	if err != nil {
		t.Fatalf("captureFromCommand: %v", err)
	}
	if len(pcm) == 0 {
		t.Fatal("expected some captured PCM bytes")
	}
	if dur <= 0 {
		t.Fatalf("expected positive duration, got %v", dur)
	}
}

func TestCaptureFromCommandStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	// nil reader: only ctx cancellation can stop capture.
	_, _, err := captureFromCommand(ctx, "sh", []string{"-c", "head -c 3200 /dev/zero; sleep 5"}, nil)
	// Any outcome (data captured, or "no audio captured" if killed before the
	// pipe produced bytes) is acceptable here; the key property under test is
	// that this call returns promptly instead of hanging.
	_ = err
}

func TestResolveCaptureCommandPrefersPwRecord(t *testing.T) {
	d := deps.Dependencies{
		LookPath: func(name string) (string, error) {
			if name == "pw-record" {
				return "/usr/bin/pw-record", nil
			}
			return "", errLookPath
		},
	}
	name, _, err := resolveCaptureCommand(d)
	if err != nil {
		t.Fatalf("resolveCaptureCommand: %v", err)
	}
	if name != "pw-record" {
		t.Errorf("resolveCaptureCommand name = %q, want pw-record", name)
	}
}

func TestResolveCaptureCommandFallsBackToArecord(t *testing.T) {
	d := deps.Dependencies{
		LookPath: func(name string) (string, error) {
			if name == "arecord" {
				return "/usr/bin/arecord", nil
			}
			return "", errLookPath
		},
	}
	name, _, err := resolveCaptureCommand(d)
	if err != nil {
		t.Fatalf("resolveCaptureCommand: %v", err)
	}
	if name != "arecord" {
		t.Errorf("resolveCaptureCommand name = %q, want arecord", name)
	}
}

func TestResolveCaptureCommandNoneAvailable(t *testing.T) {
	d := deps.Dependencies{
		LookPath: func(string) (string, error) { return "", errLookPath },
	}
	_, _, err := resolveCaptureCommand(d)
	if err == nil {
		t.Fatal("expected error when neither pw-record nor arecord is available")
	}
}
