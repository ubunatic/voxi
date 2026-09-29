package devsample

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ubunatic.com/voxi/internal/deps"
)

func envDeps(env map[string]string) deps.Dependencies {
	return deps.Dependencies{Getenv: func(k string) string { return env[k] }}
}

func TestResolveEditorPrecedence(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"none", map[string]string{}, ""},
		{"editor only", map[string]string{"EDITOR": "nvim"}, "nvim"},
		{"visual wins", map[string]string{"VISUAL": "code -w", "EDITOR": "nvim"}, "code -w"},
		{"off switch", map[string]string{"EDITOR": "nvim", "VOXI_SAMPLE_EDITOR": "off"}, ""},
	}
	for _, c := range cases {
		if got := resolveEditor(envDeps(c.env)); got != c.want {
			t.Errorf("%s: resolveEditor = %q, want %q", c.name, got, c.want)
		}
	}
	if got := resolveEditor(deps.Dependencies{}); got != "" {
		t.Errorf("nil Getenv: resolveEditor = %q, want empty", got)
	}
}

func TestStripCommentLines(t *testing.T) {
	got := stripCommentLines("# header\nhello there\n  # note\nsecond\tline\n")
	if want := "hello there second line"; got != want {
		t.Errorf("stripCommentLines = %q, want %q", got, want)
	}
}

func TestEditTranscriptInEditorRunsEditorOnTempFile(t *testing.T) {
	script := filepath.Join(t.TempDir(), "fake-editor.sh")
	body := "#!/bin/sh\ngrep -q 'asr guess' \"$1\" || exit 3\nprintf '# c\\ncorrected text\\n' > \"$1\"\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := editTranscriptInEditor(context.Background(), script, "asr guess")
	if err != nil {
		t.Fatalf("editTranscriptInEditor: %v", err)
	}
	if got != "corrected text" {
		t.Errorf("got %q, want %q", got, "corrected text")
	}
	if _, err := editTranscriptInEditor(context.Background(), "false", "x"); err == nil {
		t.Error("expected error from failing editor")
	}
}

func TestPromptTranscriptFallsBackToInline(t *testing.T) {
	old := editTranscriptFn
	defer func() { editTranscriptFn = old }()
	editTranscriptFn = func(context.Context, string, string) (string, error) {
		t.Fatal("editor must not run without a terminal")
		return "", nil
	}
	d := envDeps(map[string]string{"EDITOR": "nvim"}) // Stdin/Stdout unset: not a terminal
	got, err := promptTranscript(context.Background(), d, "raw", func() (string, error) { return "inline", nil })
	if err != nil || got != "inline" {
		t.Errorf("promptTranscript = %q, %v; want inline", got, err)
	}
}
