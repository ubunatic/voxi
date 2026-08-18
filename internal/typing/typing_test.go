package typing

import (
	"context"
	"errors"
	"strings"
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
	d := deps.Dependencies{
		Getenv: func(k string) string { return "" },
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
