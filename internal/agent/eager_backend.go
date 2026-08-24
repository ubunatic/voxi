package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/eager"
)

const eagerBackendStartTimeout = 10 * time.Second

// EagerChildBackend runs the existing eager daemon as a child process owned by
// the agent. This is a migration bridge until eager capture moves behind the
// Backend interface directly.
type EagerChildBackend struct {
	d deps.Dependencies

	mu        sync.Mutex
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	running   bool
	recording RecordingState
}

// NewEagerChildBackend creates a backend that launches `voxi eager --daemon`.
func NewEagerChildBackend(d deps.Dependencies) *EagerChildBackend {
	return &EagerChildBackend{d: d, recording: RecordingIdle}
}

func (b *EagerChildBackend) Start(ctx context.Context) error {
	b.mu.Lock()
	if b.running {
		b.mu.Unlock()
		return nil
	}

	voxiPath, err := b.voxiPath()
	if err != nil {
		b.mu.Unlock()
		return err
	}
	childCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(childCtx, voxiPath, "eager", "--daemon")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		cancel()
		b.mu.Unlock()
		return fmt.Errorf("start eager child daemon: %w", err)
	}

	b.cmd = cmd
	b.cancel = cancel
	b.running = true
	b.recording = RecordingIdle
	b.mu.Unlock()

	if err := b.waitReady(ctx); err != nil {
		_ = b.Stop(context.Background())
		return err
	}
	return nil
}

func (b *EagerChildBackend) Stop(ctx context.Context) error {
	b.mu.Lock()
	cmd := b.cmd
	cancel := b.cancel
	if !b.running || cmd == nil {
		b.cmd = nil
		b.cancel = nil
		b.running = false
		b.recording = RecordingIdle
		b.mu.Unlock()
		return nil
	}
	b.cmd = nil
	b.cancel = nil
	b.running = false
	b.recording = RecordingIdle
	b.mu.Unlock()

	_ = eager.ControlEagerDaemon(ctx, b.d, "stop")
	if cancel != nil {
		cancel()
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil && !errors.Is(ctx.Err(), context.Canceled) {
			return fmt.Errorf("wait eager child daemon: %w", err)
		}
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
		return fmt.Errorf("stop eager child daemon: %w", ctx.Err())
	case <-time.After(3 * time.Second):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
		return errors.New("stop eager child daemon: timed out")
	}
	return nil
}

func (b *EagerChildBackend) Record(ctx context.Context, action RecordAction) (RecordingState, error) {
	b.mu.Lock()
	running := b.running
	b.mu.Unlock()
	if !running {
		return RecordingIdle, errors.New("backend is not running")
	}

	if err := eager.ControlEagerDaemon(ctx, b.d, string(action)); err != nil {
		return RecordingIdle, err
	}
	status, err := eager.GetEagerRecordingStatus(ctx, b.d)
	if err != nil {
		return RecordingIdle, err
	}
	recording := RecordingIdle
	if status == "recording" {
		recording = RecordingActive
	}

	b.mu.Lock()
	b.recording = recording
	b.mu.Unlock()
	return recording, nil
}

func (b *EagerChildBackend) Status() BackendStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	return BackendStatus{Running: b.running, Recording: b.recording}
}

func (b *EagerChildBackend) voxiPath() (string, error) {
	if b.d.LookPath != nil {
		if path, err := b.d.LookPath("voxi"); err == nil {
			return path, nil
		}
	}
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve voxi executable: %w", err)
	}
	return path, nil
}

func (b *EagerChildBackend) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(eagerBackendStartTimeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return fmt.Errorf("wait eager child daemon: %w", ctx.Err())
		}
		status, _ := eager.GetEagerRecordingStatus(ctx, b.d)
		if status != "inactive" {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("eager child daemon did not become reachable")
}
