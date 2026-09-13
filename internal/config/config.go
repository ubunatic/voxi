package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	typeDelayActiveRe = regexp.MustCompile(`(?m)^([ \t]*)type_delay_ms([ \t]*=[ \t]*)([0-9]+)[ \t]*$`)
	outputHeaderRe    = regexp.MustCompile(`(?m)^\[output\][ \t]*$`)
)

// UserSettings defines user-configurable Voxi settings across config.yaml, env, and config.toml.
type UserSettings struct {
	LLMCleaner       bool   `json:"llm_cleaner" yaml:"llm_cleaner"`
	CleanupModel     string `json:"cleanup_model" yaml:"cleanup_model"`
	OpenAIBaseURL    string `json:"openai_base_url" yaml:"openai_base_url"`
	ASRModel         string `json:"asr_model" yaml:"asr_model"`
	TypeDelayMs      int    `json:"type_delay_ms" yaml:"type_delay_ms"`
	DictationHistory bool   `json:"dictation_history" yaml:"dictation_history"`
	ModifierGating   bool   `json:"modifier_gating" yaml:"modifier_gating"`
}

// DefaultUserSettings returns standard user settings.
func DefaultUserSettings() *UserSettings {
	return &UserSettings{
		LLMCleaner:       false,
		CleanupModel:     "qwen3-4b-instruct-2507-q4",
		OpenAIBaseURL:    "http://127.0.0.1:8734/v1",
		ASRModel:         "cohere-transcribe-03-2026",
		TypeDelayMs:      0,
		DictationHistory: true,
		ModifierGating:   true,
	}
}

// VoxiConfigDir is ~/.config/voxi.
func VoxiConfigDir(home string) string {
	return filepath.Join(home, ".config", "voxi")
}

// VoxiConfigYAMLPath is ~/.config/voxi/config.yaml.
func VoxiConfigYAMLPath(home string) string {
	return filepath.Join(home, ".config", "voxi", "config.yaml")
}

// VoxiEnvPath is ~/.config/voxi/env.
func VoxiEnvPath(home string) string {
	return filepath.Join(home, ".config", "voxi", "env")
}

// VoxtypeConfigPath is ~/.config/voxtype/config.toml.
func VoxtypeConfigPath(home string) string {
	return filepath.Join(home, ".config", "voxtype", "config.toml")
}

// LoadUserSettings loads user settings with precedence:
// 1. Defaults
// 2. ~/.config/voxi/config.yaml (if present)
// 3. ~/.config/voxi/env (if present)
// 4. ~/.config/voxtype/config.toml (for type_delay_ms if present)
func LoadUserSettings(home string) (*UserSettings, error) {
	s := DefaultUserSettings()
	if home == "" {
		if h, err := os.UserHomeDir(); err == nil {
			home = h
		}
	}

	yamlPath := VoxiConfigYAMLPath(home)
	if data, err := os.ReadFile(yamlPath); err == nil {
		if err := yaml.Unmarshal(data, s); err != nil {
			return nil, fmt.Errorf("parse %s: %w", yamlPath, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read %s: %w", yamlPath, err)
	}

	envPath := VoxiEnvPath(home)
	if envData, err := os.ReadFile(envPath); err == nil {
		envMap := ParseEnv(envData)
		if v, ok := envMap["VOXI_LLM_CLEANER"]; ok {
			s.LLMCleaner = parseBool(v, s.LLMCleaner)
		}
		if v, ok := envMap["VOXI_CLEANUP_MODEL"]; ok && v != "" {
			s.CleanupModel = v
		}
		if v, ok := envMap["OPENAI_BASE_URL"]; ok && v != "" {
			s.OpenAIBaseURL = v
		}
		if v, ok := envMap["VOXI_ASR_MODEL"]; ok && v != "" {
			s.ASRModel = v
		}
		if v, ok := envMap["VOXI_HISTORY"]; ok {
			s.DictationHistory = parseBool(v, s.DictationHistory)
		}
		if v, ok := envMap["VOXI_MODIFIER_GATING"]; ok {
			s.ModifierGating = parseBool(v, s.ModifierGating)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read %s: %w", envPath, err)
	}

	tomlPath := VoxtypeConfigPath(home)
	if ms, ok, err := ReadTypeDelayMs(tomlPath); err == nil && ok {
		s.TypeDelayMs = ms
	}

	return s, nil
}

// SaveUserSettings writes user settings atomically to:
// - ~/.config/voxi/config.yaml
// - ~/.config/voxi/env
// - ~/.config/voxtype/config.toml (for type_delay_ms)
func SaveUserSettings(home string, s *UserSettings) error {
	if home == "" {
		if h, err := os.UserHomeDir(); err == nil {
			home = h
		}
	}

	// 1. Write YAML config
	yamlPath := VoxiConfigYAMLPath(home)
	yamlData, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("encode yaml: %w", err)
	}
	header := []byte("# Voxi user configuration\n")
	if err := WriteConfigAtomic(yamlPath, append(header, yamlData...)); err != nil {
		return fmt.Errorf("save %s: %w", yamlPath, err)
	}

	// 2. Write env file
	envPath := VoxiEnvPath(home)
	var existingEnv map[string]string
	if data, err := os.ReadFile(envPath); err == nil {
		existingEnv = ParseEnv(data)
	}
	envBytes := FormatEnv(s, existingEnv)
	if err := WriteConfigAtomic(envPath, envBytes); err != nil {
		return fmt.Errorf("save %s: %w", envPath, err)
	}

	// 3. Write type_delay_ms to voxtype config.toml
	tomlPath := VoxtypeConfigPath(home)
	if err := SetTypeDelayMs(tomlPath, s.TypeDelayMs); err != nil {
		return fmt.Errorf("save %s: %w", tomlPath, err)
	}

	return nil
}

// FormatEnv serializes UserSettings into environment file format (KEY=VALUE).
func FormatEnv(s *UserSettings, existing map[string]string) []byte {
	merged := make(map[string]string)
	for k, v := range existing {
		merged[k] = v
	}

	merged["VOXI_LLM_CLEANER"] = strconv.FormatBool(s.LLMCleaner)
	merged["VOXI_CLEANUP_MODEL"] = s.CleanupModel
	if s.OpenAIBaseURL != "" {
		merged["OPENAI_BASE_URL"] = s.OpenAIBaseURL
	}
	merged["VOXI_ASR_MODEL"] = s.ASRModel
	merged["VOXI_HISTORY"] = strconv.FormatBool(s.DictationHistory)
	merged["VOXI_MODIFIER_GATING"] = strconv.FormatBool(s.ModifierGating)

	managedOrder := []string{
		"VOXI_LLM_CLEANER",
		"VOXI_CLEANUP_MODEL",
		"OPENAI_BASE_URL",
		"VOXI_ASR_MODEL",
		"VOXI_HISTORY",
		"VOXI_MODIFIER_GATING",
	}

	var buf bytes.Buffer
	buf.WriteString("# Voxi daemon environment configuration\n")

	managedSet := make(map[string]bool)
	for _, k := range managedOrder {
		managedSet[k] = true
		if v, ok := merged[k]; ok {
			fmt.Fprintf(&buf, "%s=%s\n", k, v)
		}
	}

	var extraKeys []string
	for k := range merged {
		if !managedSet[k] {
			extraKeys = append(extraKeys, k)
		}
	}
	sort.Strings(extraKeys)
	for _, k := range extraKeys {
		fmt.Fprintf(&buf, "%s=%s\n", k, merged[k])
	}

	return buf.Bytes()
}

// ParseEnv parses standard environment file content into a key-value map.
func ParseEnv(data []byte) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			v = strings.Trim(v, `"'`)
			result[k] = v
		}
	}
	return result
}

