package spec

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
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
	if s.Backend.DefaultBackend == "tts-serve" {
		t.Error("default_backend must not be tts-serve; the backend is opt-in only")
	}
	if s.TTSServe.URL == "" || s.TTSServe.TimeoutMs <= 0 || s.TTSServe.Engine == "" {
		t.Errorf("TTSServe defaults = %#v, want a usable url, timeout_ms, and engine", s.TTSServe)
	}
}

func TestLoadTTSAcceptsExplicitTTSServeBackend(t *testing.T) {
	s, err := LoadTTS()
	if err != nil {
		t.Fatalf("LoadTTS() error = %v", err)
	}
	s.Backend.DefaultBackend = "tts-serve"
	data, err := yaml.Marshal(s)
	if err != nil {
		t.Fatalf("marshal TTSSpec: %v", err)
	}
	var reparsed TTSSpec
	if err := yaml.Unmarshal(data, &reparsed); err != nil {
		t.Fatalf("unmarshal TTSSpec: %v", err)
	}
	if reparsed.Backend.DefaultBackend != "tts-serve" {
		t.Fatalf("backend.default_backend = %q, want tts-serve to round-trip", reparsed.Backend.DefaultBackend)
	}
}

func TestLoadTTSRejectsInvalidTTSServeSpec(t *testing.T) {
	base, err := LoadTTS()
	if err != nil {
		t.Fatalf("LoadTTS() error = %v", err)
	}
	for _, tc := range []struct {
		name    string
		mutate  func(*TTSSpec)
		wantErr string
	}{
		{"empty url", func(s *TTSSpec) { s.TTSServe.URL = "" }, "tts_serve.url"},
		{"relative url", func(s *TTSSpec) { s.TTSServe.URL = "not-a-url" }, "tts_serve.url"},
		{"timeout too low", func(s *TTSSpec) { s.TTSServe.TimeoutMs = 0 }, "tts_serve.timeout_ms"},
		{"timeout too high", func(s *TTSSpec) { s.TTSServe.TimeoutMs = 999999 }, "tts_serve.timeout_ms"},
		{"empty engine", func(s *TTSSpec) { s.TTSServe.Engine = "" }, "tts_serve.engine"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mutated := *base
			tc.mutate(&mutated)
			if err := validateTTSSpec(&mutated); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("validateTTSSpec() error = %v, want it to mention %q", err, tc.wantErr)
			}
		})
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
