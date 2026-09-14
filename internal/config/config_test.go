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

func TestSetTypeDelayMsMissingFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nested", "config.toml")

	// Missing file should be created automatically
	if err := SetTypeDelayMs(configPath, 5); err != nil {
		t.Fatalf("SetTypeDelayMs failed: %v", err)
	}

	val, ok, err := ReadTypeDelayMs(configPath)
	if err != nil || !ok || val != 5 {
		t.Fatalf("expected 5, got ok=%v, val=%d, err=%v", ok, val, err)
	}
}

func TestLoadUserSettingsDefaults(t *testing.T) {
	dir := t.TempDir()
	s, err := LoadUserSettings(dir)
	if err != nil {
		t.Fatalf("LoadUserSettings failed: %v", err)
	}

	def := DefaultUserSettings()
	if s.LLMCleaner != def.LLMCleaner {
		t.Errorf("LLMCleaner = %v, want %v", s.LLMCleaner, def.LLMCleaner)
	}
	if s.CleanupModel != def.CleanupModel {
		t.Errorf("CleanupModel = %v, want %v", s.CleanupModel, def.CleanupModel)
	}
	if s.CleanupBackend != "local_http" {
		t.Errorf("CleanupBackend = %v, want local_http", s.CleanupBackend)
	}
	if s.ASRModel != def.ASRModel {
		t.Errorf("ASRModel = %v, want %v", s.ASRModel, def.ASRModel)
	}
	if s.TypeDelayMs != def.TypeDelayMs {
		t.Errorf("TypeDelayMs = %v, want %v", s.TypeDelayMs, def.TypeDelayMs)
	}
	if s.DictationHistory != def.DictationHistory {
		t.Errorf("DictationHistory = %v, want %v", s.DictationHistory, def.DictationHistory)
	}
	if s.ModifierGating != def.ModifierGating {
		t.Errorf("ModifierGating = %v, want %v", s.ModifierGating, def.ModifierGating)
	}
}

