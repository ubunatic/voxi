package tts

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"ubunatic.com/voxi/internal/runtimepath"
)

const (
	maxRequestBytes = 1 << 20
	dialTimeout     = 300 * time.Millisecond
	requestTimeout  = 2 * time.Second
)

// ErrNoMonitor reports that no monitor watch session accepts TTS requests.
var ErrNoMonitor = errors.New("no monitor is running; start `voxi monitor -w` first")

// SocketPath returns the per-user monitor-gated TTS socket path.
func SocketPath(xdgRuntimeDir string, uid int) string {
	return filepath.Join(runtimepath.VoxiDir(xdgRuntimeDir, uid), "tts.sock")
}

// Action names monitor playback controls accepted over the TTS socket.
type Action string

const (
	ActionPlayPause      Action = "play-pause"
	ActionPause          Action = "pause"
	ActionResume         Action = "resume"
	ActionPrevious       Action = "previous"
	ActionNext           Action = "next"
	ActionStop           Action = "stop"
	ActionClear          Action = "clear"
	ActionRecordingStart Action = "recording-start"
	ActionRecordingEnd   Action = "recording-end"
)

// QueueController is the monitor-owned queue and playback surface.
type QueueController interface {
	Enqueue(string) (int, error)
	Control(Action) error
}

type request struct {
	Command Action `json:"command"`
	Text    string `json:"text,omitempty"`
}

type response struct {
	Accepted int       `json:"accepted,omitempty"`
	Error    string    `json:"error,omitempty"`
	Snapshot *Snapshot `json:"snapshot,omitempty"`
}

// Client sends bounded JSON-line requests to the active monitor.
type Client struct {
	SocketPath string
}

// Enqueue submits text for the monitor's playback queue.
func (c Client) Enqueue(ctx context.Context, text string) (int, error) {
	res, err := c.request(ctx, request{Command: "say", Text: text})
	return res.Accepted, err
}

// Replace submits text after stopping current playback and clearing queued text.
func (c Client) Replace(ctx context.Context, text string) (int, error) {
	res, err := c.request(ctx, request{Command: "replace", Text: text})
	return res.Accepted, err
}

// Control sends a monitor queue control action.
func (c Client) Control(ctx context.Context, action Action) error {
	_, err := c.request(ctx, request{Command: action})
	return err
}

// Snapshot reads queue telemetry without modifying playback.
func (c Client) Snapshot(ctx context.Context) (Snapshot, error) {
	res, err := c.request(ctx, request{Command: "snapshot"})
	if err != nil {
		return Snapshot{}, err
	}
	if res.Snapshot == nil {
		return Snapshot{}, errors.New("TTS server returned no snapshot")
	}
	return *res.Snapshot, nil
}

func (c Client) request(ctx context.Context, req request) (response, error) {
	path := c.SocketPath
	if path == "" {
		path = SocketPath(os.Getenv("XDG_RUNTIME_DIR"), os.Getuid())
	}
	dialer := net.Dialer{Timeout: dialTimeout}
	conn, err := dialer.DialContext(ctx, "unix", path)
	if err != nil {
		return response{}, fmt.Errorf("%w (%s): %v", ErrNoMonitor, path, err)
	}
	defer conn.Close()
	deadline := time.Now().Add(requestTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return response{}, fmt.Errorf("send TTS request: %w", err)
	}
	var res response
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&res); err != nil {
		return response{}, fmt.Errorf("read TTS response: %w", err)
	}
	if res.Error != "" {
		if res.Error == ErrTTSDisabled.Error() {
			return response{}, ErrTTSDisabled
		}
		return response{}, errors.New(res.Error)
	}
	return res, nil
}

// Server owns the monitor-only listening socket.
type Server struct {
	listener   net.Listener
	controller QueueController
	socketInfo os.FileInfo
	done       chan struct{}
	handlers   sync.WaitGroup
}