func parseBool(s string, fallback bool) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "1", "t", "true", "yes", "on", "enable", "enabled":
		return true
	case "0", "f", "false", "no", "off", "disable", "disabled":
		return false
	default:
		return fallback
	}
}

// ReadTypeDelayMs reads the active (uncommented) `type_delay_ms` value from
// a voxtype config.toml. Returns 0, false if the key is absent (voxtype's default).
func ReadTypeDelayMs(path string) (int, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("read %s: %w", path, err)
	}
	m := typeDelayActiveRe.FindSubmatch(data)
	if m == nil {
		return 0, false, nil
	}
	ms, err := strconv.Atoi(string(m[3]))
	if err != nil {
		return 0, false, fmt.Errorf("parse type_delay_ms: %w", err)
	}
	return ms, true, nil
}

// SetTypeDelayMs writes `ms` into config.toml at `path`, creating the file if missing
// and preserving comments and unrelated settings if existing.
func SetTypeDelayMs(path string, ms int) error {
	if ms < 0 {
		return fmt.Errorf("type_delay_ms must be >= 0, got %d", ms)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			initial := fmt.Sprintf("# Voxtype config\n[output]\ndriver = \"dotool\"\ntype_delay_ms = %d\n", ms)
			return WriteConfigAtomic(path, []byte(initial))
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	replacement := []byte(fmt.Sprintf("${1}type_delay_ms${2}%d", ms))
	if typeDelayActiveRe.Match(data) {
		updated := typeDelayActiveRe.ReplaceAll(data, replacement)
		return WriteConfigAtomic(path, updated)
	}
	loc := outputHeaderRe.FindIndex(data)
	if loc == nil {
		// Append [output] section if none found
		line := fmt.Sprintf("\n[output]\ntype_delay_ms = %d\n", ms)
		updated := append(data, []byte(line)...)
		return WriteConfigAtomic(path, updated)
	}
	insertAt := loc[1]
	line := []byte(fmt.Sprintf("\ntype_delay_ms = %d", ms))
	updated := append(append(append([]byte{}, data[:insertAt]...), line...), data[insertAt:]...)
	return WriteConfigAtomic(path, updated)
}

// WriteConfigAtomic writes data to path atomically using a temporary file.
func WriteConfigAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir %s: %w", dir, err)
	}
	info, err := os.Stat(path)
	mode := os.FileMode(0644)
	if err == nil {
		mode = info.Mode()
	}
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Chmod(mode)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("install config: %w", err)
	}
	return nil
}