func TestSaveAndLoadUserSettings(t *testing.T) {
	dir := t.TempDir()
	custom := &UserSettings{
		LLMCleaner:       true,
		CleanupBackend:   "agy",
		CleanupModel:     "smollm3-3b-instruct-q4",
		OpenAIBaseURL:    "http://127.0.0.1:9999/v1",
		ASRModel:         "large-v3-turbo",
		TypeDelayMs:      12,
		DictationHistory: false,
		ModifierGating:   false,
	}

	if err := SaveUserSettings(dir, custom); err != nil {
		t.Fatalf("SaveUserSettings failed: %v", err)
	}

	// Verify YAML file exists
	yamlPath := VoxiConfigYAMLPath(dir)
	if _, err := os.Stat(yamlPath); err != nil {
		t.Fatalf("YAML file missing: %v", err)
	}

	// Verify env file exists
	envPath := VoxiEnvPath(dir)
	envData, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("Env file missing: %v", err)
	}
	envMap := ParseEnv(envData)
	if envMap["VOXI_LLM_CLEANER"] != "true" {
		t.Errorf("VOXI_LLM_CLEANER = %s, want true", envMap["VOXI_LLM_CLEANER"])
	}
	if envMap["VOXI_CLEANUP_BACKEND"] != "agy" {
		t.Errorf("VOXI_CLEANUP_BACKEND = %s, want agy", envMap["VOXI_CLEANUP_BACKEND"])
	}
	if envMap["VOXI_CLEANUP_MODEL"] != "smollm3-3b-instruct-q4" {
		t.Errorf("VOXI_CLEANUP_MODEL = %s, want smollm3-3b-instruct-q4", envMap["VOXI_CLEANUP_MODEL"])
	}
	if envMap["OPENAI_BASE_URL"] != "http://127.0.0.1:9999/v1" {
		t.Errorf("OPENAI_BASE_URL = %s, want http://127.0.0.1:9999/v1", envMap["OPENAI_BASE_URL"])
	}
	if envMap["VOXI_ASR_MODEL"] != "large-v3-turbo" {
		t.Errorf("VOXI_ASR_MODEL = %s, want large-v3-turbo", envMap["VOXI_ASR_MODEL"])
	}
	if envMap["VOXI_HISTORY"] != "false" {
		t.Errorf("VOXI_HISTORY = %s, want false", envMap["VOXI_HISTORY"])
	}
	if envMap["VOXI_MODIFIER_GATING"] != "false" {
		t.Errorf("VOXI_MODIFIER_GATING = %s, want false", envMap["VOXI_MODIFIER_GATING"])
	}

	// Verify toml file has type_delay_ms
	tomlPath := VoxtypeConfigPath(dir)
	val, ok, err := ReadTypeDelayMs(tomlPath)
	if err != nil || !ok || val != 12 {
		t.Errorf("ReadTypeDelayMs = %d, ok=%v, want 12", val, ok)
	}

	// Reload settings
	loaded, err := LoadUserSettings(dir)
	if err != nil {
		t.Fatalf("LoadUserSettings failed: %v", err)
	}

	if loaded.LLMCleaner != custom.LLMCleaner {
		t.Errorf("loaded LLMCleaner = %v, want %v", loaded.LLMCleaner, custom.LLMCleaner)
	}
	if loaded.CleanupModel != custom.CleanupModel {
		t.Errorf("loaded CleanupModel = %v, want %v", loaded.CleanupModel, custom.CleanupModel)
	}
	if loaded.CleanupBackend != custom.CleanupBackend {
		t.Errorf("loaded CleanupBackend = %v, want %v", loaded.CleanupBackend, custom.CleanupBackend)
	}
	if loaded.OpenAIBaseURL != custom.OpenAIBaseURL {
		t.Errorf("loaded OpenAIBaseURL = %v, want %v", loaded.OpenAIBaseURL, custom.OpenAIBaseURL)
	}
	if loaded.ASRModel != custom.ASRModel {
		t.Errorf("loaded ASRModel = %v, want %v", loaded.ASRModel, custom.ASRModel)
	}
	if loaded.TypeDelayMs != custom.TypeDelayMs {
		t.Errorf("loaded TypeDelayMs = %v, want %v", loaded.TypeDelayMs, custom.TypeDelayMs)
	}
	if loaded.DictationHistory != custom.DictationHistory {
		t.Errorf("loaded DictationHistory = %v, want %v", loaded.DictationHistory, custom.DictationHistory)
	}
	if loaded.ModifierGating != custom.ModifierGating {
		t.Errorf("loaded ModifierGating = %v, want %v", loaded.ModifierGating, custom.ModifierGating)
	}
}

func TestFormatEnvPreservesExtraKeys(t *testing.T) {
	existing := map[string]string{
		"CUSTOM_TOKEN":   "secret123",
		"OPENAI_API_KEY": "sk-test",
	}
	s := &UserSettings{
		LLMCleaner:       true,
		CleanupModel:     "qwen3-4b-instruct-2507-q4",
		OpenAIBaseURL:    "http://127.0.0.1:8734/v1",
		ASRModel:         "cohere-transcribe-03-2026",
		TypeDelayMs:      0,
		DictationHistory: true,
		ModifierGating:   true,
	}

	data := FormatEnv(s, existing)
	parsed := ParseEnv(data)

	if parsed["CUSTOM_TOKEN"] != "secret123" {
		t.Errorf("CUSTOM_TOKEN lost: %v", parsed["CUSTOM_TOKEN"])
	}
	if parsed["OPENAI_API_KEY"] != "sk-test" {
		t.Errorf("OPENAI_API_KEY lost: %v", parsed["OPENAI_API_KEY"])
	}
	if parsed["VOXI_LLM_CLEANER"] != "true" {
		t.Errorf("VOXI_LLM_CLEANER = %s", parsed["VOXI_LLM_CLEANER"])
	}
}
