package eager

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/media"
	spec "ubunatic.com/voxi/spec"
)

// onePlayerBus is a session bus with a single media player that is playing.
type onePlayerBus struct {
	mu     sync.Mutex
	status string
	calls  []string
	paused chan struct{} // closed on the first Pause
}

func newOnePlayerBus() *onePlayerBus {
	return &onePlayerBus{status: media.StatusPlaying, paused: make(chan struct{})}
}

func (b *onePlayerBus) connect(context.Context) (media.Bus, error) { return b, nil }

func (b *onePlayerBus) Players(context.Context) ([]media.Player, error) {
	return []media.Player{{Name: "org.mpris.MediaPlayer2.music", Owner: ":1.7"}}, nil
}

func (b *onePlayerBus) Status(context.Context, media.Player) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.status, nil
}

func (b *onePlayerBus) Pause(context.Context, media.Player) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, "Pause")
	if b.status == media.StatusPlaying {
		b.status = media.StatusPaused
		close(b.paused)
	}
	return nil
}

func (b *onePlayerBus) Play(context.Context, media.Player) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, "Play")
	b.status = media.StatusPlaying
	return nil
}

func (b *onePlayerBus) Close() error { return nil }

func (b *onePlayerBus) snapshot() (string, []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.status, append([]string(nil), b.calls...)
}

// pipeCapture is an audio capture the test ends by closing its stream.
type pipeCapture struct {
	startErr error
	reader   *io.PipeReader
	writer   *io.PipeWriter
}

func newPipeCapture() *pipeCapture {
	r, w := io.Pipe()
	return &pipeCapture{reader: r, writer: w}
}

func (c *pipeCapture) Start(context.Context) (io.ReadCloser, error) {
	if c.startErr != nil {
		return nil, c.startErr
	}
	return c.reader, nil
}

func (c *pipeCapture) Kill()       { _ = c.writer.Close() }
func (c *pipeCapture) Wait() error { return nil }

func mediaSessionDeps(env map[string]string) deps.Dependencies {
	d := envDeps(env)
	d.LookPath = func(name string) (string, error) { return name, nil }
	d.Stdout = io.Discard
	return d
}

// A capture session pauses the playing player while it records and resumes it
// when the capture ends (issue 125).
func TestCaptureSessionPausesAndResumesMedia(t *testing.T) {
	bus := newOnePlayerBus()
	pauser := media.New(media.Options{Connect: bus.connect, Timeout: time.Minute})
	capture := newPipeCapture()
	opts := EagerOptions{Model: "small.en", AudioCapture: capture, mediaPauser: pauser}

	done := make(chan error, 1)
	go func() {
		done <- runEagerCaptureSession(context.Background(), mediaSessionDeps(nil), opts, t.TempDir(), "", nil, true, "media-session", nil, nil, nil)
	}()

	<-bus.paused // the recording is still running: the capture stream is open
	if status, calls := bus.snapshot(); status != media.StatusPaused || !reflect.DeepEqual(calls, []string{"Pause"}) {
		t.Fatalf("during the recording: status %q, calls %v; want Paused after one Pause", status, calls)
	}

	_ = capture.writer.Close() // the recording ends
	if err := <-done; err != nil {
		t.Fatalf("capture session: %v", err)
	}
	pauser.Wait()
	if status, calls := bus.snapshot(); status != media.StatusPlaying || !reflect.DeepEqual(calls, []string{"Pause", "Play"}) {
		t.Errorf("after the recording: status %q, calls %v; want Playing after Pause, Play", status, calls)
	}
}

// A recording that never starts must not leave the music paused.
func TestCaptureStartFailureResumesMedia(t *testing.T) {
	bus := newOnePlayerBus()
	pauser := media.New(media.Options{Connect: bus.connect, Timeout: time.Minute})
	capture := newPipeCapture()
	capture.startErr = errors.New("no microphone")
	opts := EagerOptions{Model: "small.en", AudioCapture: capture, mediaPauser: pauser}

	err := runEagerCaptureSession(context.Background(), mediaSessionDeps(nil), opts, t.TempDir(), "", nil, true, "media-session", nil, nil, nil)
	if err == nil {
		t.Fatal("capture session returned nil, want the start failure")
	}
	pauser.Wait()
	if status, calls := bus.snapshot(); status != media.StatusPlaying || !reflect.DeepEqual(calls, []string{"Pause", "Play"}) {
		t.Errorf("after a failed start: status %q, calls %v; want Playing after Pause, Play", status, calls)
	}
}

