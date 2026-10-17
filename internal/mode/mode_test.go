package mode

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

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
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

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

func TestDescribeVoiceInputMode(t *testing.T) {
	cases := []struct {
		mode VoiceInputMode
		want string
	}{
		{ModeBatch, "batch (voxtype.service active, base.en whisper, typed at end of utterance)"},
		{ModeStreaming, "streaming (voxtype-streaming.service active, local Parakeet, typed incrementally)"},
		{ModeEager, "eager (voxi-eager.service active, continuous sentence streaming via local whisper)"},
		{ModeNeither, "neither (all services stopped)"},
		{ModeInconsistent, "inconsistent (multiple voice-input services active; stop others manually)"},
		{"custom", "custom"},
	}

	for _, tc := range cases {
		t.Run(string(tc.mode), func(t *testing.T) {
			got := DescribeVoiceInputMode(tc.mode)
			if got != tc.want {
				t.Fatalf("DescribeVoiceInputMode(%v) = %q, want %q", tc.mode, got, tc.want)
			}
		})
	}
}

func TestCurrentVoiceInputModeInconsistentAndNil(t *testing.T) {
	t.Run("nil Run returns neither", func(t *testing.T) {
		d := deps.Dependencies{}
		if got := CurrentVoiceInputMode(context.Background(), d); got != ModeNeither {
			t.Fatalf("got %v, want %v", got, ModeNeither)
		}
	})

	t.Run("multiple active returns inconsistent", func(t *testing.T) {
		d := deps.Dependencies{
			Run: func(ctx context.Context, name string, args ...string) error {
				if len(args) >= 4 && (args[3] == BatchService || args[3] == StreamingService) {
					return nil
				}
				return errors.New("inactive")
			},
		}
		if got := CurrentVoiceInputMode(context.Background(), d); got != ModeInconsistent {
			t.Fatalf("got %v, want %v", got, ModeInconsistent)
		}
	})
}

