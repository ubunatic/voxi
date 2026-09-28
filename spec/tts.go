// Package spec loads the embedded TTS playback tuning specification.
package spec

import (
	_ "embed"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed tts.yaml
var ttsYAML []byte

// TTSPlaybackSpec tunes silence trimming between TTS chunks.
type TTSPlaybackSpec struct {
	TrailingSilenceTrimMs      int     `yaml:"trailing_silence_trim_ms"`
	TrailingSilenceThresholdDB float64 `yaml:"trailing_silence_threshold_db"`
}

// TTSSpec is the parsed contents of spec/tts.yaml.
type TTSSpec struct {
	Backend  TTSBackendSpec  `yaml:"backend"`
	Piper    TTSPiperSpec    `yaml:"piper"`
	VoxCPM   TTSVoxCPMSpec   `yaml:"voxcpm"`
	Playback TTSPlaybackSpec `yaml:"playback"`
	LLM      TTSLLMSpec      `yaml:"llm"`
}

// TTSVoxCPMSpec defines the CLI runtime and cloned-voice profiles.
type TTSVoxCPMSpec struct {
	Binary            string                    `yaml:"binary"`
	Model             string                    `yaml:"model"`
	Backend           string                    `yaml:"backend"`
	Family            string                    `yaml:"family"`
	Threads           int                       `yaml:"threads"`
	Seed              int                       `yaml:"seed"`
	NumInferenceSteps int                       `yaml:"num_inference_steps"`
	GuidanceScale     float64                   `yaml:"guidance_scale"`
	DefaultPreset     string                    `yaml:"default_preset"`
	MaxTextChars      int                       `yaml:"max_text_chars"`
	Presets           map[string]TTSVoicePreset `yaml:"presets"`
}

// TTSVoicePreset selects an exact reference WAV and its transcript.
type TTSVoicePreset struct {
	ReferenceWav   string   `yaml:"reference_wav"`
	ReferenceText  string   `yaml:"reference_text"`
	SessionOptions []string `yaml:"session_options"`
}

// TTSBackendSpec defines the default synthesis backend.
type TTSBackendSpec struct {
	DefaultBackend string `yaml:"default_backend"`
}

// TTSPiperSpec defines the default Piper model and configuration paths.
type TTSPiperSpec struct {
	Model  string `yaml:"model"`
	Config string `yaml:"config"`
}

// TTSLLMSpec defines defaults for speech-ready LLM narration.
type TTSLLMSpec struct {
	DefaultHost string `yaml:"default_host"`
	MaxTokens   int    `yaml:"max_tokens"`
}

// LoadTTS parses the embedded TTS playback specification.
func LoadTTS() (*TTSSpec, error) {
	var s TTSSpec
	if err := yaml.Unmarshal(ttsYAML, &s); err != nil {
		return nil, fmt.Errorf("spec: parse tts.yaml: %w", err)
	}
	if err := validateTTSSpec(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func validateTTSSpec(s *TTSSpec) error {
	switch s.Backend.DefaultBackend {
	case "auto", "piper", "festival", "espeak-ng", "voxcpm":
	default:
		return fmt.Errorf("spec: backend.default_backend is invalid")
	}
	if s.VoxCPM.Binary == "" || s.VoxCPM.Model == "" || s.VoxCPM.Backend != "vulkan" || s.VoxCPM.Family == "" || s.VoxCPM.Threads < 1 || s.VoxCPM.NumInferenceSteps < 1 || s.VoxCPM.GuidanceScale <= 0 || s.VoxCPM.MaxTextChars < 1 {
		return fmt.Errorf("spec: voxcpm runtime settings must be complete and valid")
	}
	if s.VoxCPM.DefaultPreset == "" || len(s.VoxCPM.Presets) == 0 {
		return fmt.Errorf("spec: voxcpm.default_preset and presets are required")
	}
	if _, ok := s.VoxCPM.Presets[s.VoxCPM.DefaultPreset]; !ok {
		return fmt.Errorf("spec: voxcpm.default_preset must name a configured preset")
	}
	for name, preset := range s.VoxCPM.Presets {
		if name == "" || preset.ReferenceWav == "" || preset.ReferenceText == "" {
			return fmt.Errorf("spec: voxcpm preset %q requires reference_wav and reference_text", name)
		}
	}
	if s.Playback.TrailingSilenceTrimMs < 0 || s.Playback.TrailingSilenceTrimMs > 1000 {
		return fmt.Errorf("spec: playback.trailing_silence_trim_ms must be between 0 and 1000")
	}
	if s.Playback.TrailingSilenceThresholdDB < -80 || s.Playback.TrailingSilenceThresholdDB > -10 {
		return fmt.Errorf("spec: playback.trailing_silence_threshold_db must be between -80 and -10")
	}
	if s.LLM.DefaultHost == "" || s.LLM.MaxTokens < 1 || s.LLM.MaxTokens > 32768 {
		return fmt.Errorf("spec: llm.default_host and llm.max_tokens must be valid")
	}
	return nil
}

// TrailingSilenceTrim is the configured maximum trailing audio trim duration.
func (s *TTSSpec) TrailingSilenceTrim() time.Duration {
	return time.Duration(s.Playback.TrailingSilenceTrimMs) * time.Millisecond
}
