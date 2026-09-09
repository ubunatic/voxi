// Package spec also loads spec/eager.yaml: the eager pipeline's
// injection-safety tuning, currently the modifier-release race guard's
// staleness window (issue 101). It is the single source of truth for that
// tuning — see docs/Spec.md. Application code must not hardcode values that
// duplicate or shadow it.
package spec

import (
	_ "embed"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed eager.yaml
var eagerYAML []byte

// ModifierGateSpec tunes the modifier-release buffering guard (issue 101):
// while a gating modifier is held, eager output is buffered rather than
// typed immediately, since the release that ends gating can simultaneously
// trigger unrelated WM/DE behavior that steals focus.
type ModifierGateSpec struct {
	TimeoutMs    int `yaml:"timeout_ms"`
	StartGraceMs int `yaml:"start_grace_ms"`
}

// EagerSpec is the parsed contents of spec/eager.yaml.
type EagerSpec struct {
	ModifierGate ModifierGateSpec `yaml:"modifier_gate"`
}

// LoadEager parses the embedded eager pipeline tuning spec.
func LoadEager() (*EagerSpec, error) {
	return parseEagerSpec(eagerYAML)
}

// parseEagerSpec unmarshals and validates an eager.yaml document. Split out
// from LoadEager so tests can exercise validation against ad hoc YAML
// without touching the embedded spec.
func parseEagerSpec(data []byte) (*EagerSpec, error) {
	var s EagerSpec
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("spec: parse eager.yaml: %w", err)
	}
	if s.ModifierGate.TimeoutMs <= 0 {
		return nil, fmt.Errorf("spec: modifier_gate.timeout_ms must be positive")
	}
	if s.ModifierGate.StartGraceMs < 0 {
		return nil, fmt.Errorf("spec: modifier_gate.start_grace_ms must not be negative")
	}
	return &s, nil
}

// ModifierTimeout converts ModifierGate.TimeoutMs to a time.Duration.
func (s *EagerSpec) ModifierTimeout() time.Duration {
	return time.Duration(s.ModifierGate.TimeoutMs) * time.Millisecond
}

// ModifierStartGrace converts ModifierGate.StartGraceMs to a time.Duration.
func (s *EagerSpec) ModifierStartGrace() time.Duration {
	return time.Duration(s.ModifierGate.StartGraceMs) * time.Millisecond
}
