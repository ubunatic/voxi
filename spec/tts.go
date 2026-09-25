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
	Playback TTSPlaybackSpec `yaml:"playback"`
	LLM      TTSLLMSpec      `yaml:"llm"`
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
	if s.Backend.DefaultBackend != "auto" && s.Backend.DefaultBackend != "piper" && s.Backend.DefaultBackend != "festival" && s.Backend.DefaultBackend != "espeak-ng" {
		return nil, fmt.Errorf("spec: backend.default_backend is invalid")
	}
	if s.Playback.TrailingSilenceTrimMs < 0 || s.Playback.TrailingSilenceTrimMs > 1000 {
		return nil, fmt.Errorf("spec: playback.trailing_silence_trim_ms must be between 0 and 1000")
	}
	if s.Playback.TrailingSilenceThresholdDB < -80 || s.Playback.TrailingSilenceThresholdDB > -10 {
		return nil, fmt.Errorf("spec: playback.trailing_silence_threshold_db must be between -80 and -10")
	}
	if s.LLM.DefaultHost == "" || s.LLM.MaxTokens < 1 || s.LLM.MaxTokens > 32768 {
		return nil, fmt.Errorf("spec: llm.default_host and llm.max_tokens must be valid")
	}
	return &s, nil
}

// TrailingSilenceTrim is the configured maximum trailing audio trim duration.
func (s *TTSSpec) TrailingSilenceTrim() time.Duration {
	return time.Duration(s.Playback.TrailingSilenceTrimMs) * time.Millisecond
}
