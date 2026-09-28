package spec

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoadTTSBackendDefaults(t *testing.T) {
	s, err := LoadTTS()
	if err != nil {
		t.Fatalf("LoadTTS() error = %v", err)
	}
	if s.Backend.DefaultBackend != "auto" {
		t.Errorf("default backend = %q, want auto", s.Backend.DefaultBackend)
	}
	if s.Piper.Model != "~/.local/share/voxi/voices/en_US-lessac-medium.onnx" || s.Piper.Config != "~/.local/share/voxi/voices/en_US-lessac-medium.onnx.json" {
		t.Errorf("Piper defaults = %#v, want installed Lessac medium voice paths", s.Piper)
	}
	if s.Backend.DefaultBackend == "voxcpm" {
		t.Error("Piper/legacy auto behavior must remain the default")
	}
}

func TestLoadTTSAcceptsVoxCPMPlaceholderBackend(t *testing.T) {
	s, err := LoadTTS()
	if err != nil {
		t.Fatalf("LoadTTS() error = %v", err)
	}
	s.Backend.DefaultBackend = "voxcpm"
	if err := validateTTSSpec(s); err != nil {
		t.Fatalf("validateTTSSpec() rejected voxcpm placeholder: %v", err)
	}
}

func TestTTSMatchesSchema(t *testing.T) {
	schemaData, err := os.ReadFile("schemas/tts.schema.json")
	if err != nil {
		t.Fatalf("read tts.schema.json: %v", err)
	}
	var schema struct {
		Properties map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schemaData, &schema); err != nil {
		t.Fatalf("parse tts.schema.json: %v", err)
	}

	var raw map[string]map[string]any
	if err := yaml.Unmarshal(ttsYAML, &raw); err != nil {
		t.Fatalf("parse tts.yaml: %v", err)
	}

	for section, fields := range raw {
		sectionSchema, ok := schema.Properties[section]
		if !ok {
			t.Errorf("tts.yaml defines section %q not present in tts.schema.json", section)
			continue
		}
		for field := range fields {
			if _, fieldOk := sectionSchema.Properties[field]; !fieldOk {
				t.Errorf("tts.yaml defines field %q.%q not in schema", section, field)
			}
		}
	}

	ttsVal := TTSSpec{}
	ttsValType := reflect.TypeOf(ttsVal)
	for i := 0; i < ttsValType.NumField(); i++ {
		secTag := ttsValType.Field(i).Tag.Get("yaml")
		if secTag == "" {
			continue
		}
		if _, ok := schema.Properties[secTag]; !ok {
			t.Errorf("Go TTSSpec struct defines section %q missing in tts.schema.json", secTag)
		}
	}
}
