package spec

import (
	"testing"
	"time"
)

// validDrain is appended to the ad hoc modifier_gate documents below so a
// rejection is attributable to the field under test rather than to the
// unrelated, now also-required drain section.
const validDrain = "\ndrain: {delivery_deadline_ms: 60000, injection_timeout_ms: 15000}\n"

func TestLoadEager(t *testing.T) {
	s, err := LoadEager()
	if err != nil {
		t.Fatalf("LoadEager() error = %v", err)
	}
	if s.ModifierGate.TimeoutMs <= 0 {
		t.Errorf("ModifierGate.TimeoutMs = %v, want positive", s.ModifierGate.TimeoutMs)
	}
	if s.ModifierGate.StartGraceMs < 0 {
		t.Errorf("ModifierGate.StartGraceMs = %v, want non-negative", s.ModifierGate.StartGraceMs)
	}
	if s.ModifierGate.NotifyDelayMs < 0 {
		t.Errorf("ModifierGate.NotifyDelayMs = %v, want non-negative", s.ModifierGate.NotifyDelayMs)
	}
	if got, want := s.ModifierTimeout().Milliseconds(), int64(s.ModifierGate.TimeoutMs); got != want {
		t.Errorf("ModifierTimeout() = %vms, want %vms", got, want)
	}
	if got, want := s.ModifierStartGrace().Milliseconds(), int64(s.ModifierGate.StartGraceMs); got != want {
		t.Errorf("ModifierStartGrace() = %vms, want %vms", got, want)
	}
	if got, want := s.ModifierNotifyDelay().Milliseconds(), int64(s.ModifierGate.NotifyDelayMs); got != want {
		t.Errorf("ModifierNotifyDelay() = %vms, want %vms", got, want)
	}
	if got, want := s.DeliveryDeadline().Milliseconds(), int64(s.Drain.DeliveryDeadlineMs); got != want {
		t.Errorf("DeliveryDeadline() = %vms, want %vms", got, want)
	}
	if got, want := s.InjectionTimeout().Milliseconds(), int64(s.Drain.InjectionTimeoutMs); got != want {
		t.Errorf("InjectionTimeout() = %vms, want %vms", got, want)
	}
	// Issue 115: the delivery deadline is an absolute safety net, so it must
	// stay clear of the 30s per-job transcription timeout plus an injection.
	if s.DeliveryDeadline() <= 30*time.Second+s.InjectionTimeout() {
		t.Errorf("DeliveryDeadline() = %v, want more than one 30s transcription plus one injection (%v)", s.DeliveryDeadline(), s.InjectionTimeout())
	}
	if s.InjectionTimeout() <= 5*time.Second {
		t.Errorf("InjectionTimeout() = %v, want more than the 5s modifier-release wait", s.InjectionTimeout())
	}
}

func TestParseEagerSpecRejectsNonPositiveTimeout(t *testing.T) {
	cases := []string{
		`modifier_gate: {timeout_ms: 0, start_grace_ms: 750, notify_delay_ms: 500}`,
		`modifier_gate: {timeout_ms: -1, start_grace_ms: 750, notify_delay_ms: 500}`,
	}
	for _, doc := range cases {
		if _, err := parseEagerSpec([]byte(doc + validDrain)); err == nil {
			t.Errorf("parseEagerSpec(%q) expected an error, got nil", doc)
		}
	}
}

func TestParseEagerSpecRejectsNegativeStartGrace(t *testing.T) {
	doc := `modifier_gate: {timeout_ms: 10000, start_grace_ms: -1, notify_delay_ms: 500}`
	if _, err := parseEagerSpec([]byte(doc + validDrain)); err == nil {
		t.Errorf("parseEagerSpec(%q) expected an error, got nil", doc)
	}
}

func TestParseEagerSpecRejectsNegativeNotifyDelay(t *testing.T) {
	doc := `modifier_gate: {timeout_ms: 10000, start_grace_ms: 750, notify_delay_ms: -1}`
	if _, err := parseEagerSpec([]byte(doc + validDrain)); err == nil {
		t.Errorf("parseEagerSpec(%q) expected an error, got nil", doc)
	}
}

// TestParseEagerSpecRejectsUnsafeDrain guards issue 115's two invariants: a
// non-positive safety net would drop every post-stop delivery, and an
// injection budget at or below the 5s modifier-release wait could expire
// before a single keystroke is sent.
func TestParseEagerSpecRejectsUnsafeDrain(t *testing.T) {
	const gate = "modifier_gate: {timeout_ms: 10000, start_grace_ms: 750, notify_delay_ms: 500}\n"
	cases := map[string]string{
		"zero deadline":           "drain: {delivery_deadline_ms: 0, injection_timeout_ms: 15000}",
		"negative deadline":       "drain: {delivery_deadline_ms: -1, injection_timeout_ms: 15000}",
		"missing drain":           "",
		"injection equal to wait": "drain: {delivery_deadline_ms: 60000, injection_timeout_ms: 5000}",
		"injection below wait":    "drain: {delivery_deadline_ms: 60000, injection_timeout_ms: 1000}",
	}
	for name, drain := range cases {
		if _, err := parseEagerSpec([]byte(gate + drain)); err == nil {
			t.Errorf("parseEagerSpec(%s) expected an error, got nil", name)
		}
	}
}
