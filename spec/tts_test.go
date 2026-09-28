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

func TestLoadTTSVoxCPMDefaultsAndPresets(t *testing.T) {
	s, err := LoadTTS()
	if err != nil {
		t.Fatalf("LoadTTS() error = %v", err)
	}
	if s.VoxCPM.Binary == "" || s.VoxCPM.Model == "" || s.VoxCPM.Backend != "vulkan" {
		t.Fatalf("VoxCPM runtime defaults = %#v, want configured CLI/model and Vulkan", s.VoxCPM)
	}
	if s.VoxCPM.DefaultPreset != "full" || s.VoxCPM.MaxTextChars < 1 {
		t.Fatalf("VoxCPM default preset/limit = %q/%d, want full and positive cap", s.VoxCPM.DefaultPreset, s.VoxCPM.MaxTextChars)
	}
	full, fullOK := s.VoxCPM.Presets["full"]
	short, shortOK := s.VoxCPM.Presets["short"]
	if !fullOK || !shortOK || full.ReferenceWav == "" || short.ReferenceWav == "" || full.ReferenceText == "" || short.ReferenceText == "" {
		t.Fatalf("VoxCPM presets = %#v, want full and short WAV/transcript profiles", s.VoxCPM.Presets)
	}
	if len(full.SessionOptions) != 1 || full.SessionOptions[0] != "voxcpm1.audiovae_encoder_sample_capacity=320000" || len(short.SessionOptions) != 0 {
		t.Fatalf("preset session options full=%v short=%v", full.SessionOptions, short.SessionOptions)
	}
	if s.VoxCPM.Binary != "~/.local/share/voxi/voxcpm/bin/audiocpp_cli" ||
		s.VoxCPM.Model != "~/.local/share/voxi/voxcpm/model/voxcpm-0.5b-q8_0-audiovae-f16.gguf" ||
		full.ReferenceWav != "~/.local/share/voxi/voxcpm/voices/full.wav" ||
		short.ReferenceWav != "~/.local/share/voxi/voxcpm/voices/short.wav" {
		t.Fatalf("VoxCPM install paths = binary %q model %q presets %#v", s.VoxCPM.Binary, s.VoxCPM.Model, s.VoxCPM.Presets)
	}
	if s.VoxCPM.RuntimeVersion != "v0.8.2-audio8-perf-hotfix" ||
		s.VoxCPM.SourceCommit != "ac16661d144f00f84ea0483f3574c374c9868e2d" ||
		s.VoxCPM.ModelRevision != "5f57aad57dc0acea2e6a571ec99c71e4035f812d" ||
		s.VoxCPM.ModelSHA256 != "01210319c5ce617613c9d1c38e34649f7479e98a60d51b719f7df82970658241" || len(s.VoxCPM.RuntimeAssets) != 2 {
		t.Fatalf("VoxCPM pins = version %q commit %q model rev %q sha %q assets %#v", s.VoxCPM.RuntimeVersion, s.VoxCPM.SourceCommit, s.VoxCPM.ModelRevision, s.VoxCPM.ModelSHA256, s.VoxCPM.RuntimeAssets)
	}
	if short.ReferenceStartSample != 109386 || short.ReferenceEndSample != 248091 {
		t.Fatalf("short reference sample range = [%d,%d), want [109386,248091)", short.ReferenceStartSample, short.ReferenceEndSample)
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
