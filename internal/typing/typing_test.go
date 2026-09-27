package typing

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	"ubunatic.com/voxi/internal/deps"
)

func TestBuildDotoolCommands(t *testing.T) {
	cmd1 := BuildDotoolCommands("Hello World", 0)
	if cmd1 != "type Hello World\n" {
		t.Fatalf("expected 'type Hello World\\n', got %q", cmd1)
	}

	cmd2 := BuildDotoolCommands("Line 1\nLine 2", 15)
	expected2 := "typedelay 15\ntypehold 15\ntype Line 1\nkey enter\ntype Line 2\n"
	if cmd2 != expected2 {
		t.Fatalf("expected %q, got %q", expected2, cmd2)
	}
}

func TestTypeTextSynchronizesDotooldLayoutBeforeFIFO(t *testing.T) {
	for _, tc := range []struct {
		name, current string
		wantRestart   bool
	}{
		{"unchanged", "DOTOOL_XKB_LAYOUT=us DOTOOL_XKB_VARIANT=mac-iso", false},
		{"changed", "DOTOOL_XKB_LAYOUT=de DOTOOL_XKB_VARIANT=nodeadkeys", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pipe := filepath.Join(t.TempDir(), "dotool-pipe")
			if err := syscall.Mkfifo(pipe, 0600); err != nil {
				t.Fatal(err)
			}
			reader, err := os.OpenFile(pipe, os.O_RDONLY|syscall.O_NONBLOCK, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			var restarts atomic.Int32
			var wrote string
			d := deps.Dependencies{
				Getenv: func(key string) string {
					if key == "HOME" {
						return t.TempDir()
					}
					if key == "DOTOOL_PIPE" {
						return pipe
					}
					return ""
				},
				RunOutput: func(_ context.Context, name string, args ...string) (string, error) {
					if name == "ibus" {
						return "", errors.New("no ibus")
					}
					if name == "gsettings" {
						if args[len(args)-1] == "sources" {
							return "[('xkb', 'us+mac-iso')]", nil
						}
						return "uint32 0", nil
					}
					return tc.current, nil
				},
				MkdirAll:  os.MkdirAll,
				WriteFile: func(_ string, data []byte, _ os.FileMode) error { wrote = string(data); return nil },
				Run: func(_ context.Context, name string, args ...string) error {
					if len(args) > 0 && args[len(args)-1] == "dotoold.service" {
						restarts.Add(1)
					}
					return nil
				},
				RunStdin: func(_ context.Context, _ string, name string, _ ...string) error {
					if name != "dotoolc" {
						t.Fatalf("injector=%q, want dotoolc", name)
					}
					return nil
				},
			}
			if err := TypeText(context.Background(), d, "zebra"); err != nil {
				t.Fatal(err)
			}
			want := int32(0)
			if tc.wantRestart {
				want = 1
			}
			if got := restarts.Load(); got != want {
				t.Fatalf("restart count=%d, want %d", got, want)
			}
			if tc.wantRestart && (!strings.Contains(wrote, "DOTOOL_XKB_LAYOUT=us") || !strings.Contains(wrote, "DOTOOL_XKB_VARIANT=mac-iso")) {
				t.Fatalf("drop-in=%q", wrote)
			}
		})
	}
}

