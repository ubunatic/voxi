package clone

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ubunatic.com/voxi/internal/deps"
)

func TestEnsureCheckpointDownloadsAndCachesURL(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, "checkpoint bytes")
	}))
	defer server.Close()
	home := t.TempDir()
	getenv := func(key string) string {
		if key == "XDG_CACHE_HOME" {
			return filepath.Join(home, "cache")
		}
		return ""
	}
	baseURL := server.URL + "/en_US/epoch%3D4.ckpt"
	first, err := ensureCheckpoint(context.Background(), baseURL, home, getenv)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ensureCheckpoint(context.Background(), baseURL, home, getenv)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || calls != 1 {
		t.Fatalf("cached checkpoint = %q, %q; downloads = %d", first, second, calls)
	}
	if got, err := os.ReadFile(first); err != nil || string(got) != "checkpoint bytes" {
		t.Fatalf("cached data = %q, %v", got, err)
	}
}

func TestEnsureCheckpointRejectsFailedAndEmptyDownloads(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
		body string
	}{
		{name: "http error", code: http.StatusNotFound},
		{name: "empty", code: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.code)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			_, err := ensureCheckpoint(context.Background(), server.URL+"/base.ckpt", t.TempDir(), nil)
			if err == nil {
				t.Fatal("ensureCheckpoint unexpectedly succeeded")
			}
		})
	}
}

func TestTrainCommandPreparesRunsUVAndInstallsVoice(t *testing.T) {
	home := t.TempDir()
	samples := filepath.Join(home, "samples")
	if err := os.MkdirAll(samples, 0700); err != nil {
		t.Fatal(err)
	}
	writeCorpus(t, samples, "sample\ta.wav\tText.\t\n")
	writeFile(t, filepath.Join(samples, "a.wav"), []byte("source"))
	base := filepath.Join(home, "base.ckpt")
	writeFile(t, base, []byte("base checkpoint"))
	var stdout strings.Builder
	var invocations []string
	run := func(_ context.Context, executable string, args ...string) error {
		if executable == "/usr/bin/ffmpeg" {
			return os.WriteFile(args[len(args)-1], wavBytes(SampleRate, channels, bits), 0600)
		}
		if executable != "/usr/bin/uv" {
			t.Fatalf("unexpected command %q %v", executable, args)
		}
		invocations = append(invocations, strings.Join(args, " "))
		if args[0] == "sync" {
			return nil
		}
		if args[0] != "run" {
			t.Fatalf("unexpected uv invocation: %v", args)
		}
		output := flagValue(args, "--output-dir")
		if got := flagValue(args, "--base"); got != base {
			t.Fatalf("base checkpoint arg = %q, want %q", got, base)
		}
		if got := flagValue(args, "--epochs"); got != "7" {
			t.Fatalf("epochs arg = %q, want 7", got)
		}
		if got := flagValue(args, "--batch-size"); got != "4" {
			t.Fatalf("batch size arg = %q, want 4", got)
		}
		if got := flagValue(args, "--accelerator"); got != "gpu" {
			t.Fatalf("accelerator arg = %q, want gpu", got)
		}
		if err := os.MkdirAll(output, 0700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(output, "my-voice.onnx"), []byte("fake ONNX"), 0600); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(output, "my-voice.onnx.json"), []byte(`{"audio":{"sample_rate":22050}}`), 0600)
	}
	d := deps.Dependencies{
		Getenv: func(key string) string {
			if key == "HOME" {
				return home
			}
			return ""
		},
		LookPath: func(name string) (string, error) {
			switch name {
			case "ffmpeg":
				return "/usr/bin/ffmpeg", nil
			case "uv":
				return "/usr/bin/uv", nil
			default:
				return "", fmt.Errorf("missing %s", name)
			}
		},
		Stdout: &stdout,
	}
	cmd := newTrainCommand(d, run)
	cmd.SetArgs([]string{"--name", "my-voice", "--samples-dir", samples, "--base", base, "--epochs", "7", "--batch-size", "4", "--accelerator", "gpu"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(invocations) != 2 || !strings.HasPrefix(invocations[0], "sync ") || !strings.Contains(invocations[1], "runner.py") {
		t.Fatalf("uv invocations = %v", invocations)
	}
	voiceDir := filepath.Join(home, ".local", "share", "voxi", "voices")
	model, err := os.ReadFile(filepath.Join(voiceDir, "my-voice.onnx"))
	if err != nil || string(model) != "fake ONNX" {
		t.Fatalf("installed model = %q, %v", model, err)
	}
	config, err := os.ReadFile(filepath.Join(voiceDir, "my-voice.onnx.json"))
	if err != nil || !json.Valid(config) {
		t.Fatalf("installed config = %q, %v", config, err)
	}
	if !strings.Contains(stdout.String(), "installed Piper voice my-voice (1 sample(s))") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestTrainCommandValidatesOptionsBeforeStartingProcesses(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "bad voice name", args: []string{"--name", "../bad"}, want: "invalid voice name"},
		{name: "zero epochs", args: []string{"--name", "voice", "--epochs", "0"}, want: "epochs and batch size must be positive"},
		{name: "bad accelerator", args: []string{"--name", "voice", "--accelerator", "tpu"}, want: "unsupported accelerator"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			d := deps.Dependencies{Getenv: func(key string) string {
				if key == "HOME" {
					return t.TempDir()
				}
				return ""
			}}
			cmd := newTrainCommand(d, func(context.Context, string, ...string) error { called = true; return nil })
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Execute error = %v, want %q", err, tc.want)
			}
			if called {
				t.Fatal("invalid options launched a process")
			}
		})
	}
}

func TestInstallVoiceRejectsInvalidConfigWithoutReplacingExistingPair(t *testing.T) {
	root := t.TempDir()
	modelSource := filepath.Join(root, "new.onnx")
	configSource := filepath.Join(root, "new.onnx.json")
	writeFile(t, modelSource, []byte("new model"))
	writeFile(t, configSource, []byte("not json"))
	voiceDir := filepath.Join(root, "voices")
	if err := os.MkdirAll(voiceDir, 0700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(voiceDir, "voice.onnx"), []byte("old model"))
	writeFile(t, filepath.Join(voiceDir, "voice.onnx.json"), []byte(`{"old":true}`))
	if err := installVoice(modelSource, configSource, voiceDir, "voice"); err == nil {
		t.Fatal("installVoice accepted invalid JSON")
	}
	if got, err := os.ReadFile(filepath.Join(voiceDir, "voice.onnx")); err != nil || string(got) != "old model" {
		t.Fatalf("existing model = %q, %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(voiceDir, "voice.onnx.json")); err != nil || string(got) != `{"old":true}` {
		t.Fatalf("existing config = %q, %v", got, err)
	}
}

func flagValue(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}
