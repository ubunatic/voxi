package notify

import (
	"context"
	"testing"

	"ubunatic.com/voxi/internal/deps"
)

// TestPlayTypingPausedInvokesAvailablePlayerWithEmbeddedClip verifies that
// PlayTypingPaused writes the embedded WAV to a temp file and invokes
// whatever local player chunks.PlayerCommand picks -- without needing a real
// audio device.
func TestPlayTypingPausedInvokesAvailablePlayerWithEmbeddedClip(t *testing.T) {
	var gotName string
	var gotArgs []string
	d := deps.Dependencies{
		LookPath: func(name string) (string, error) {
			if name == "aplay" {
				return "/usr/bin/aplay", nil
			}
			return "", context.DeadlineExceeded
		},
		Run: func(ctx context.Context, name string, args ...string) error {
			gotName = name
			gotArgs = args
			return nil
		},
	}

	if err := PlayTypingPaused(context.Background(), d); err != nil {
		t.Fatalf("PlayTypingPaused() error = %v", err)
	}
	if gotName != "aplay" {
		t.Fatalf("player invoked = %q, want %q", gotName, "aplay")
	}
	if len(gotArgs) != 1 || gotArgs[0] == "" {
		t.Fatalf("player args = %v, want exactly one non-empty wav path", gotArgs)
	}
}

// TestPlayTypingPausedErrorsWithNoPlayer verifies a missing player surfaces
// as an error rather than a silent no-op.
func TestPlayTypingPausedErrorsWithNoPlayer(t *testing.T) {
	d := deps.Dependencies{
		LookPath: func(string) (string, error) { return "", context.DeadlineExceeded },
		Run:      func(context.Context, string, ...string) error { return nil },
	}
	if err := PlayTypingPaused(context.Background(), d); err == nil {
		t.Fatal("PlayTypingPaused() error = nil, want an error when no player is available")
	}
}
