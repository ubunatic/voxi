// Package spec also loads spec/eager.yaml: the eager pipeline's
// injection-safety tuning — the modifier-release race guard's staleness window
// (issue 101) and the post-stop delivery drain bounds (issue 115). It is the
// single source of truth for that tuning — see docs/Spec.md. Application code
// must not hardcode values that duplicate or shadow it.
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
	TimeoutMs     int `yaml:"timeout_ms"`
	StartGraceMs  int `yaml:"start_grace_ms"`
	NotifyDelayMs int `yaml:"notify_delay_ms"`
}

// DrainSpec tunes the post-stop delivery drain (issue 115): how long an
// already-captured, already-transcribed utterance may still be typed after its
// session stopped, and how long one injection may take. The real "stop
// typing" signal is a newer session starting, not either of these values --
// see internal/eager's stopRequest.generation.
type DrainSpec struct {
	DeliveryDeadlineMs int `yaml:"delivery_deadline_ms"`
	InjectionTimeoutMs int `yaml:"injection_timeout_ms"`
}

// EagerSpec is the parsed contents of spec/eager.yaml.
type EagerSpec struct {
	ModifierGate ModifierGateSpec `yaml:"modifier_gate"`
	Drain        DrainSpec        `yaml:"drain"`
}

// minInjectionTimeoutMs is the physical-modifier-release wait inside
// typing.TypeTextObserved; an injection budget at or below it could expire
// before a single keystroke is ever sent.
const minInjectionTimeoutMs = 5000

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
	if s.ModifierGate.NotifyDelayMs < 0 {
		return nil, fmt.Errorf("spec: modifier_gate.notify_delay_ms must not be negative")
	}
	if s.Drain.DeliveryDeadlineMs <= 0 {
		return nil, fmt.Errorf("spec: drain.delivery_deadline_ms must be positive")
	}
	if s.Drain.InjectionTimeoutMs <= minInjectionTimeoutMs {
		return nil, fmt.Errorf("spec: drain.injection_timeout_ms must exceed %dms, the modifier-release wait inside typing injection", minInjectionTimeoutMs)
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

// ModifierNotifyDelay converts ModifierGate.NotifyDelayMs to a time.Duration.
func (s *EagerSpec) ModifierNotifyDelay() time.Duration {
	return time.Duration(s.ModifierGate.NotifyDelayMs) * time.Millisecond
}

// DeliveryDeadline converts Drain.DeliveryDeadlineMs to a time.Duration.
func (s *EagerSpec) DeliveryDeadline() time.Duration {
	return time.Duration(s.Drain.DeliveryDeadlineMs) * time.Millisecond
}

// InjectionTimeout converts Drain.InjectionTimeoutMs to a time.Duration.
func (s *EagerSpec) InjectionTimeout() time.Duration {
	return time.Duration(s.Drain.InjectionTimeoutMs) * time.Millisecond
}
