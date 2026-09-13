package settings

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/deps"
)

func TestSettingsCommandDump(t *testing.T) {
	home := t.TempDir()
	custom := &config.UserSettings{
		LLMCleaner:       true,
		CleanupModel:     "qwen3-4b-instruct-2507-q4",
		OpenAIBaseURL:    "http://127.0.0.1:8734/v1",
		ASRModel:         "large-v3-turbo",
		TypeDelayMs:      5,
		DictationHistory: true,
		ModifierGating:   true,
	}
	if err := config.SaveUserSettings(home, custom); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	d := deps.DefaultDependencies(strings.NewReader(""), &stdout)
	d.Getenv = func(k string) string {
		if k == "HOME" {
			return home
		}
		return ""
	}

	cmd := NewCommand(d, []string{"large-v3-turbo"})
	cmd.SetArgs([]string{"--dump"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() failed: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "Voxi Configuration Summary:") {
		t.Errorf("expected summary title, got: %s", out)
	}
	if !strings.Contains(out, "LLM Cleaner:           [✓] Enabled") {
		t.Errorf("expected LLM Cleaner enabled, got: %s", out)
	}
	if !strings.Contains(out, "large-v3-turbo") {
		t.Errorf("expected large-v3-turbo, got: %s", out)
	}
}

func TestSettingsCommandJSON(t *testing.T) {
	home := t.TempDir()
	var stdout bytes.Buffer
	d := deps.DefaultDependencies(strings.NewReader(""), &stdout)
	d.Getenv = func(k string) string {
		if k == "HOME" {
			return home
		}
		return ""
	}

	cmd := NewCommand(d, nil)
	cmd.SetArgs([]string{"--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() failed: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, `"llm_cleaner": false`) {
		t.Errorf("expected json llm_cleaner: false, got: %s", out)
	}
	if !strings.Contains(out, `"cohere-transcribe-03-2026"`) {
		t.Errorf("expected default asr model in json, got: %s", out)
	}
}

func TestSettingsCommandTestFlag(t *testing.T) {
	home := t.TempDir()
	var stdout bytes.Buffer
	d := deps.DefaultDependencies(strings.NewReader(""), &stdout)
	d.Getenv = func(k string) string {
		if k == "HOME" {
			return home
		}
		return ""
	}
	d.LookPath = func(file string) (string, error) {
		return "/usr/bin/" + file, nil
	}
	d.RunOutput = func(ctx context.Context, name string, args ...string) (string, error) {
		return "active\n", nil
	}

	cmd := NewCommand(d, nil)
	cmd.SetArgs([]string{"--test"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() --test failed: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "Voxi Configuration & Component Diagnostics:") {
		t.Errorf("expected diagnostics header, got: %s", out)
	}
	if !strings.Contains(out, "ASR Engine:") {
		t.Errorf("expected ASR Engine check, got: %s", out)
	}
}

func TestSettingsCommandTestSubcommand(t *testing.T) {
	home := t.TempDir()
	var stdout bytes.Buffer
	d := deps.DefaultDependencies(strings.NewReader(""), &stdout)
	d.Getenv = func(k string) string {
		if k == "HOME" {
			return home
		}
		return ""
	}
	d.LookPath = func(file string) (string, error) {
		return "/usr/bin/" + file, nil
	}
	d.RunOutput = func(ctx context.Context, name string, args ...string) (string, error) {
		return "active\n", nil
	}

	cmd := NewCommand(d, nil)
	cmd.SetArgs([]string{"test", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() test --json failed: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, `"name": "ASR Engine"`) {
		t.Errorf("expected diagnostic JSON, got: %s", out)
	}
}
