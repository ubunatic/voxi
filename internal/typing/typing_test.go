package typing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestCopyText(t *testing.T) {
	var copied string
	d := deps.Dependencies{
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
