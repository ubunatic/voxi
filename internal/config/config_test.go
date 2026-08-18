package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadWriteTypeDelayMs(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	initial := `# Voxtype config
[input]
device = "default"

[output]
driver = "dotool"
# type_delay_ms = 0
`
	if err := os.WriteFile(configPath, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	// 1. Initial read should be 0, false because commented out
	val, ok, err := ReadTypeDelayMs(configPath)
	if err != nil || ok || val != 0 {
		t.Fatalf("expected ok=false, got ok=%v, val=%d, err=%v", ok, val, err)
	}

	// 2. Set to 25ms (inserts into [output])
	if err := SetTypeDelayMs(configPath, 25); err != nil {
		t.Fatalf("SetTypeDelayMs failed: %v", err)
	}

	val, ok, err = ReadTypeDelayMs(configPath)
	if err != nil || !ok || val != 25 {
		t.Fatalf("expected 25, got ok=%v, val=%d, err=%v", ok, val, err)
	}

	// Verify comments preserved
	data, _ := os.ReadFile(configPath)
	if !strings.Contains(string(data), "# Voxtype config") {
		t.Fatalf("comments lost: %s", string(data))
	}

	// 3. Update existing to 40ms
	if err := SetTypeDelayMs(configPath, 40); err != nil {
		t.Fatalf("SetTypeDelayMs second update failed: %v", err)
	}

	val, ok, err = ReadTypeDelayMs(configPath)
	if err != nil || !ok || val != 40 {
		t.Fatalf("expected 40, got ok=%v, val=%d, err=%v", ok, val, err)
	}
}
