package record

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/deps"
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
