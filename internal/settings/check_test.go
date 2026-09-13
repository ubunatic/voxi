package settings

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/deps"
)

func TestRunDiagnosticsPass(t *testing.T) {
	home := t.TempDir()
	d := deps.DefaultDependencies(strings.NewReader(""), nil)
	d.LookPath = func(file string) (string, error) {
		return "/usr/bin/" + file, nil
	}
	d.RunOutput = func(ctx context.Context, name string, args ...string) (string, error) {
		if name == "systemctl" {
			return "active\n", nil
		}
		return "", nil
	}

	s := &config.UserSettings{
		LLMCleaner:       false,
		ASRModel:         "cohere-transcribe-03-2026",
		TypeDelayMs:      5,
		DictationHistory: true,
		ModifierGating:   false,
	}

	report := RunDiagnostics(context.Background(), d, home, s)
	if report == nil {
		t.Fatal("expected non-nil report")
	}

	if report.Passed < 2 {
		t.Errorf("expected at least 2 passes, got %d", report.Passed)
	}
	if report.Failed > 0 {
		t.Errorf("expected 0 failures, got %d", report.Failed)
	}

	text := RenderDiagnosticReport(report)
	if !strings.Contains(text, "Voxi Configuration & Component Diagnostics:") {
		t.Errorf("rendered report missing header")
	}
	if !strings.Contains(text, "ASR Engine:") {
		t.Errorf("rendered report missing ASR Engine item")
	}

	jsonStr, err := RenderDiagnosticJSON(report)
	if err != nil {
		t.Fatalf("RenderDiagnosticJSON failed: %v", err)
	}
	if !strings.Contains(jsonStr, `"name": "ASR Engine"`) {
		t.Errorf("json report missing ASR Engine: %s", jsonStr)
	}
}

func TestRunDiagnosticsMissingBinary(t *testing.T) {
	home := t.TempDir()
	d := deps.DefaultDependencies(strings.NewReader(""), nil)
	d.LookPath = func(file string) (string, error) {
		if file == "crispasr" {
			return "", exec.ErrNotFound
		}
		return "/usr/bin/" + file, nil
	}

	s := &config.UserSettings{
		ASRModel: "cohere-transcribe-03-2026",
	}

	report := RunDiagnostics(context.Background(), d, home, s)
	if report.Failed == 0 {
		t.Errorf("expected failure for missing crispasr, got %d failures", report.Failed)
	}
}
