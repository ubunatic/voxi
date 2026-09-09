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
	if got, want := s.ModifierTimeout().Milliseconds(), int64(s.ModifierGate.TimeoutMs); got != want {
		t.Errorf("ModifierTimeout() = %vms, want %vms", got, want)
	}
}

func TestParseEagerSpecRejectsNonPositiveTimeout(t *testing.T) {
	cases := []string{
		`modifier_gate: {timeout_ms: 0}`,
		`modifier_gate: {timeout_ms: -1}`,
	}
	for _, doc := range cases {
		if _, err := parseEagerSpec([]byte(doc)); err == nil {
			t.Errorf("parseEagerSpec(%q) expected an error, got nil", doc)
		}
	}
}
