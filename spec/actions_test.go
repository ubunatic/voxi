package spec

import "testing"

func TestLoadActions(t *testing.T) {
	s, err := LoadActions()
	if err != nil {
		t.Fatalf("LoadActions() error = %v", err)
	}
	for id, a := range s.Actions {
		if a.Title == "" {
			t.Errorf("action %q: title must not be empty", id)
		}
		if a.Short == "" {
			t.Errorf("action %q: short must not be empty", id)
		}
		if len(a.Keys) == 0 {
			t.Errorf("action %q: keys must not be empty", id)
		}
	}
	for _, id := range []string{"speed", "hardware", "transcript", "daemons", "all", "tts-play-pause", "tts-previous", "tts-next", "tts-stop", "tts-clear", "quit"} {
		if _, ok := s.Actions[id]; !ok {
			t.Errorf("expected action %q to be defined", id)
		}
	}
}

func TestKeyDispatch(t *testing.T) {
	s, err := LoadActions()
	if err != nil {
		t.Fatalf("LoadActions() error = %v", err)
	}
	dispatch := s.KeyDispatch()
	if dispatch['s'] != "speed" {
		t.Errorf(`dispatch['s'] = %q, want "speed"`, dispatch['s'])
	}
	if dispatch['q'] != "quit" {
		t.Errorf(`dispatch['q'] = %q, want "quit"`, dispatch['q'])
	}
	if dispatch[3] != "quit" {
		t.Errorf(`dispatch[3] (<c-c>) = %q, want "quit"`, dispatch[3])
	}
	if dispatch[27] != "quit" {
		t.Errorf(`dispatch[27] (<esc>) = %q, want "quit"`, dispatch[27])
	}
	if dispatch['m'] != "tts-play-pause" || dispatch['b'] != "tts-previous" || dispatch['n'] != "tts-next" || dispatch['x'] != "tts-stop" || dispatch['k'] != "tts-clear" {
		t.Errorf("TTS dispatch = m:%q b:%q n:%q x:%q k:%q", dispatch['m'], dispatch['b'], dispatch['n'], dispatch['x'], dispatch['k'])
	}
}

func TestParseActionSpecRejectsDuplicateKeyAcrossActions(t *testing.T) {
	yamlDoc := []byte(`
actions:
  a:
    title: "A"
    short: "a"
    keys: ["x"]
    category: "monitor"
  b:
    title: "B"
    short: "b"
    keys: ["x"]
    category: "monitor"
`)
	if _, err := parseActionSpec(yamlDoc); err == nil {
		t.Fatal("expected an error for a key shared by two actions, got nil")
	}
}

func TestParseActionSpecRejectsInvalidCategory(t *testing.T) {
	yamlDoc := []byte(`
actions:
  a:
    title: "A"
    short: "a"
    keys: ["x"]
    category: "bogus"
`)
	if _, err := parseActionSpec(yamlDoc); err == nil {
		t.Fatal("expected an error for an invalid category, got nil")
	}
}

func TestParseActionSpecRejectsUnsupportedKeyToken(t *testing.T) {
	yamlDoc := []byte(`
actions:
  a:
    title: "A"
    short: "a"
    keys: ["<f5>"]
    category: "monitor"
`)
	if _, err := parseActionSpec(yamlDoc); err == nil {
		t.Fatal("expected an error for an unsupported key token, got nil")
	}
}

func TestTokenDispatch(t *testing.T) {
	s, err := LoadActions()
	if err != nil {
		t.Fatalf("LoadActions() error = %v", err)
	}
	tokens := s.TokenDispatch()

	cases := []struct {
		token string
		want  string
	}{
		{"speed", "speed"},
		{"s", "speed"},
		{"1", "speed"},
		{"v", "speed"},
		{"hardware", "hardware"},
		{"h", "hardware"},
		{"2", "hardware"},
		{"g", "hardware"},
		{"transcript", "transcript"},
		{"t", "transcript"},
		{"3", "transcript"},
		{"daemons", "daemons"},
		{"d", "daemons"},
		{"4", "daemons"},
		{"p", "daemons"},
		{"compact", "compact"},
		{"c", "compact"},
		{"all", "all"},
		{"a", "all"},
		{"tts-box", "tts-box"},
		{"tts", "tts-box"},
		{"y", "tts-box"},
	}

	for _, tc := range cases {
		if got := tokens[tc.token]; got != tc.want {
			t.Errorf("tokens[%q] = %q, want %q", tc.token, got, tc.want)
		}
		if got, ok := s.ActionForToken(tc.token); !ok || got != tc.want {
			t.Errorf("ActionForToken(%q) = (%q, %v), want (%q, true)", tc.token, got, ok, tc.want)
		}
	}

	// Case insensitivity and whitespace trimming
	if got, ok := s.ActionForToken("  SPEED  "); !ok || got != "speed" {
		t.Errorf(`ActionForToken("  SPEED  ") = (%q, %v), want ("speed", true)`, got, ok)
	}
	if got, ok := s.ActionForToken("S"); !ok || got != "speed" {
		t.Errorf(`ActionForToken("S") = (%q, %v), want ("speed", true)`, got, ok)
	}

	// Unknown token
	if got, ok := s.ActionForToken("nonexistent"); ok || got != "" {
		t.Errorf(`ActionForToken("nonexistent") = (%q, %v), want ("", false)`, got, ok)
	}
}

func TestParseActionSpecRejectsConflictingTokens(t *testing.T) {
	yamlDoc := []byte(`
actions:
  speed:
    title: "Speed"
    short: "speed"
    keys: ["s"]
    category: "monitor"
  other:
    title: "Other"
    short: "speed"
    keys: ["o"]
    category: "monitor"
`)
	if _, err := parseActionSpec(yamlDoc); err == nil {
		t.Fatal("expected error for duplicate short token across actions, got nil")
	}
}
