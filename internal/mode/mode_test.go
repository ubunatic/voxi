package mode

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"ubunatic.com/voxi/internal/deps"
)

func TestCurrentVoiceInputMode(t *testing.T) {
	cases := []struct {
		name     string
		active   string
		expected VoiceInputMode
	}{
		{"batch active", BatchService, ModeBatch},
		{"streaming active", StreamingService, ModeStreaming},
		{"eager active", EagerService, ModeEager},
		{"legacy eager active", LegacyEagerService, ModeEager},
		{"none active", "", ModeNeither},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := deps.Dependencies{
				Run: func(ctx context.Context, name string, args ...string) error {
					if len(args) >= 4 && args[3] == tc.active {
						return nil
					}
					return errors.New("inactive")
				},
			}

			mode := CurrentVoiceInputMode(context.Background(), d)
			if mode != tc.expected {
				t.Fatalf("expected mode %v, got %v", tc.expected, mode)
			}
		})
	}
}

func TestSwitchVoiceInputModeValidation(t *testing.T) {
	var out bytes.Buffer
	d := deps.Dependencies{
		Getenv: func(k string) string { return "/home/test" },
		Stdout: &out,
		Run: func(ctx context.Context, name string, args ...string) error {
			return errors.New("inactive")
		},
	}

	err := SwitchVoiceInputMode(context.Background(), d, "invalid-mode")
	if err == nil || !strings.Contains(err.Error(), "invalid voice-input mode") {
		t.Fatalf("expected error on invalid mode, got: %v", err)
	}
}

func TestSwitchVoiceInputModePersistsAlreadyActiveEager(t *testing.T) {
	var out bytes.Buffer
	var calls []string
	d := deps.Dependencies{
		Getenv: func(k string) string { return "/home/test" },
		Stdout: &out,
		Stat: func(name string) (os.FileInfo, error) {
			return nil, nil
		},
		Run: func(ctx context.Context, name string, args ...string) error {
			call := name + " " + strings.Join(args, " ")
			calls = append(calls, call)
			if call == "systemctl --user is-active --quiet "+EagerService {
				return nil
			}
			if strings.HasPrefix(call, "systemctl --user is-active --quiet ") {
				return errors.New("inactive")
			}
			return nil
		},
	}

	err := SwitchVoiceInputMode(context.Background(), d, ModeEager)
	if err != nil {
		t.Fatalf("SwitchVoiceInputMode eager returned error: %v", err)
	}

	wantCalls := []string{
		"systemctl --user enable " + EagerService,
		"systemctl --user disable " + BatchService,
		"systemctl --user disable " + StreamingService,
		"systemctl --user disable " + LegacyEagerService,
	}
	for _, want := range wantCalls {
		if !hasCall(calls, want) {
			t.Fatalf("missing call %q in %#v", want, calls)
		}
	}
}

func hasCall(calls []string, want string) bool {
	for _, call := range calls {
		if call == want {
			return true
		}
	}
	return false
}
