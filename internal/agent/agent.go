// Package agent provides the Voxi daemon control plane.
package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ubunatic.com/voxi/internal/deps"
)

// Mode identifies the transcription backend selected by the Voxi agent.
type Mode string

const (
	// ModeBatch uses utterance-at-a-time transcription.
	ModeBatch Mode = "batch"
	// ModeEager uses sentence-oriented rolling transcription.
	ModeEager Mode = "eager"
	// ModeStreaming uses incremental streaming transcription.
	ModeStreaming Mode = "streaming"
)

// RecordingState reports whether the selected backend is accepting microphone input.
type RecordingState string

const (
	// RecordingIdle means the backend is ready but not recording.
	RecordingIdle RecordingState = "idle"
	// RecordingActive means the backend is recording.
	RecordingActive RecordingState = "recording"
)

// RecordAction is a recording control verb.
type RecordAction string

const (
	// RecordToggle toggles recording.
	RecordToggle RecordAction = "toggle"
	// RecordStart starts recording.
	RecordStart RecordAction = "start"
	// RecordStop stops recording.
	RecordStop RecordAction = "stop"
)

// ErrUnavailable means no Voxi agent is listening on the requested socket.
var ErrUnavailable = errors.New("voxi agent unavailable")

// ErrRecordingActive means a backend transition was requested while recording.
var ErrRecordingActive = errors.New("cannot change mode while recording")

// BackendStatus describes the selected backend's lifecycle state.
type BackendStatus struct {
	Running   bool           `json:"running"`
	Recording RecordingState `json:"recording"`
}

// Backend is the lifecycle contract implemented by each transcription mode.
// The first agent slice uses state-only backends; later slices can replace them
// with adapters around the existing batch, eager, and streaming implementations.
type Backend interface {
	Start(context.Context) error
	Stop(context.Context) error
	Record(context.Context, RecordAction) (RecordingState, error)
	Status() BackendStatus
}

// Status is the control-plane view returned to clients.
type Status struct {
	Mode      Mode           `json:"mode"`
	Recording RecordingState `json:"recording"`
	Backend   BackendStatus  `json:"backend"`
}

// Options configures an Agent.
type Options struct {
	StatePath string
	Backends  map[Mode]Backend
}

// Agent serializes mode and recording transitions for one Voxi session.
type Agent struct {
	mu        sync.Mutex
	mode      Mode
	statePath string
	backends  map[Mode]Backend
	started   bool
}

type persistedState struct {
	Mode Mode `json:"mode"`
}

// DefaultSocketPath returns the per-user Unix socket used by the agent.
func DefaultSocketPath() string {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	return filepath.Join(runtimeDir, "voxi", "agent.sock")
}

// DefaultStatePath returns the persistent selected-mode state location.
func DefaultStatePath() string {
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return filepath.Join(os.TempDir(), "voxi-agent-state.json")
		}
		stateDir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateDir, "voxi", "agent.json")
}

// New creates an agent and restores its last selected mode. Eager is the
// default until a mode is explicitly persisted.
func New(opts Options) (*Agent, error) {
	if opts.StatePath == "" {
		opts.StatePath = DefaultStatePath()
	}
	if opts.Backends == nil {
		opts.Backends = defaultBackends()
	}
	for _, mode := range []Mode{ModeBatch, ModeEager, ModeStreaming} {
		if opts.Backends[mode] == nil {
			return nil, fmt.Errorf("agent backend %q is not configured", mode)
		}
	}

	selected := ModeEager
	data, err := os.ReadFile(opts.StatePath)
	if err == nil {
		var state persistedState
		if err := json.Unmarshal(data, &state); err != nil {
			return nil, fmt.Errorf("read agent state %s: %w", opts.StatePath, err)
		}
		if !validMode(state.Mode) {
			return nil, fmt.Errorf("read agent state %s: invalid mode %q", opts.StatePath, state.Mode)
		}
		selected = state.Mode
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read agent state %s: %w", opts.StatePath, err)
	}

	return &Agent{mode: selected, statePath: opts.StatePath, backends: opts.Backends}, nil
}

// RuntimeBackends returns the transitional production backend set. Eager is
// functional through an agent-owned child `voxi eager --daemon`; batch and
// streaming remain state-only until their real backend slices land.
func RuntimeBackends(d deps.Dependencies) map[Mode]Backend {
	return map[Mode]Backend{
		ModeBatch:     &stateBackend{recording: RecordingIdle},
		ModeEager:     NewEagerChildBackend(d),
		ModeStreaming: &stateBackend{recording: RecordingIdle},
	}
}