// StartServer opens the TTS socket, removing a refused stale socket left by a crash.
func StartServer(ctx context.Context, path string, controller QueueController) (*Server, error) {
	if path == "" {
		path = SocketPath(os.Getenv("XDG_RUNTIME_DIR"), os.Getuid())
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create TTS runtime directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("secure TTS runtime directory: %w", err)
	}
	listener, err := net.Listen("unix", path)
	if errors.Is(err, syscall.EADDRINUSE) {
		if cleanErr := removeStaleSocket(path); cleanErr != nil {
			return nil, cleanErr
		}
		listener, err = net.Listen("unix", path)
	}
	if err != nil {
		return nil, fmt.Errorf("listen on TTS socket %s: %w", path, err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("secure TTS socket: %w", err)
	}
	socketInfo, err := os.Lstat(path)
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("inspect TTS socket after bind: %w", err)
	}
	s := &Server{listener: listener, controller: controller, socketInfo: socketInfo, done: make(chan struct{})}
	go s.serve(ctx, path)
	return s, nil
}

// Close stops accepting TTS requests and removes the socket.
func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	_ = s.listener.Close()
	<-s.done
	return nil
}

func (s *Server) serve(ctx context.Context, path string) {
	defer func() {
		s.handlers.Wait()
		close(s.done)
	}()
	defer removeOwnedSocket(path, s.socketInfo)
	go func() {
		select {
		case <-ctx.Done():
			_ = s.listener.Close()
		case <-s.done:
		}
	}()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.handlers.Add(1)
		go func() {
			defer s.handlers.Done()
			s.handle(conn)
		}()
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(requestTimeout))
	decoder := json.NewDecoder(ioLimitReader(conn, maxRequestBytes))
	var req request
	if err := decoder.Decode(&req); err != nil {
		_ = json.NewEncoder(conn).Encode(response{Error: "invalid TTS request"})
		return
	}
	res := response{}
	switch req.Command {
	case "snapshot":
		if provider, ok := s.controller.(interface{ Snapshot() Snapshot }); ok {
			res.Snapshot = ptrSnapshot(provider.Snapshot())
		} else {
			res.Error = "TTS telemetry unavailable"
		}
	case "say":
		if strings.TrimSpace(req.Text) == "" {
			res.Error = "text is empty"
		} else if count, err := s.controller.Enqueue(req.Text); err != nil {
			res.Error = err.Error()
		} else {
			res.Accepted = count
		}
	case "replace":
		if strings.TrimSpace(req.Text) == "" {
			res.Error = "text is empty"
		} else if replacer, ok := s.controller.(interface{ Replace(string) (int, error) }); !ok {
			res.Error = "TTS replacement is unavailable"
		} else if count, err := replacer.Replace(req.Text); err != nil {
			res.Error = err.Error()
		} else {
			res.Accepted = count
		}
	case ActionPlayPause, ActionPause, ActionResume, ActionPrevious, ActionNext, ActionStop, ActionClear, ActionRecordingStart, ActionRecordingEnd:
		if err := s.controller.Control(req.Command); err != nil {
			res.Error = err.Error()
		}
	default:
		res.Error = "unknown TTS command"
	}
	_ = json.NewEncoder(conn).Encode(res)
}

func ptrSnapshot(s Snapshot) *Snapshot { return &s }

func removeStaleSocket(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect existing TTS socket: %w", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("TTS socket path exists but is not a socket")
	}
	conn, dialErr := net.DialTimeout("unix", path, dialTimeout)
	if dialErr == nil {
		_ = conn.Close()
		return fmt.Errorf("TTS monitor is already listening on %s", path)
	}
	if !errors.Is(dialErr, syscall.ECONNREFUSED) && !errors.Is(dialErr, syscall.ENOENT) {
		return fmt.Errorf("cannot verify whether existing TTS socket is stale: %w", dialErr)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove stale TTS socket: %w", err)
	}
	return nil
}

func removeOwnedSocket(path string, owned os.FileInfo) {
	current, err := os.Lstat(path)
	if err != nil || owned == nil || !os.SameFile(owned, current) {
		return
	}
	_ = os.Remove(path)
}

type limitedReader struct {
	reader *bufio.Reader
	left   int64
}

func ioLimitReader(r net.Conn, limit int64) *limitedReader {
	return &limitedReader{reader: bufio.NewReader(r), left: limit}
}

func (r *limitedReader) Read(p []byte) (int, error) {
	if r.left <= 0 {
		return 0, fmt.Errorf("TTS request exceeds size limit")
	}
	if int64(len(p)) > r.left {
		p = p[:r.left]
	}
	n, err := r.reader.Read(p)
	r.left -= int64(n)
	return n, err
}
