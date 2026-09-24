package tts

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestClientWithoutMonitorFailsQuickly(t *testing.T) {
	client := Client{SocketPath: filepath.Join(t.TempDir(), "missing.sock")}
	started := time.Now()
	_, err := client.Enqueue(context.Background(), "hello")
	if !errors.Is(err, ErrNoMonitor) || !strings.Contains(err.Error(), "no monitor") {
		t.Fatalf("Enqueue() error = %v, want no monitor", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Enqueue() took %s with no monitor", elapsed)
	}
}

func TestClientReportsConfigurationOptOutWhileMonitorRuns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tts.sock")
	manager := NewManagerWithEnabled(context.Background(), &fakeBackend{}, false)
	defer manager.Close()
	server, err := StartServer(context.Background(), path, manager)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	_, err = (Client{SocketPath: path}).Enqueue(context.Background(), "not spoken")
	if !errors.Is(err, ErrTTSDisabled) {
		t.Fatalf("Enqueue() error = %v, want TTS-disabled response", err)
	}
}

func TestClientWithStaleSocketReportsNoMonitorQuickly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stale.sock")
	stale, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	stale.(*net.UnixListener).SetUnlinkOnClose(false)
	if err := stale.Close(); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	_, err = (Client{SocketPath: path}).Enqueue(context.Background(), "hello")
	if !errors.Is(err, ErrNoMonitor) || !strings.Contains(err.Error(), "no monitor") {
		t.Fatalf("Enqueue() error = %v, want no monitor", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Enqueue() took %s with a stale socket", elapsed)
	}
}

func TestServerRemovesStaleSocketAndAcceptsRequest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tts.sock")
	stale, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	stale.(*net.UnixListener).SetUnlinkOnClose(false)
	if err := stale.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("stale socket missing before start: %v", err)
	}

	controller := &recordingController{}
	ctx, cancel := context.WithCancel(context.Background())
	server, err := StartServer(ctx, path, controller)
	if err != nil {
		t.Fatalf("StartServer() with stale socket: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("socket mode = %04o, want 0600", got)
	}
	if got := infoForDir(t, dir).Mode().Perm(); got != 0700 {
		t.Fatalf("runtime directory mode = %04o, want 0700", got)
	}

	count, err := (Client{SocketPath: path}).Enqueue(context.Background(), "one. Two!")
	if err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("accepted count = %d, want 2", count)
	}
	if got := controller.text(); got != "one. Two!" {
		t.Fatalf("controller text = %q", got)
	}
	if err := (Client{SocketPath: path}).Control(context.Background(), Action("bad-action")); err == nil {
		t.Fatal("unknown control unexpectedly succeeded")
	}

	cancel()
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket remains after close: %v", err)
	}
}

func TestServerDoesNotReplaceActiveSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tts.sock")
	ctx, cancel := context.WithCancel(context.Background())
	server, err := StartServer(ctx, path, &recordingController{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		_ = server.Close()
	}()
	if _, err := StartServer(context.Background(), path, &recordingController{}); err == nil || !strings.Contains(err.Error(), "already listening") {
		t.Fatalf("second StartServer() error = %v, want active monitor error", err)
	}
}

type recordingController struct {
	mu       sync.Mutex
	lastText string
}

func (c *recordingController) Enqueue(text string) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastText = text
	return len(SplitText(text)), nil
}

func (c *recordingController) Control(Action) error { return nil }

func infoForDir(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}
