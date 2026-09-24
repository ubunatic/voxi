package record

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/tts"
)

func TestControlRecordingVoxtype(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	var invokedCmd string
	var invokedArgs []string

	d := deps.Dependencies{
		LookPath: func(name string) (string, error) {
			if name == "voxtype" {
				return "/usr/bin/voxtype", nil
			}
			return "", errors.New("not found")
		},
		Run: func(ctx context.Context, name string, args ...string) error {
			invokedCmd = name
			invokedArgs = args
			return nil
		},
	}

	if err := ControlRecording(context.Background(), d, RecordActionToggle); err != nil {
		t.Fatalf("ControlRecording toggle failed: %v", err)
	}

	if invokedCmd != "voxtype" || len(invokedArgs) != 2 || invokedArgs[0] != "record" || invokedArgs[1] != "toggle" {
		t.Fatalf("unexpected call: %s %v", invokedCmd, invokedArgs)
	}
}

func TestStopTTSForRecordingSendsStopToMonitor(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	events := make(chan string, 1)
	controller := &recordingTTSController{events: events}
	server, err := tts.StartServer(context.Background(), tts.SocketPath(runtimeDir, os.Getuid()), controller)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	d := deps.Dependencies{Getenv: os.Getenv}
	if err := stopTTSForRecording(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if got := <-events; got != "tts-stop" {
		t.Fatalf("event = %q, want TTS stop", got)
	}
}

func TestStopTTSForRecordingWarnsAndContinuesAfterShortTimeout(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	server, err := tts.StartServer(context.Background(), tts.SocketPath(runtimeDir, os.Getuid()), &delayedTTSController{delay: 600 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	var warning bytes.Buffer
	started := time.Now()
	d := deps.Dependencies{Getenv: os.Getenv, Stderr: &warning}
	if err := stopTTSForRecording(context.Background(), d); err != nil {
		t.Fatalf("TTS stop failure blocked recording: %v", err)
	}
	if elapsed := time.Since(started); elapsed >= 500*time.Millisecond {
		t.Fatalf("TTS stop delayed recording for %s; want < 500ms", elapsed)
	}
	if !strings.Contains(warning.String(), "warning") || !strings.Contains(warning.String(), "continuing") {
		t.Fatalf("missing warn-and-continue diagnostic: %q", warning.String())
	}
}

type recordingTTSController struct{ events chan<- string }

func (c *recordingTTSController) Enqueue(string) (int, error) { return 0, nil }
func (c *recordingTTSController) Control(action tts.Action) error {
	if action == tts.ActionStop {
		c.events <- "tts-stop"
	}
	return nil
}

type delayedTTSController struct{ delay time.Duration }

func (*delayedTTSController) Enqueue(string) (int, error) { return 0, nil }
func (c *delayedTTSController) Control(tts.Action) error {
	time.Sleep(c.delay)
	return errors.New("delayed test failure")
}

// listenEagerSocket starts a fake eager-daemon listener at
// $runtimeDir/voxi/eager.sock and returns the first line written to it by
// the client, delivered on the returned channel once a connection lands.
func listenEagerSocket(t *testing.T, runtimeDir string) <-chan string {
	t.Helper()
	sockDir := filepath.Join(runtimeDir, "voxi")
	if err := os.MkdirAll(sockDir, 0o755); err != nil {
		t.Fatalf("mkdir eager sock dir: %v", err)
	}
	ln, err := net.Listen("unix", filepath.Join(sockDir, "eager.sock"))
	if err != nil {
		t.Fatalf("listen eager sock: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	received := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		line, _ := bufio.NewReader(conn).ReadString('\n')
		received <- line
		fmt.Fprintln(conn, "ok")
	}()
	return received
}

// issue 077: with no voice-input systemd unit active (the common state
// before any dictation session has started, or the steady state when the
// default engine runs behind the eager daemon rather than a tracked
// systemd unit) and the model spec's default engine resolved to
// cohere-transcribe (not whisper), ControlRecording and GetRecordingStatus
// must reach the eager daemon directly and must never require voxtype on
// PATH.
func TestControlRecordingDefaultEngineNoVoxtype(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	received := listenEagerSocket(t, runtimeDir)

	d := deps.Dependencies{
		LookPath: func(name string) (string, error) {
			return "", errors.New("voxtype not found on PATH (deliberately absent)")
		},
		Run: func(ctx context.Context, name string, args ...string) error {
			// No voice-input systemd unit is active.
			return errors.New("systemctl: inactive")
		},
		Getenv: os.Getenv,
		Stdout: io.Discard,
	}

	if err := ControlRecording(context.Background(), d, RecordActionToggle); err != nil {
		t.Fatalf("ControlRecording toggle failed without voxtype on PATH: %v", err)
	}

	select {
	case line := <-received:
		if line != "toggle\n" {
			t.Fatalf("unexpected eager daemon line: %q", line)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("eager daemon never received the toggle action")
	}
}

func TestGetRecordingStatusDefaultEngineNoVoxtype(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	d := deps.Dependencies{
		LookPath: func(name string) (string, error) {
			return "", errors.New("voxtype not found on PATH (deliberately absent)")
		},
		Run: func(ctx context.Context, name string, args ...string) error {
			return errors.New("systemctl: inactive")
		},
		Getenv: os.Getenv,
	}

	// No eager daemon socket is listening either; GetEagerRecordingStatus
	// degrades to "inactive" rather than erroring, and -- critically --
	// this path must never touch voxtype for the default Cohere engine.
	status, err := GetRecordingStatus(context.Background(), d)
	if err != nil {
		t.Fatalf("GetRecordingStatus failed without voxtype on PATH: %v", err)
	}
	if status != "inactive" {
		t.Fatalf("unexpected status: %q", status)
	}
}
