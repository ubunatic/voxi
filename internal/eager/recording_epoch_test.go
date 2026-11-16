package eager

import (
	"context"
	"os"
	"sync"
	"testing"

	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/tts"
)

type epochController struct {
	mu      sync.Mutex
	actions []tts.Action
}

func (c *epochController) Enqueue(string) (int, error) { return 0, nil }
func (c *epochController) Control(action tts.Action) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.actions = append(c.actions, action)
	return nil
}

// shortTempDir allocates a minimal temp directory under $TMPDIR so Unix
// domain sockets stay well within Linux's 107-byte sun_path limit.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "vx")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestEagerCaptureMutesAndUnmutesPlaybackEpoch(t *testing.T) {
	runtimeDir := shortTempDir(t)
	controller := &epochController{}
	ctx, cancel := context.WithCancel(context.Background())
	server, err := tts.StartServer(ctx, tts.SocketPath(runtimeDir, os.Getuid()), controller)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = server.Close() }()
	d := deps.Dependencies{Getenv: func(key string) string {
		if key == "XDG_RUNTIME_DIR" {
			return runtimeDir
		}
		return ""
	}}
	beginPlaybackMute(ctx, d)
	endPlaybackMute(ctx, d)
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if len(controller.actions) != 2 || controller.actions[0] != tts.ActionRecordingStart || controller.actions[1] != tts.ActionRecordingEnd {
		t.Fatalf("recording epoch actions = %v", controller.actions)
	}
}
