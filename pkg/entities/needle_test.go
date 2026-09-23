package entities

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNeedleRunnerMissingBinaryFallback(t *testing.T) {
	cfg := &Config{
		Enabled:             true,
		EngineBinary:        "/nonexistent/binary/needle",
		ConfidenceThreshold: 0.65,
	}
	runner := NewNeedleRunner(cfg)

	raw := "ich spreche mit uwe"
	out := runner.Process(context.Background(), raw)
	if out != raw {
		t.Errorf("expected fallback to raw text %q, got %q", raw, out)
	}
}

func TestNeedleRunnerTimeoutFallback(t *testing.T) {
	tmpDir := t.TempDir()
	slowScript := filepath.Join(tmpDir, "slow_needle.sh")
	scriptContent := "#!/bin/sh\nexec sleep 1\n"
	if err := os.WriteFile(slowScript, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		Enabled:             true,
		EngineBinary:        slowScript,
		ConfidenceThreshold: 0.65,
	}
	runner := NewNeedleRunner(cfg)
	runner.Timeout = 50 * time.Millisecond

	raw := "ich spreche mit uwe"
	start := time.Now()
	out := runner.Process(context.Background(), raw)
	elapsed := time.Since(start)

	if out != raw {
		t.Errorf("expected fallback to raw text %q, got %q", raw, out)
	}
	if elapsed > 150*time.Millisecond {
		t.Errorf("expected timeout execution within ~50ms, took %v", elapsed)
	}
}

func TestNeedleRunnerSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	mockScript := filepath.Join(tmpDir, "mock_needle.sh")
	jsonResp := `{"detected_entities": [{"raw_token": "andja", "is_person": true, "matched_name": "Anja", "confidence": 0.95}]}`
	scriptContent := fmt.Sprintf("#!/bin/sh\ncat > /dev/null\necho '%s'\n", jsonResp)
	if err := os.WriteFile(mockScript, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		Enabled:             true,
		EngineBinary:        mockScript,
		ConfidenceThreshold: 0.65,
	}
	runner := NewNeedleRunner(cfg)
	runner.Timeout = 500 * time.Millisecond

	raw := "hallo andja"
	out := runner.Process(context.Background(), raw)
	expected := "hallo Anja"
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}