// Start starts the restored selected backend. It is safe to call more than once.
func (a *Agent) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.started {
		return nil
	}
	if err := a.backends[a.mode].Start(ctx); err != nil {
		return fmt.Errorf("start %s backend: %w", a.mode, err)
	}
	a.started = true
	return nil
}

// Stop stops the active backend and leaves the agent available for a later Start.
func (a *Agent) Stop(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.started {
		return nil
	}
	if err := a.backends[a.mode].Stop(ctx); err != nil {
		return fmt.Errorf("stop %s backend: %w", a.mode, err)
	}
	a.started = false
	return nil
}

// Status returns a consistent snapshot of the selected backend.
func (a *Agent) Status() Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	backend := a.backends[a.mode].Status()
	return Status{Mode: a.mode, Recording: backend.Recording, Backend: backend}
}

// SetMode switches the active backend. Switching while recording is refused so
// a future concrete backend never loses an in-flight utterance.
func (a *Agent) SetMode(ctx context.Context, target Mode) (Status, error) {
	if !validMode(target) {
		return Status{}, fmt.Errorf("invalid agent mode %q (want batch, streaming, or eager)", target)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	current := a.backends[a.mode]
	if current.Status().Recording == RecordingActive {
		return Status{}, ErrRecordingActive
	}
	if target == a.mode {
		return a.statusLocked(), nil
	}

	next := a.backends[target]
	if a.started {
		if err := current.Stop(ctx); err != nil {
			return Status{}, fmt.Errorf("stop %s backend: %w", a.mode, err)
		}
		if err := next.Start(ctx); err != nil {
			if restoreErr := current.Start(ctx); restoreErr != nil {
				return Status{}, fmt.Errorf("start %s backend: %w; restore %s backend: %v", target, err, a.mode, restoreErr)
			}
			return Status{}, fmt.Errorf("start %s backend: %w", target, err)
		}
	}

	previous := a.mode
	a.mode = target
	if err := a.persistLocked(); err != nil {
		a.mode = previous
		if a.started {
			_ = next.Stop(ctx)
			_ = current.Start(ctx)
		}
		return Status{}, err
	}
	return a.statusLocked(), nil
}

// Record forwards a recording action to the selected backend.
func (a *Agent) Record(ctx context.Context, action RecordAction) (Status, error) {
	if !validRecordAction(action) {
		return Status{}, fmt.Errorf("unknown record action %q (want toggle, start, or stop)", action)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.started {
		return Status{}, fmt.Errorf("%s backend is not running", a.mode)
	}
	if _, err := a.backends[a.mode].Record(ctx, action); err != nil {
		return Status{}, fmt.Errorf("record %s: %w", action, err)
	}
	return a.statusLocked(), nil
}

func (a *Agent) statusLocked() Status {
	backend := a.backends[a.mode].Status()
	return Status{Mode: a.mode, Recording: backend.Recording, Backend: backend}
}

func (a *Agent) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(a.statePath), 0700); err != nil {
		return fmt.Errorf("create agent state directory: %w", err)
	}
	data, err := json.Marshal(persistedState{Mode: a.mode})
	if err != nil {
		return fmt.Errorf("encode agent state: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(a.statePath), ".agent-*.json")
	if err != nil {
		return fmt.Errorf("create agent state temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write agent state: %w", err)
	}
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set agent state permissions: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close agent state: %w", err)
	}
	if err := os.Rename(tmpName, a.statePath); err != nil {
		return fmt.Errorf("persist agent state: %w", err)
	}
	return nil
}

func validMode(mode Mode) bool {
	return mode == ModeBatch || mode == ModeEager || mode == ModeStreaming
}

func validRecordAction(action RecordAction) bool {
	return action == RecordToggle || action == RecordStart || action == RecordStop
}

type stateBackend struct {
	mu        sync.Mutex
	running   bool
	recording RecordingState
}

func defaultBackends() map[Mode]Backend {
	return map[Mode]Backend{
		ModeBatch:     &stateBackend{recording: RecordingIdle},
		ModeEager:     &stateBackend{recording: RecordingIdle},
		ModeStreaming: &stateBackend{recording: RecordingIdle},
	}
}

func (b *stateBackend) Start(context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.running = true
	b.recording = RecordingIdle
	return nil
}

func (b *stateBackend) Stop(context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.running = false
	b.recording = RecordingIdle
	return nil
}

func (b *stateBackend) Record(_ context.Context, action RecordAction) (RecordingState, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.running {
		return b.recording, errors.New("backend is not running")
	}
	switch action {
	case RecordToggle:
		if b.recording == RecordingActive {
			b.recording = RecordingIdle
		} else {
			b.recording = RecordingActive
		}
	case RecordStart:
		b.recording = RecordingActive
	case RecordStop:
		b.recording = RecordingIdle
	}
	return b.recording, nil
}

func (b *stateBackend) Status() BackendStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	return BackendStatus{Running: b.running, Recording: b.recording}
}

