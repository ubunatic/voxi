package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestPostProcessorDisabled(t *testing.T) {
	pp, err := NewPostProcessor(PostProcessorOptions{Disabled: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw := "ich spreche mit uwe"
	out := pp.Process(context.Background(), raw)
	if out != raw {
		t.Errorf("expected %q, got %q", raw, out)
	}
}

func TestPostProcessorConfigDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "entities.yaml")
	if err := os.WriteFile(cfgFile, []byte("enabled: false\n"), 0644); err != nil {
		t.Fatal(err)
	}

	pp, err := NewPostProcessor(PostProcessorOptions{ConfigPath: cfgFile})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw := "ich spreche mit uwe"
	out := pp.Process(context.Background(), raw)
	if out != raw {
		t.Errorf("expected %q, got %q", raw, out)
	}
}

func TestPostProcessorSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	mockScript := filepath.Join(tmpDir, "mock_needle.sh")
	jsonResp := `{"detected_entities": [{"raw_token": "andja", "is_person": true, "matched_name": "Anja", "confidence": 0.95}]}`
	scriptContent := fmt.Sprintf("#!/bin/sh\ncat > /dev/null\necho '%s'\n", jsonResp)
	if err := os.WriteFile(mockScript, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	cfgContent := fmt.Sprintf("enabled: true\nengine_binary: %q\nconfidence_threshold: 0.65\n", mockScript)
	cfgFile := filepath.Join(tmpDir, "entities.yaml")
	if err := os.WriteFile(cfgFile, []byte(cfgContent), 0644); err != nil {
		t.Fatal(err)
	}

	pp, err := NewPostProcessor(PostProcessorOptions{ConfigPath: cfgFile})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw := "hallo andja"
	expected := "hallo Anja"
	out := pp.Process(context.Background(), raw)
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}
