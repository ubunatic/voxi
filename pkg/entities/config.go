package entities

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type PersonEntity struct {
	Name    string   `yaml:"name"`
	Aliases []string `yaml:"aliases"`
}

type Entities struct {
	Persons []PersonEntity `yaml:"persons"`
}

type Config struct {
	Enabled             bool     `yaml:"enabled"`
	EngineBinary        string   `yaml:"engine_binary"`
	ConfidenceThreshold float64  `yaml:"confidence_threshold"`
	Entities            Entities `yaml:"entities"`
}

const (
	DefaultEngineBinary        = "/usr/local/bin/needle"
	DefaultConfidenceThreshold = 0.65
)

func DefaultConfig() *Config {
	return &Config{
		Enabled:             true,
		EngineBinary:        DefaultEngineBinary,
		ConfidenceThreshold: DefaultConfidenceThreshold,
		Entities:            Entities{Persons: []PersonEntity{}},
	}
}

func ExpandHome(path string) string {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, ".config", "voxi", "entities.yaml")
	}
	if path == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if len(path) > 1 && path[:2] == "~/" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

func LoadConfig(path string) (*Config, error) {
	resolvedPath := ExpandHome(path)
	cfg := DefaultConfig()

	if resolvedPath == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read entities config: %w", err)
	}

	if len(data) == 0 {
		return cfg, nil
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return cfg, fmt.Errorf("unmarshal entities config: %w", err)
	}

	if cfg.EngineBinary == "" {
		cfg.EngineBinary = DefaultEngineBinary
	}
	if cfg.ConfidenceThreshold == 0 {
		cfg.ConfidenceThreshold = DefaultConfidenceThreshold
	}

	return cfg, nil
}

func (c *Config) GetPersonNames() []string {
	if c == nil {
		return nil
	}
	names := make([]string, 0, len(c.Entities.Persons))
	for _, p := range c.Entities.Persons {
		if p.Name != "" {
			names = append(names, p.Name)
		}
	}
	return names
}

func (c *Config) GenerateJSONSchema() ([]byte, error) {
	names := c.GetPersonNames()

	matchedNameProp := map[string]any{
		"type": "string",
	}
	if len(names) > 0 {
		matchedNameProp["enum"] = names
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"detected_entities": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"raw_token":    map[string]any{"type": "string"},
						"is_person":    map[string]any{"type": "boolean"},
						"matched_name": matchedNameProp,
						"confidence":   map[string]any{"type": "number"},
					},
					"required": []string{"raw_token", "is_person", "matched_name"},
				},
			},
		},
		"required": []string{"detected_entities"},
	}

	return json.MarshalIndent(schema, "", "  ")
}