// Serve exposes an Agent over a per-user Unix socket until ctx is canceled.
func Serve(ctx context.Context, socketPath string, a *Agent) error {
	if socketPath == "" {
		socketPath = DefaultSocketPath()
	}
	if err := os.MkdirAll(filepath.Dir(socketPath), 0700); err != nil {
		return fmt.Errorf("create agent socket directory: %w", err)
	}
	if err := removeSocket(socketPath); err != nil {
		return fmt.Errorf("remove stale agent socket %s: %w", socketPath, err)
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen agent socket %s: %w", socketPath, err)
	}
	if err := os.Chmod(socketPath, 0600); err != nil {
		_ = listener.Close()
		return fmt.Errorf("set agent socket permissions: %w", err)
	}
	defer func() {
		_ = listener.Close()
		_ = removeSocket(socketPath)
	}()

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("accept agent control connection: %w", err)
		}
		go handleConnection(ctx, a, conn)
	}
}

func removeSocket(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("path exists but is not a Unix socket")
	}
	return os.Remove(path)
}

func handleConnection(ctx context.Context, a *Agent, conn net.Conn) {
	defer conn.Close()
	var req request
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&req); err != nil {
		_ = json.NewEncoder(conn).Encode(response{Error: fmt.Sprintf("decode request: %v", err)})
		return
	}
	res := response{}
	switch req.Command {
	case "status":
		status := a.Status()
		res.Status = &status
	case "set-mode":
		status, err := a.SetMode(ctx, req.Mode)
		if err != nil {
			res.Error = err.Error()
		} else {
			res.Status = &status
		}
	case "record":
		status, err := a.Record(ctx, req.Action)
		if err != nil {
			res.Error = err.Error()
		} else {
			res.Status = &status
		}
	default:
		res.Error = fmt.Sprintf("unknown agent command %q", req.Command)
	}
	_ = json.NewEncoder(conn).Encode(res)
}

type request struct {
	Command string       `json:"command"`
	Mode    Mode         `json:"mode,omitempty"`
	Action  RecordAction `json:"action,omitempty"`
}

type response struct {
	Status *Status `json:"status,omitempty"`
	Error  string  `json:"error,omitempty"`
}

// Client talks to a running Voxi agent.
type Client struct {
	SocketPath string
}

// DefaultClient returns a client for the current user's agent socket.
func DefaultClient() Client {
	return Client{SocketPath: DefaultSocketPath()}
}

// Status queries the active mode and recording state.
func (c Client) Status(ctx context.Context) (Status, error) {
	return c.request(ctx, request{Command: "status"})
}

// SetMode asks the agent to switch backend modes.
func (c Client) SetMode(ctx context.Context, mode Mode) (Status, error) {
	return c.request(ctx, request{Command: "set-mode", Mode: mode})
}

// Record asks the agent to control its selected backend's recording state.
func (c Client) Record(ctx context.Context, action RecordAction) (Status, error) {
	return c.request(ctx, request{Command: "record", Action: action})
}

func (c Client) request(ctx context.Context, req request) (Status, error) {
	socketPath := c.SocketPath
	if socketPath == "" {
		socketPath = DefaultSocketPath()
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
	if err != nil {
		return Status{}, fmt.Errorf("%w at %s: %v", ErrUnavailable, socketPath, err)
	}
	defer conn.Close()
	deadline := time.Now().Add(5 * time.Second)
	if deadlineFromContext, ok := ctx.Deadline(); ok && deadlineFromContext.Before(deadline) {
		deadline = deadlineFromContext
	}
	_ = conn.SetDeadline(deadline)
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Status{}, fmt.Errorf("send agent command: %w", err)
	}
	var res response
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&res); err != nil {
		return Status{}, fmt.Errorf("read agent response: %w", err)
	}
	if res.Error != "" {
		return Status{}, errors.New(res.Error)
	}
	if res.Status == nil {
		return Status{}, errors.New("agent returned no status")
	}
	return *res.Status, nil
}
