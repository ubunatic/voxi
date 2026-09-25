package tts

import (
	"bytes"
	"context"
	"errors"
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

func TestSayCommandInterruptFromPrimaryRunsWlPasteAndReplaces(t *testing.T) {
	runtimeDir := t.TempDir()
	controller := &recordingReplaceController{}
	ctx, cancel := context.WithCancel(context.Background())
	server, err := StartServer(ctx, SocketPath(runtimeDir, 0), controller)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = server.Close() })
	d := deps.Dependencies{
		Getenv: func(key string) string {
			if key == "XDG_RUNTIME_DIR" {
				return runtimeDir
			}
			return ""
		},
		LookPath: func(string) (string, error) { return "/usr/bin/wl-paste", nil },
		RunOutput: func(_ context.Context, name string, args ...string) (string, error) {
			if name != "wl-paste" || strings.Join(args, " ") != "--primary" {
				t.Fatalf("wl-paste invocation: %s %v", name, args)
			}
			return "selection text", nil
		},
		Stdin: strings.NewReader(""), Stdout: &bytes.Buffer{},
	}
	cmd := NewSayCommand(d)
	cmd.SetArgs([]string{"--interrupt", "--from", "primary"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if controller.got != "selection text" {
		t.Fatalf("replace text = %q", controller.got)
	}
}

func TestSayCommandRejectsEmptyClipboard(t *testing.T) {
	d := deps.Dependencies{
		Getenv:    func(string) string { return t.TempDir() },
		LookPath:  func(string) (string, error) { return "/usr/bin/wl-paste", nil },
		RunOutput: func(context.Context, string, ...string) (string, error) { return " \n", nil },
		Stdin:     strings.NewReader(""), Stdout: &bytes.Buffer{},
	}
	cmd := NewSayCommand(d)
	cmd.SetArgs([]string{"--from", "clipboard"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("error = %v", err)
	}
}

func TestReportCommandErrorNotifiesWhenAvailable(t *testing.T) {
	var args []string
	d := deps.Dependencies{
		Stderr:   &bytes.Buffer{},
		LookPath: func(string) (string, error) { return "/usr/bin/notify-send", nil },
		Run: func(_ context.Context, name string, got ...string) error {
			args = append([]string{name}, got...)
			return nil
		},
	}
	ReportCommandError(d, errors.New("empty selection"))
	if strings.Join(args, " ") != "/usr/bin/notify-send Voxi empty selection" {
		t.Fatalf("notify args = %v", args)
	}
}

type recordingReplaceController struct{ got string }

func (c *recordingReplaceController) Enqueue(string) (int, error)      { return 0, nil }
func (c *recordingReplaceController) Control(Action) error             { return nil }
func (c *recordingReplaceController) Replace(text string) (int, error) { c.got = text; return 1, nil }

func (c *recordingController) text() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastText
}
