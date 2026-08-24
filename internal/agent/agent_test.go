package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSetModePersistsAndSwitchesBackends(t *testing.T) {
	backends, backendConfig := testBackends()
	statePath := filepath.Join(t.TempDir(), "state", "agent.json")
	a, err := New(Options{StatePath: statePath, Backends: backendConfig})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got := backends[ModeEager].starts; got != 1 {
		t.Fatalf("eager starts = %d, want 1", got)
	}

	status, err := a.SetMode(context.Background(), ModeBatch)
	if err != nil {
		t.Fatalf("SetMode(batch) error = %v", err)
	}
	if status.Mode != ModeBatch || !status.Backend.Running {
		t.Fatalf("SetMode(batch) status = %+v", status)
	}
	if backends[ModeEager].stops != 1 || backends[ModeBatch].starts != 1 {
		t.Fatalf("backend lifecycle = eager stops %d, batch starts %d", backends[ModeEager].stops, backends[ModeBatch].starts)
	}

	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile(state) error = %v", err)
	}
	var state persistedState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if state.Mode != ModeBatch {
		t.Fatalf("persisted mode = %q, want %q", state.Mode, ModeBatch)
	}
}

func TestSetModeRefusesActiveRecording(t *testing.T) {
	a, err := New(Options{StatePath: filepath.Join(t.TempDir(), "agent.json")})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if _, err := a.Record(context.Background(), RecordStart); err != nil {
		t.Fatalf("Record(start) error = %v", err)
	}
	if _, err := a.SetMode(context.Background(), ModeBatch); !errors.Is(err, ErrRecordingActive) {
		t.Fatalf("SetMode while recording error = %v, want ErrRecordingActive", err)
	}
	if got := a.Status().Mode; got != ModeEager {
		t.Fatalf("mode after refused switch = %q, want %q", got, ModeEager)
	}
}

func TestServeClientCommands(t *testing.T) {
	dir := t.TempDir()
	a, err := New(Options{StatePath: filepath.Join(dir, "agent.json")})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	socketPath := filepath.Join(dir, "agent.sock")
	go func() { done <- Serve(ctx, socketPath, a) }()
	client := Client{SocketPath: socketPath}
	waitForAgent(t, client)

	status, err := client.SetMode(context.Background(), ModeStreaming)
	if err != nil {
		t.Fatalf("SetMode(streaming) error = %v", err)
	}
	if status.Mode != ModeStreaming || status.Recording != RecordingIdle {
		t.Fatalf("SetMode(streaming) status = %+v", status)
	}
	status, err = client.Record(context.Background(), RecordStart)
	if err != nil {
		t.Fatalf("Record(start) error = %v", err)
	}
	if status.Recording != RecordingActive {
		t.Fatalf("Record(start) status = %+v", status)
	}
	if _, err := client.SetMode(context.Background(), ModeBatch); err == nil {
		t.Fatal("SetMode(batch) while recording succeeded")
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
}

func TestClientUnavailable(t *testing.T) {
	client := Client{SocketPath: filepath.Join(t.TempDir(), "missing.sock")}
	if _, err := client.Status(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Status() error = %v, want ErrUnavailable", err)
	}
}

func TestServeRefusesToReplaceNonSocket(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "agent.sock")
	if err := os.WriteFile(socketPath, []byte("do not replace"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	a, err := New(Options{StatePath: filepath.Join(t.TempDir(), "agent.json")})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := Serve(context.Background(), socketPath, a); err == nil {
		t.Fatal("Serve() with regular file succeeded")
	}
	data, err := os.ReadFile(socketPath)
	if err != nil {
		t.Fatalf("ReadFile(socket path) error = %v", err)
	}
	if string(data) != "do not replace" {
		t.Fatalf("socket path contents = %q, want original file", data)
	}
}

func waitForAgent(t *testing.T, client Client) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := client.Status(context.Background()); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("agent did not become reachable")
}

type testBackend struct {
	mu        sync.Mutex
	starts    int
	stops     int
	running   bool
	recording RecordingState
}

func testBackends() (map[Mode]*testBackend, map[Mode]Backend) {
	backends := map[Mode]*testBackend{
		ModeBatch:     {},
		ModeEager:     {},
		ModeStreaming: {},
	}
	return backends, map[Mode]Backend{
		ModeBatch:     backends[ModeBatch],
		ModeEager:     backends[ModeEager],
		ModeStreaming: backends[ModeStreaming],
	}
}

func (b *testBackend) Start(context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.starts++
	b.running = true
	b.recording = RecordingIdle
	return nil
}

func (b *testBackend) Stop(context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stops++
	b.running = false
	b.recording = RecordingIdle
	return nil
}

func (b *testBackend) Record(_ context.Context, action RecordAction) (RecordingState, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch action {
	case RecordStart:
		b.recording = RecordingActive
	case RecordStop:
		b.recording = RecordingIdle
	case RecordToggle:
		if b.recording == RecordingActive {
			b.recording = RecordingIdle
		} else {
			b.recording = RecordingActive
		}
	}
	return b.recording, nil
}

func (b *testBackend) Status() BackendStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	return BackendStatus{Running: b.running, Recording: b.recording}
}