// With pause_media off a session leaves every player alone.
func TestCaptureSessionLeavesMediaAloneWhenSettingIsOff(t *testing.T) {
	home := t.TempDir()
	path := config.VoxiConfigYAMLPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("pause_media: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bus := newOnePlayerBus()
	pauser := media.New(media.Options{Connect: bus.connect, Timeout: time.Minute})
	capture := newPipeCapture()
	capture.startErr = errors.New("no microphone")
	opts := EagerOptions{Model: "small.en", AudioCapture: capture, mediaPauser: pauser}

	_ = runEagerCaptureSession(context.Background(), mediaSessionDeps(map[string]string{"HOME": home}), opts, t.TempDir(), "", nil, true, "media-session", nil, nil, nil)
	pauser.Wait()
	if status, calls := bus.snapshot(); status != media.StatusPlaying || len(calls) != 0 {
		t.Errorf("with pause_media off: status %q, calls %v; want Playing and no calls", status, calls)
	}
}

func envDeps(env map[string]string) deps.Dependencies {
	return deps.Dependencies{Getenv: func(key string) string { return env[key] }}
}

// Without a session bus address in the injected environment there must be no
// pauser at all: that is what keeps every other test of this package, which
// runs real capture sessions, away from the developer's media players.
func TestNewMediaPauserNeedsInjectedBusAddress(t *testing.T) {
	es, err := spec.LoadEager()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent/process-env-must-not-be-used")
	if p := newMediaPauser(envDeps(nil), es); p != nil {
		t.Error("newMediaPauser built a pauser without an injected DBUS_SESSION_BUS_ADDRESS")
	}
	if p := newMediaPauser(deps.Dependencies{}, es); p != nil {
		t.Error("newMediaPauser built a pauser without a Getenv")
	}
	if p := newMediaPauser(envDeps(map[string]string{"DBUS_SESSION_BUS_ADDRESS": "unix:path=/nonexistent/bus"}), es); p == nil {
		t.Error("newMediaPauser returned nil with a bus address")
	}
}

func TestSessionMediaPauserFollowsSetting(t *testing.T) {
	failConnect := func(context.Context) (media.Bus, error) { return nil, errors.New("no bus in tests") }
	pauser := media.New(media.Options{Connect: failConnect, Timeout: time.Second})

	writeConfig := func(t *testing.T, body string) string {
		t.Helper()
		home := t.TempDir()
		path := config.VoxiConfigYAMLPath(home)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return home
	}

	cases := []struct {
		name   string
		home   func(t *testing.T) string
		wantOn bool
	}{
		{"no HOME uses the default", func(*testing.T) string { return "" }, true},
		{"no config file uses the default", func(t *testing.T) string { return t.TempDir() }, true},
		{"key missing from config uses the default", func(t *testing.T) string { return writeConfig(t, "dictation_history: true\n") }, true},
		{"pause_media true", func(t *testing.T) string { return writeConfig(t, "pause_media: true\n") }, true},
		{"pause_media false", func(t *testing.T) string { return writeConfig(t, "pause_media: false\n") }, false},
		{"broken config uses the default", func(t *testing.T) string { return writeConfig(t, "pause_media: [\n") }, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sessionMediaPauser(envDeps(map[string]string{"HOME": c.home(t)}), pauser)
			if (got != nil) != c.wantOn {
				t.Errorf("session pauser on = %v, want %v", got != nil, c.wantOn)
			}
		})
	}
	if got := sessionMediaPauser(envDeps(nil), nil); got != nil {
		t.Error("a nil pauser must stay nil")
	}
}
