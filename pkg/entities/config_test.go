package entities

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistent := filepath.Join(tmpDir, "does_not_exist.yaml")

	cfg, err := LoadConfig(nonExistent)
	if err != nil {
		t.Fatalf("expected no error loading non-existent config, got %v", err)
	}
	if !cfg.Enabled {
		t.Errorf("expected Enabled true by default, got false")
	}
	if cfg.EngineBinary != DefaultEngineBinary {
		t.Errorf("expected default engine binary %q, got %q", DefaultEngineBinary, cfg.EngineBinary)
	}
	if cfg.ConfidenceThreshold != DefaultConfidenceThreshold {
		t.Errorf("expected default threshold %f, got %f", DefaultConfidenceThreshold, cfg.ConfidenceThreshold)
	}
}

func TestLoadConfigEmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	emptyFile := filepath.Join(tmpDir, "empty.yaml")
	if err := os.WriteFile(emptyFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(emptyFile)
	if err != nil {
		t.Fatalf("expected no error loading empty config, got %v", err)
	}
	if !cfg.Enabled {
		t.Errorf("expected Enabled true by default, got false")
	}
	if cfg.ConfidenceThreshold != DefaultConfidenceThreshold {
		t.Errorf("expected default confidence threshold %f, got %f", DefaultConfidenceThreshold, cfg.ConfidenceThreshold)
	}
}

func TestLoadConfigValid(t *testing.T) {
	yamlData := `
enabled: true
engine_binary: "/custom/bin/needle"
confidence_threshold: 0.75
entities:
  persons:
    - name: "Uwe"
      aliases: ["uwe", "oowe", "uve"]
    - name: "Anja"
      aliases: ["anja", "andja", "ania"]
    - name: "Guenther"
      aliases: ["guenther", "günther", "gunter"]
`
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "entities.yaml")
	if err := os.WriteFile(cfgFile, []byte(yamlData), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(cfgFile)
	if err != nil {
		t.Fatalf("failed to load valid config: %v", err)
	}
	if !cfg.Enabled {
		t.Errorf("expected enabled true")
	}
	if cfg.EngineBinary != "/custom/bin/needle" {
		t.Errorf("expected engine_binary /custom/bin/needle, got %q", cfg.EngineBinary)
	}
	if cfg.ConfidenceThreshold != 0.75 {
		t.Errorf("expected confidence_threshold 0.75, got %f", cfg.ConfidenceThreshold)
	}
	if len(cfg.Entities.Persons) != 3 {
		t.Fatalf("expected 3 persons, got %d", len(cfg.Entities.Persons))
	}
	if cfg.Entities.Persons[0].Name != "Uwe" {
		t.Errorf("expected first person Uwe, got %q", cfg.Entities.Persons[0].Name)
	}
}

func TestGenerateJSONSchema(t *testing.T) {
	cfg := &Config{
		Enabled:             true,
		EngineBinary:        "/usr/local/bin/needle",
		ConfidenceThreshold: 0.65,
		Entities: Entities{
			Persons: []PersonEntity{
				{Name: "Uwe", Aliases: []string{"uwe"}},
				{Name: "Anja", Aliases: []string{"anja"}},
				{Name: "Guenther", Aliases: []string{"guenther"}},
			},
		},
	}

	schemaBytes, err := cfg.GenerateJSONSchema()
	if err != nil {
		t.Fatalf("GenerateJSONSchema failed: %v", err)
	}

	var schema map[string]any
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		t.Fatalf("unmarshal generated schema: %v", err)
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties in schema")
	}
	detectedEntities, ok := props["detected_entities"].(map[string]any)
	if !ok {
		t.Fatalf("expected detected_entities in properties")
	}
	items, ok := detectedEntities["items"].(map[string]any)
	if !ok {
		t.Fatalf("expected items in detected_entities")
	}
	itemProps, ok := items["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties in items")
	}

	matchedName, ok := itemProps["matched_name"].(map[string]any)
	if !ok {
		t.Fatalf("expected matched_name property")
	}

	enums, ok := matchedName["enum"].([]any)
	if !ok {
		t.Fatalf("expected enum array in matched_name")
	}

	expectedNames := map[string]bool{"Uwe": true, "Anja": true, "Guenther": true}
	for _, e := range enums {
		name, ok := e.(string)
		if !ok {
			t.Errorf("enum element not string: %v", e)
			continue
		}
		if !expectedNames[name] {
			t.Errorf("unexpected enum value: %s", name)
		}
	}
}
