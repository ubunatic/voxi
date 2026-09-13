package settings

import (
	"bytes"
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
