// Package spec loads the embedded TTS playback tuning specification.
package spec

import (
	_ "embed"
	"fmt"
	"net/url"
	"strings"
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
	TTSServe TTSServeSpec    `yaml:"tts_serve"`
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

// TTSServeSpec configures the tts-serve HTTP client backend (issue 155 M1,
// API in issue 157). It is opt-in only: "auto" never selects it.
type TTSServeSpec struct {
	URL           string         `yaml:"url"`
	TimeoutMs     int            `yaml:"timeout_ms"`
	ReferenceWav  string         `yaml:"reference_wav"`
	Engine        string         `yaml:"engine"`
	Settings      map[string]any `yaml:"settings"`
	MaxChunkRunes int            `yaml:"max_chunk_runes"`
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
	case "auto", "piper", "festival", "espeak-ng", "tts-serve":
	default:
		return fmt.Errorf("spec: backend.default_backend is invalid")
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
	if strings.TrimSpace(s.TTSServe.URL) == "" {
		return fmt.Errorf("spec: tts_serve.url must not be empty")
	}
	if parsed, err := url.Parse(s.TTSServe.URL); err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("spec: tts_serve.url must be an absolute URL")
	}
	if s.TTSServe.TimeoutMs < 1000 || s.TTSServe.TimeoutMs > 600000 {
		return fmt.Errorf("spec: tts_serve.timeout_ms must be between 1000 and 600000")
	}
	if strings.TrimSpace(s.TTSServe.Engine) == "" {
		return fmt.Errorf("spec: tts_serve.engine must not be empty")
	}
	if s.TTSServe.MaxChunkRunes < 20 || s.TTSServe.MaxChunkRunes > 2000 {
		return fmt.Errorf("spec: tts_serve.max_chunk_runes must be between 20 and 2000")
	}
	return nil
}

// TrailingSilenceTrim is the configured maximum trailing audio trim duration.
func (s *TTSSpec) TrailingSilenceTrim() time.Duration {
	return time.Duration(s.Playback.TrailingSilenceTrimMs) * time.Millisecond
}

// Timeout is the configured tts-serve HTTP request timeout.
func (s *TTSServeSpec) Timeout() time.Duration {
	return time.Duration(s.TimeoutMs) * time.Millisecond
}
