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
	for _, id := range []string{"speed", "hardware", "transcript", "daemons", "all", "quit"} {
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
