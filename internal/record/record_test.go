package record

import (
	"context"
	"errors"
	"testing"

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