func TestCheckStreamingPreconditions(t *testing.T) {
	t.Run("missing HOME", func(t *testing.T) {
		d := deps.Dependencies{
			Getenv: func(k string) string { return "" },
		}
		if err := checkStreamingPreconditions(d); err == nil || !strings.Contains(err.Error(), "cannot resolve $HOME") {
			t.Fatalf("expected HOME error, got: %v", err)
		}
	})

	t.Run("missing config", func(t *testing.T) {
		home := t.TempDir()
		d := deps.Dependencies{
			Getenv: func(k string) string { return home },
			Stat:   os.Stat,
		}
		if err := checkStreamingPreconditions(d); err == nil || !strings.Contains(err.Error(), "streaming config missing") {
			t.Fatalf("expected missing config error, got: %v", err)
		}
	})

	t.Run("missing model", func(t *testing.T) {
		home := t.TempDir()
		configFile := filepath.Join(home, streamingConfigRelPath)
		_ = os.MkdirAll(filepath.Dir(configFile), 0o755)
		_ = os.WriteFile(configFile, []byte("# config"), 0o644)

		d := deps.Dependencies{
			Getenv: func(k string) string { return home },
			Stat:   os.Stat,
		}
		if err := checkStreamingPreconditions(d); err == nil || !strings.Contains(err.Error(), "streaming model missing") {
			t.Fatalf("expected missing model error, got: %v", err)
		}
	})

	t.Run("model is a file not a directory", func(t *testing.T) {
		home := t.TempDir()
		configFile := filepath.Join(home, streamingConfigRelPath)
		_ = os.MkdirAll(filepath.Dir(configFile), 0o755)
		_ = os.WriteFile(configFile, []byte("# config"), 0o644)

		modelFile := filepath.Join(home, streamingModelRelPath)
		_ = os.MkdirAll(filepath.Dir(modelFile), 0o755)
		_ = os.WriteFile(modelFile, []byte("file"), 0o644)

		d := deps.Dependencies{
			Getenv: func(k string) string { return home },
			Stat:   os.Stat,
		}
		if err := checkStreamingPreconditions(d); err == nil || !strings.Contains(err.Error(), "not a directory") {
			t.Fatalf("expected not a directory error, got: %v", err)
		}
	})

	t.Run("preconditions valid", func(t *testing.T) {
		home := t.TempDir()
		configFile := filepath.Join(home, streamingConfigRelPath)
		_ = os.MkdirAll(filepath.Dir(configFile), 0o755)
		_ = os.WriteFile(configFile, []byte("# config"), 0o644)

		modelDir := filepath.Join(home, streamingModelRelPath)
		_ = os.MkdirAll(modelDir, 0o755)

		d := deps.Dependencies{
			Getenv: func(k string) string { return home },
			Stat:   os.Stat,
		}
		if err := checkStreamingPreconditions(d); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestEnsureEagerServiceUnit(t *testing.T) {
	t.Run("missing HOME", func(t *testing.T) {
		d := deps.Dependencies{
			Getenv: func(k string) string { return "" },
		}
		if err := ensureEagerServiceUnit(context.Background(), d); err == nil {
			t.Fatal("expected error on missing HOME")
		}
	})

	t.Run("creates unit and reloads daemon", func(t *testing.T) {
		home := t.TempDir()
		var reloaded bool
		d := deps.Dependencies{
			Getenv: func(k string) string { return home },
			Stat:   os.Stat,
			Run: func(ctx context.Context, name string, args ...string) error {
				if name == "systemctl" && strings.Join(args, " ") == "--user daemon-reload" {
					reloaded = true
				}
				return nil
			},
		}

		if err := ensureEagerServiceUnit(context.Background(), d); err != nil {
			t.Fatalf("ensureEagerServiceUnit error: %v", err)
		}
		if !reloaded {
			t.Fatal("expected daemon-reload to be called")
		}

		unitFile := filepath.Join(home, ".config/systemd/user", EagerService)
		content, err := os.ReadFile(unitFile)
		if err != nil {
			t.Fatalf("read unit file error: %v", err)
		}
		if !strings.Contains(string(content), "Description=Voxi Continuous Eager Sentence Streaming Dictation") {
			t.Fatalf("unexpected unit file content: %s", string(content))
		}

		// Calling it again when file exists does not re-create or reload
		reloaded = false
		if err := ensureEagerServiceUnit(context.Background(), d); err != nil {
			t.Fatalf("second call error: %v", err)
		}
		if reloaded {
			t.Fatal("expected daemon-reload to not be called on existing file")
		}
	})
}

func TestSwitchVoiceInputModeBatchSuccess(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	var out bytes.Buffer
	activeService := StreamingService
	d := deps.Dependencies{
		Getenv: func(k string) string { return "/home/test" },
		Stdout: &out,
		Stat: func(name string) (os.FileInfo, error) {
			return nil, nil
		},
		Sleep: func(d time.Duration) {},
		Run: func(ctx context.Context, name string, args ...string) error {
			cmdStr := name + " " + strings.Join(args, " ")
			switch cmdStr {
			case "systemctl --user is-active --quiet " + BatchService:
				if activeService == BatchService {
					return nil
				}
				return errors.New("inactive")
			case "systemctl --user is-active --quiet " + StreamingService:
				if activeService == StreamingService {
					return nil
				}
				return errors.New("inactive")
			case "systemctl --user stop " + StreamingService:
				activeService = ""
				return nil
			case "systemctl --user start " + BatchService:
				activeService = BatchService
				return nil
			default:
				return nil
			}
		},
	}

	err := SwitchVoiceInputMode(context.Background(), d, ModeBatch)
	if err != nil {
		t.Fatalf("SwitchVoiceInputMode failed: %v", err)
	}
	if !strings.Contains(out.String(), "switched voice input to batch") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestSwitchVoiceInputModeAlreadyActiveInconsistent(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	var out bytes.Buffer
	d := deps.Dependencies{
		Getenv: func(k string) string { return "/home/test" },
		Stdout: &out,
		Stat: func(name string) (os.FileInfo, error) {
			return nil, nil
		},
		Run: func(ctx context.Context, name string, args ...string) error {
			cmdStr := name + " " + strings.Join(args, " ")
			if cmdStr == "systemctl --user is-active --quiet "+BatchService || cmdStr == "systemctl --user is-active --quiet "+StreamingService {
				return nil
			}
			return errors.New("inactive")
		},
	}

	err := SwitchVoiceInputMode(context.Background(), d, ModeBatch)
	if err == nil || !strings.Contains(err.Error(), "stop it manually") {
		t.Fatalf("expected inconsistent error, got: %v", err)
	}
}

func TestSwitchVoiceInputModeStartFailsRollback(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	var out bytes.Buffer
	var restored bool
	d := deps.Dependencies{
		Getenv: func(k string) string { return "/home/test" },
		Stdout: &out,
		Stat: func(name string) (os.FileInfo, error) {
			return nil, nil
		},
		Run: func(ctx context.Context, name string, args ...string) error {
			cmdStr := name + " " + strings.Join(args, " ")
			switch cmdStr {
			case "systemctl --user is-active --quiet " + StreamingService:
				return nil
			case "systemctl --user is-active --quiet " + BatchService:
				return errors.New("inactive")
			case "systemctl --user is-active --quiet " + EagerService,
				"systemctl --user is-active --quiet " + LegacyEagerService:
				return errors.New("inactive")
			case "systemctl --user start " + BatchService:
				return errors.New("failed to start unit")
			case "systemctl --user start " + StreamingService:
				restored = true
				return nil
			default:
				return nil
			}
		},
	}

	err := SwitchVoiceInputMode(context.Background(), d, ModeBatch)
	if err == nil || !strings.Contains(err.Error(), "start voxtype.service") {
		t.Fatalf("expected start error, got: %v", err)
	}
	if !restored {
		t.Fatal("expected StreamingService to be restored on start failure")
	}
}

func TestSwitchVoiceInputModeWaitTimeout(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	var out bytes.Buffer
	var restored bool
	d := deps.Dependencies{
		Getenv: func(k string) string { return "/home/test" },
		Stdout: &out,
		Stat: func(name string) (os.FileInfo, error) {
			return nil, nil
		},
		Sleep: func(d time.Duration) {},
		Run: func(ctx context.Context, name string, args ...string) error {
			cmdStr := name + " " + strings.Join(args, " ")
			switch cmdStr {
			case "systemctl --user is-active --quiet " + StreamingService:
				return nil
			case "systemctl --user is-active --quiet " + BatchService:
				return errors.New("not active yet")
			case "systemctl --user is-active --quiet " + EagerService,
				"systemctl --user is-active --quiet " + LegacyEagerService:
				return errors.New("inactive")
			case "systemctl --user start " + StreamingService:
				restored = true
				return nil
			default:
				return nil
			}
		},
	}

	err := SwitchVoiceInputMode(context.Background(), d, ModeBatch)
	if err == nil || !strings.Contains(err.Error(), "did not become active within 10s") {
		t.Fatalf("expected timeout error, got: %v", err)
	}
	if !restored {
		t.Fatal("expected previous service to be restored on wait timeout")
	}
}
