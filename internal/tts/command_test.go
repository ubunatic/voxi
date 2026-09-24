package tts

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"ubunatic.com/voxi/internal/deps"
)

func TestSayCommandArgumentsAndStdin(t *testing.T) {
	runtimeDir := t.TempDir()
	controller := &recordingController{}
	ctx, cancel := context.WithCancel(context.Background())
	server, err := StartServer(ctx, SocketPath(runtimeDir, 0), controller)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = server.Close()
	})
	d := deps.Dependencies{
		Getenv: func(key string) string {
			if key == "XDG_RUNTIME_DIR" {
				return runtimeDir
			}
			return ""
		},
		Stdin:  strings.NewReader("First paragraph.\n\nSecond paragraph."),
		Stdout: &bytes.Buffer{},
	}
	cmd := NewSayCommand(d)
	cmd.SetArgs([]string{"Voxi", "reads", "this."})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("say with arguments: %v", err)
	}
	if got := d.Stdout.(*bytes.Buffer).String(); got != "queued 1 TTS chunk(s)\n" {
		t.Fatalf("argument output = %q", got)
	}
	if got := controller.text(); got != "Voxi reads this." {
		t.Fatalf("argument text = %q", got)
	}

	d.Stdout = &bytes.Buffer{}
	cmd = NewSayCommand(d)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("say from stdin: %v", err)
	}
	if got := controller.text(); got != "First paragraph.\n\nSecond paragraph." {
		t.Fatalf("stdin text = %q", got)
	}
	if got := d.Stdout.(*bytes.Buffer).String(); got != "queued 2 TTS chunk(s)\n" {
		t.Fatalf("stdin output = %q", got)
	}
}

func TestSayCommandReportsNoMonitor(t *testing.T) {
	d := deps.Dependencies{
		Getenv: func(string) string { return t.TempDir() },
		Stdin:  strings.NewReader("hello"),
		Stdout: &bytes.Buffer{},
	}
	err := NewSayCommand(d).Execute()
	if err == nil || !strings.Contains(err.Error(), "no monitor") {
		t.Fatalf("say without monitor error = %v, want clear no-monitor error", err)
	}
}

func (c *recordingController) text() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastText
}
