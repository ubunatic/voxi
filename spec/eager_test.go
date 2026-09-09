package spec

import "testing"

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
	if got, want := s.ModifierTimeout().Milliseconds(), int64(s.ModifierGate.TimeoutMs); got != want {
		t.Errorf("ModifierTimeout() = %vms, want %vms", got, want)
	}
	if got, want := s.ModifierStartGrace().Milliseconds(), int64(s.ModifierGate.StartGraceMs); got != want {
		t.Errorf("ModifierStartGrace() = %vms, want %vms", got, want)
	}
}

func TestParseEagerSpecRejectsNonPositiveTimeout(t *testing.T) {
	cases := []string{
		`modifier_gate: {timeout_ms: 0, start_grace_ms: 750}`,
		`modifier_gate: {timeout_ms: -1, start_grace_ms: 750}`,
	}
	for _, doc := range cases {
		if _, err := parseEagerSpec([]byte(doc)); err == nil {
			t.Errorf("parseEagerSpec(%q) expected an error, got nil", doc)
		}
	}
}

func TestParseEagerSpecRejectsNegativeStartGrace(t *testing.T) {
	doc := `modifier_gate: {timeout_ms: 10000, start_grace_ms: -1}`
	if _, err := parseEagerSpec([]byte(doc)); err == nil {
		t.Errorf("parseEagerSpec(%q) expected an error, got nil", doc)
	}
}
