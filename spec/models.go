// Package spec loads spec/models.yaml: the local ASR models Voxi can drive
// and each model's hallucination stop-word patterns. It is
// the single source of truth for model names and hallucination filtering
// — see docs/Spec.md. Application code must not duplicate these values.
package spec

import (
	_ "embed"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

//go:embed models.yaml
var modelsYAML []byte

// Model describes one local ASR model Voxi can invoke.
type Model struct {
	Label       string     `yaml:"label"`
	Engine      string     `yaml:"engine"`
	StopWords   []StopWord `yaml:"stop_words"`
	RequiresGPU bool       `yaml:"requires_gpu"`
	CPUFallback string     `yaml:"cpu_fallback"`
}

// StopWord is a shipped hallucination filter. ID is stable so a user can
// disable a rule without editing the model specification.
type StopWord struct {
	ID      string `yaml:"id"`
	Pattern string `yaml:"pattern"`
}

// ModelSpec is the parsed contents of spec/models.yaml.
type ModelSpec struct {
	DefaultModel     string               `yaml:"default_model"`
	SpeechContext    SpeechContextSpec    `yaml:"speech_context"`
	TranscriptSafety TranscriptSafetySpec `yaml:"transcript_safety"`
	Models           map[string]Model     `yaml:"models"`
}

// TranscriptSafetySpec defines conservative limits applied after transcript
// cleanup/replacements and before history or desktop injection.
type TranscriptSafetySpec struct {
	MaxChars           int `yaml:"max_chars"`
	MaxTokenChars      int `yaml:"max_token_chars"`
	MaxRepeatUnitChars int `yaml:"max_repeat_unit_chars"`
	MinRepeatCount     int `yaml:"min_repeat_count"`
	MinRepeatedChars   int `yaml:"min_repeated_chars"`
}

// SpeechContextSpec defines the bounded decoder prompt used by the
// technical-dictation context, active by default for small.en (see
// EagerOptions.SpeechContext / --speech-context=false to disable). It is
// data rather than runtime user config.
type SpeechContextSpec struct {
	PromptPrefix string   `yaml:"prompt_prefix"`
	Terms        []string `yaml:"terms"`
	MaxTerms     int      `yaml:"max_terms"`
	MaxChars     int      `yaml:"max_chars"`
	MaxTermChars int      `yaml:"max_term_chars"`
}

// LoadModels parses the embedded model spec. It fails if the spec is
// malformed or if default_model does not name an entry in models.
func LoadModels() (*ModelSpec, error) {
	return parseModelSpec(modelsYAML)
}

// parseModelSpec unmarshals and validates a models.yaml document. Split out
// from LoadModels so tests can exercise validation against ad hoc YAML
// without touching the embedded spec.
func parseModelSpec(data []byte) (*ModelSpec, error) {
	var s ModelSpec
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("spec: parse models.yaml: %w", err)
	}
	if len(s.Models) == 0 {
		return nil, fmt.Errorf("spec: models.yaml declares no models")
	}
	if _, ok := s.Models[s.DefaultModel]; !ok {
		return nil, fmt.Errorf("spec: default_model %q is not defined in models", s.DefaultModel)
	}
	if s.SpeechContext.PromptPrefix == "" {
		return nil, fmt.Errorf("spec: speech_context.prompt_prefix must not be empty")
	}
	if len(s.SpeechContext.Terms) == 0 {
		return nil, fmt.Errorf("spec: speech_context.terms must not be empty")
	}
	if s.SpeechContext.MaxTerms <= 0 || s.SpeechContext.MaxChars <= 0 || s.SpeechContext.MaxTermChars <= 0 {
		return nil, fmt.Errorf("spec: speech_context limits must be positive")
	}
	if s.TranscriptSafety.MaxChars <= 0 || s.TranscriptSafety.MaxTokenChars <= 0 || s.TranscriptSafety.MaxRepeatUnitChars <= 0 || s.TranscriptSafety.MinRepeatCount <= 1 || s.TranscriptSafety.MinRepeatedChars <= 0 {
		return nil, fmt.Errorf("spec: transcript_safety limits must be positive and min_repeat_count must exceed one")
	}
	for name, m := range s.Models {
		seen := make(map[string]bool)
		for _, word := range m.StopWords {
			if word.ID == "" || word.Pattern == "" {
				return nil, fmt.Errorf("spec: model %q has stop_word without id or pattern", name)
			}
			if seen[word.ID] {
				return nil, fmt.Errorf("spec: model %q has duplicate stop_word id %q", name, word.ID)
			}
			seen[word.ID] = true
		}
		if m.CPUFallback == "" {
			continue
		}
		fallback, ok := s.Models[m.CPUFallback]
		if !ok {
			return nil, fmt.Errorf("spec: model %q cpu_fallback %q is not defined in models", name, m.CPUFallback)
		}
		if fallback.RequiresGPU {
			return nil, fmt.Errorf("spec: model %q cpu_fallback %q itself requires_gpu; fallback chains are not supported", name, m.CPUFallback)
		}
	}
	return &s, nil
}

