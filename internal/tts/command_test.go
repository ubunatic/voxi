package tts

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	cmd.SetArgs([]string{"--no-llm", "Voxi", "reads", "this."})
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
	cmd.SetArgs([]string{"--no-llm"})
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
	home := t.TempDir()
	runtimeDir := t.TempDir()
	d := deps.Dependencies{
		Getenv: func(key string) string {
			switch key {
			case "HOME":
				return home
			case "XDG_RUNTIME_DIR":
				return runtimeDir
			}
			return ""
		},
		Stdin:  strings.NewReader("hello"),
		Stdout: &bytes.Buffer{},
	}
	cmd := NewSayCommand(d)
	cmd.SetArgs([]string{"--no-llm"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "no monitor") {
		t.Fatalf("say without monitor error = %v, want clear no-monitor error", err)
	}
}

func TestSayOfflineOutputDoesNotDialDaemon(t *testing.T) {
	home := t.TempDir()
	outPath := filepath.Join(t.TempDir(), "speech.wav")
	d := deps.Dependencies{
		Getenv: func(key string) string {
			if key == "HOME" {
				return home
			}
			return t.TempDir()
		},
		LookPath: func(string) (string, error) { return "", os.ErrNotExist },
		Stdin:    strings.NewReader(""), Stdout: &bytes.Buffer{},
	}
	cmd := NewSayCommand(d)
	cmd.SetArgs([]string{"--no-play", "-o", outPath, "offline synthesis"})
	err := cmd.Execute()
	if err == nil || strings.Contains(err.Error(), "no monitor") {
		t.Fatalf("offline synthesis error = %v; expected local engine error without socket dial", err)
	}
	if _, statErr := os.Stat(outPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("output unexpectedly exists or stat failed: %v", statErr)
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
	cmd.SetArgs([]string{"--no-llm", "--interrupt", "--from", "primary"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if controller.got != "selection text" {
		t.Fatalf("replace text = %q", controller.got)
	}
}

func TestSayCommandUsesLLMByDefaultAndContinuesSession(t *testing.T) {
	runtimeDir := t.TempDir()
	home := t.TempDir()
	controller := &recordingController{}
	ctx, cancel := context.WithCancel(context.Background())
	server, err := StartServer(ctx, SocketPath(runtimeDir, 0), controller)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = server.Close() })
	var session string
	calls := 0
	d := deps.Dependencies{
		Getenv: func(key string) string {
			if key == "XDG_RUNTIME_DIR" {
				return runtimeDir
			}
			if key == "HOME" {
				return home
			}
			return ""
		},
		RunStdinOutput: func(_ context.Context, input, name string, args ...string) (string, error) {
			calls++
			if name != "lmcoder" {
				t.Fatalf("command = %q, want lmcoder", name)
			}
			joined := strings.Join(args, " ")
			if !strings.Contains(joined, "--host localhost") {
				t.Fatalf("lmcoder args = %q, want spec fallback host", joined)
			}
			var gotSession string
			for i := 0; i+1 < len(args); i++ {
				if args[i] == "--session" {
					gotSession = args[i+1]
				}
			}
			if gotSession == "" || (session != "" && session != gotSession) {
				t.Fatalf("session = %q, prior = %q", gotSession, session)
			}
			session = gotSession
			if calls == 1 && !strings.Contains(input, "first paragraph") {
				t.Fatalf("first prompt = %q", input)
			}
			if calls == 2 && !strings.Contains(input, "second paragraph") {
				t.Fatalf("continuation prompt = %q", input)
			}
			return fmt.Sprintf("spoken %d", calls), nil
		},
		Stdin: strings.NewReader("first paragraph\n\nsecond paragraph"), Stdout: &bytes.Buffer{},
	}
	cmd := NewSayCommand(d)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("lmcoder calls = %d, want 2", calls)
	}
	if got := controller.text(); got != "spoken 2" {
		t.Fatalf("last queued narration = %q", got)
	}
}

func TestSayCommandLLMHostFlagOverridesConfig(t *testing.T) {
	configHome := t.TempDir()
	runtimeDir := t.TempDir()
	controller := &recordingController{}
	ctx, cancel := context.WithCancel(context.Background())
	server, err := StartServer(ctx, SocketPath(runtimeDir, 0), controller)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = server.Close() })
	if err := os.MkdirAll(filepath.Join(configHome, ".config", "voxi"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configHome, ".config", "voxi", "config.yaml"), []byte("tts_llm_host: configured-host\n"), 0600); err != nil {
		t.Fatal(err)
	}
	d := deps.Dependencies{
		Getenv: func(key string) string {
			if key == "HOME" {
				return configHome
			}
			if key == "XDG_RUNTIME_DIR" {
				return runtimeDir
			}
			return ""
		},
		Stdin: strings.NewReader("Read me."), Stdout: &bytes.Buffer{},
		RunStdinOutput: func(_ context.Context, _ string, _ string, args ...string) (string, error) {
			if !strings.Contains(strings.Join(args, " "), "--host x600") {
				t.Fatalf("args = %v, want x600 override", args)
			}
			return "Narration.", nil
		},
	}
	cmd := NewSayCommand(d)
	cmd.SetArgs([]string{"--llm", "x600", "Read", "me."})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := controller.text(); got != "Narration." {
		t.Fatalf("queued narration = %q", got)
	}
}

func TestSayCommandFallsBackForRemainingTextWhenContinuationFails(t *testing.T) {
	runtimeDir := t.TempDir()
	controller := &recordingController{}
	ctx, cancel := context.WithCancel(context.Background())
	server, err := StartServer(ctx, SocketPath(runtimeDir, 0), controller)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = server.Close() })
	calls := 0
	d := deps.Dependencies{
		Getenv: func(key string) string {
			if key == "XDG_RUNTIME_DIR" {
				return runtimeDir
			}
			if key == "HOME" {
				return t.TempDir()
			}
			return ""
		},
		RunStdinOutput: func(context.Context, string, string, ...string) (string, error) {
			calls++
			if calls == 1 {
				return "First spoken paragraph.", nil
			}
			return "", errors.New("continuation failed")
		},
		Stdin: strings.NewReader("First original paragraph.\n\nSecond original paragraph."), Stdout: &bytes.Buffer{},
	}
	if err := NewSayCommand(d).Execute(); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("lmcoder calls = %d, want 2", calls)
	}
	if got := controller.text(); got != "Second original paragraph." {
		t.Fatalf("fallback remaining text = %q", got)
	}
}

func TestSayCommandRejectsEmptyClipboard(t *testing.T) {
	d := deps.Dependencies{
		Getenv: func(key string) string {
			if key == "HOME" {
				return t.TempDir()
			}
			return ""
		},
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