func TestTypeTextWaitsForRestartedFIFOBeforeWriting(t *testing.T) {
	pipe := filepath.Join(t.TempDir(), "dotool-pipe")
	if err := syscall.Mkfifo(pipe, 0600); err != nil {
		t.Fatal(err)
	}
	reader, err := os.OpenFile(pipe, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var restartedReader *os.File
	var order []string
	d := deps.Dependencies{
		Getenv: func(key string) string {
			if key == "HOME" {
				return t.TempDir()
			}
			if key == "DOTOOL_PIPE" {
				return pipe
			}
			return ""
		},
		RunOutput: func(_ context.Context, name string, args ...string) (string, error) {
			if name == "ibus" {
				return "", errors.New("no ibus")
			}
			if name == "gsettings" {
				if args[len(args)-1] == "sources" {
					return "[('xkb', 'us')]", nil
				}
				return "uint32 0", nil
			}
			return "DOTOOL_XKB_LAYOUT=de", nil
		},
		MkdirAll: os.MkdirAll, WriteFile: os.WriteFile,
		Run: func(_ context.Context, _ string, args ...string) error {
			if args[len(args)-1] == "dotoold.service" {
				order = append(order, "restart")
				if err := os.Remove(pipe); err != nil {
					return err
				}
				if err := syscall.Mkfifo(pipe, 0600); err != nil {
					return err
				}
				restartedReader, err = os.OpenFile(pipe, os.O_RDONLY|syscall.O_NONBLOCK, 0)
				return err
			}
			return nil
		},
		RunStdin: func(_ context.Context, _ string, name string, _ ...string) error {
			order = append(order, name)
			return nil
		},
	}
	if err := TypeText(context.Background(), d, "text"); err != nil {
		t.Fatal(err)
	}
	if restartedReader == nil {
		t.Fatal("restart did not create FIFO reader")
	}
	defer restartedReader.Close()
	if got := strings.Join(order, ","); got != "restart,dotoolc" {
		t.Fatalf("operation order=%q", got)
	}
}

func TestTypeTextDetectionFailureRestoresInstallLayoutAndWarns(t *testing.T) {
	home := t.TempDir()
	pipe := filepath.Join(t.TempDir(), "dotool-pipe")
	if err := syscall.Mkfifo(pipe, 0600); err != nil {
		t.Fatal(err)
	}
	reader, err := os.OpenFile(pipe, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	dropin := filepath.Join(home, ".config", "systemd", "user", "dotoold.service.d", "voxi-layout.conf")
	if err := os.MkdirAll(filepath.Dir(dropin), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dropin, []byte("[Service]\nEnvironment=DOTOOL_XKB_LAYOUT=us\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	var layout = "us"
	d := deps.Dependencies{
		Getenv: func(key string) string {
			if key == "HOME" {
				return home
			}
			if key == "DOTOOL_PIPE" {
				return pipe
			}
			return ""
		},
		RunOutput: func(_ context.Context, name string, _ ...string) (string, error) {
			if name == "ibus" || name == "gsettings" {
				return "", errors.New("source unavailable")
			}
			return "DOTOOL_XKB_LAYOUT=" + layout, nil
		},
		Remove: os.Remove, Run: func(_ context.Context, _ string, args ...string) error {
			if args[len(args)-1] == "dotoold.service" {
				layout = "de"
			}
			return nil
		},
		RunStdin: func(_ context.Context, _ string, name string, _ ...string) error {
			if name != "dotoolc" {
				t.Fatalf("injector=%q", name)
			}
			return nil
		},
		Stderr: &stderr,
	}
	if err := TypeText(context.Background(), d, "fallback"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dropin); !os.IsNotExist(err) {
		t.Fatalf("layout drop-in remains: %v", err)
	}
	if !strings.Contains(stderr.String(), "restoring install-time dotoold layout") {
		t.Fatalf("warning=%q", stderr.String())
	}
	status := InspectLayout(context.Background(), d)
	if !status.Fallback || status.Dotoold != "de" {
		t.Fatalf("layout status=%+v", status)
	}
}

func TestInspectLayoutReportsActiveAndDaemonSources(t *testing.T) {
	d := deps.Dependencies{
		Getenv: func(key string) string {
			if key == "DOTOOL_PIPE" {
				return t.TempDir() + "/pipe"
			}
			return t.TempDir()
		},
		RunOutput: func(_ context.Context, name string, args ...string) (string, error) {
			if name == "ibus" {
				return "", errors.New("no ibus")
			}
			if name == "gsettings" {
				if args[len(args)-1] == "sources" {
					return "[('xkb', 'us+mac-iso')]", nil
				}
				return "uint32 0", nil
			}
			return "DOTOOL_XKB_LAYOUT=us DOTOOL_XKB_VARIANT=mac-iso", nil
		},
	}
	status := InspectLayout(context.Background(), d)
	if status.ActiveSource != "us+mac-iso" || status.Dotoold != "us+mac-iso" || status.Fallback {
		t.Fatalf("layout status=%+v", status)
	}
}

func TestTypeTextCachesActiveAndDaemonSourceReads(t *testing.T) {
	pipe := filepath.Join(t.TempDir(), "dotool-pipe")
	if err := syscall.Mkfifo(pipe, 0600); err != nil {
		t.Fatal(err)
	}
	reader, err := os.OpenFile(pipe, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	home := t.TempDir()
	var ibusCalls, gsettingsCalls, systemctlCalls int
	d := deps.Dependencies{
		Getenv: func(key string) string {
			if key == "HOME" {
				return home
			}
			if key == "DOTOOL_PIPE" {
				return pipe
			}
			return ""
		},
		RunOutput: func(_ context.Context, name string, args ...string) (string, error) {
			switch name {
			case "ibus":
				ibusCalls++
				return "", errors.New("no ibus")
			case "gsettings":
				gsettingsCalls++
				if args[len(args)-1] == "sources" {
					return "[('xkb', 'de+nodeadkeys')]", nil
				}
				return "uint32 0", nil
			default:
				systemctlCalls++
				return "DOTOOL_XKB_LAYOUT=de DOTOOL_XKB_VARIANT=nodeadkeys", nil
			}
		},
		RunStdin: func(context.Context, string, string, ...string) error { return nil },
	}
	if err := TypeText(context.Background(), d, "first"); err != nil {
		t.Fatal(err)
	}
	if err := TypeText(context.Background(), d, "second"); err != nil {
		t.Fatal(err)
	}
	if ibusCalls != 1 || gsettingsCalls != 2 || systemctlCalls != 1 {
		t.Fatalf("source command calls: ibus=%d gsettings=%d systemctl=%d", ibusCalls, gsettingsCalls, systemctlCalls)
	}
}

func TestCopyText(t *testing.T) {
	var copied string
	d := deps.Dependencies{
		Getenv: func(string) string { return "" },
		LookPath: func(name string) (string, error) {
			if name == "wl-copy" {
				return "/usr/bin/wl-copy", nil
			}
			return "", errors.New("not found")
		},
		RunStdin: func(ctx context.Context, stdin string, name string, args ...string) error {
			if name == "wl-copy" {
				copied = stdin
				return nil
			}
			return errors.New("wrong command")
		},
	}

	if err := CopyText(context.Background(), d, "Clipboard content"); err != nil {
		t.Fatalf("CopyText failed: %v", err)
	}
	if copied != "Clipboard content" {
		t.Fatalf("expected 'Clipboard content', got %q", copied)
	}
}

func TestTypeTextFallback(t *testing.T) {
	var typed string
	missingPipe := t.TempDir() + "/missing-pipe"
	d := deps.Dependencies{
		Getenv: func(k string) string {
			if k == "DOTOOL_PIPE" {
				return missingPipe
			}
			return ""
		},
		LookPath: func(name string) (string, error) {
			if name == "dotool" {
				return "/usr/bin/dotool", nil
			}
			return "", errors.New("not found")
		},
		RunStdin: func(ctx context.Context, stdin string, name string, args ...string) error {
			if name == "dotool" {
				typed = stdin
				return nil
			}
			return errors.New("wrong command")
		},
	}

	if err := TypeText(context.Background(), d, "Typing test"); err != nil {
		t.Fatalf("TypeText failed: %v", err)
	}
	if !strings.Contains(typed, "type Typing test\n") {
		t.Fatalf("expected 'type Typing test\\n', got %q", typed)
	}
}

func TestTypeTextPrefersPersistentFIFOOverStandaloneDotool(t *testing.T) {
	pipe := filepath.Join(t.TempDir(), "dotool-pipe")
	if err := syscall.Mkfifo(pipe, 0600); err != nil {
		t.Fatal(err)
	}
	reader, err := os.OpenFile(pipe, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	var command string
	d := deps.Dependencies{
		Getenv: func(key string) string {
			if key == "DOTOOL_PIPE" {
				return pipe
			}
			return ""
		},
		RunStdin: func(_ context.Context, _ string, name string, _ ...string) error {
			command = name
			return nil
		},
	}

	if err := TypeText(context.Background(), d, "persistent path"); err != nil {
		t.Fatalf("TypeText failed: %v", err)
	}
	if command != "dotoolc" {
		t.Fatalf("injector=%q, want dotoolc", command)
	}
}

func TestTypeTextCanceledBeforeInjector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	d := deps.Dependencies{Getenv: func(string) string { return "" }, RunStdin: func(context.Context, string, string, ...string) error { called = true; return nil }}
	if err := TypeText(ctx, d, "must not type"); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v, want canceled", err)
	}
	if called {
		t.Fatal("injector called for canceled operation")
	}
}

func TestTypeTextDoesNotReplayFailedFIFOAttempt(t *testing.T) {
	pipe := filepath.Join(t.TempDir(), "dotool-pipe")
	if err := syscall.Mkfifo(pipe, 0600); err != nil {
		t.Fatal(err)
	}
	reader, err := os.OpenFile(pipe, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var calls []string
	d := deps.Dependencies{
		Getenv: func(key string) string {
			if key == "DOTOOL_PIPE" {
				return pipe
			}
			return ""
		},
		RunStdin: func(_ context.Context, _ string, name string, _ ...string) error {
			calls = append(calls, name)
			return errors.New("partial write unknown")
		},
		LookPath: func(string) (string, error) { return "/fake/dotool", nil },
	}
	if err := TypeText(context.Background(), d, "only once"); err == nil || !strings.Contains(err.Error(), "dotoolc") {
		t.Fatalf("error=%v, want dotoolc failure", err)
	}
	if got := strings.Join(calls, ","); got != "dotoolc" {
		t.Fatalf("injector calls=%q, want one FIFO attempt", got)
	}
}