// StopWords returns the hallucination stop-word patterns for name, falling
// back to the default model's list if name is unknown or empty.
func (s *ModelSpec) StopWords(name string) []string {
	words := s.BuiltinStopWords(name)
	patterns := make([]string, 0, len(words))
	for _, word := range words {
		patterns = append(patterns, word.Pattern)
	}
	return patterns
}

// BuiltinStopWords returns the shipped rules, including stable IDs.
func (s *ModelSpec) BuiltinStopWords(name string) []StopWord {
	if m, ok := s.Models[name]; ok {
		return m.StopWords
	}
	return s.Models[s.DefaultModel].StopWords
}

// AllowsCPU reports whether name may run on the CPU backend. Models with
// requires_gpu: true (see spec/models.yaml) must not; an unknown name is
// treated as allowed since it carries no such restriction.
func (s *ModelSpec) AllowsCPU(name string) bool {
	m, ok := s.Models[name]
	return !ok || !m.RequiresGPU
}

// ResolveModel returns the model to actually use given whether a GPU is
// available. Requiring a GPU for one model does not forbid running at all
// without one: if name requires_gpu and gpuAvailable is false, it resolves
// to that model's cpu_fallback instead of failing. It only errors if
// name requires_gpu, no GPU is available, and no cpu_fallback is
// configured for it.
func (s *ModelSpec) ResolveModel(name string, gpuAvailable bool) (resolved string, usedFallback bool, err error) {
	m, ok := s.Models[name]
	if !ok || !m.RequiresGPU || gpuAvailable {
		return name, false, nil
	}
	if m.CPUFallback == "" {
		return "", false, fmt.Errorf("model %q requires GPU acceleration and has no cpu_fallback configured in spec/models.yaml", name)
	}
	return m.CPUFallback, true, nil
}

// IsWhisperEngine reports whether name resolves to the "whisper" engine
// (including the legacy empty engine value, which means whisper). An
// unknown name reports false rather than panicking on a missing map entry.
func (s *ModelSpec) IsWhisperEngine(name string) bool {
	m, ok := s.Models[name]
	if !ok {
		return false
	}
	return m.Engine == "" || m.Engine == "whisper"
}

// DefaultWhisperModel returns the model name for tooling that is hard-wired
// to drive voxtype specifically (internal/devsample's raw ASR sampler, the
// debug VAD probe, internal/bench) rather than whatever engine DefaultModel
// currently resolves to -- which, since issue 074, may be a non-whisper
// engine such as cohere-transcribe. It never returns a model whose engine is
// not whisper. It prefers "small.en", the pre-Cohere eager default (see the
// speech_context comment in spec/models.yaml), and falls back to the first
// whisper-engine model name in sorted order so a spec that ever drops
// small.en still resolves deterministically instead of silently handing a
// non-whisper model name to voxtype (see issue 077).
func (s *ModelSpec) DefaultWhisperModel() (string, error) {
	if s.IsWhisperEngine("small.en") {
		return "small.en", nil
	}
	var names []string
	for name := range s.Models {
		if s.IsWhisperEngine(name) {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "", fmt.Errorf("spec: no whisper-engine model configured in models.yaml")
	}
	sort.Strings(names)
	return names[0], nil
}

// Names returns all configured model names.
func (s *ModelSpec) Names() []string {
	names := make([]string, 0, len(s.Models))
	for name := range s.Models {
		names = append(names, name)
	}
	return names
}
