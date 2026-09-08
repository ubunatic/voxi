// Package spec also loads spec/actions.yaml: the single-key hotkeys `voxi
// monitor -w` (internal/monitor) reads from raw-mode /dev/tty, plus their
// display titles/short labels. It is the single source of truth for those
// hotkeys — see docs/Spec.md. Application code must not hardcode key
// literals that duplicate or shadow this list.
package spec

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

//go:embed actions.yaml
var actionsYAML []byte

// Action describes one voxi monitor TUI hotkey action.
type Action struct {
	Title    string   `yaml:"title"`
	Short    string   `yaml:"short"`
	Keys     []string `yaml:"keys"`
	Category string   `yaml:"category"`
}

// ActionSpec is the parsed contents of spec/actions.yaml.
type ActionSpec struct {
	Actions map[string]Action `yaml:"actions"`
}

// LoadActions parses the embedded action spec.
func LoadActions() (*ActionSpec, error) {
	return parseActionSpec(actionsYAML)
}

// parseActionSpec unmarshals and validates an actions.yaml document. Split
// out from LoadActions so tests can exercise validation against ad hoc YAML
// without touching the embedded spec.
func parseActionSpec(data []byte) (*ActionSpec, error) {
	var s ActionSpec
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("spec: parse actions.yaml: %w", err)
	}
	if len(s.Actions) == 0 {
		return nil, fmt.Errorf("spec: actions.yaml declares no actions")
	}
	seenKeys := make(map[string]string)
	for id, a := range s.Actions {
		if a.Title == "" {
			return nil, fmt.Errorf("spec: action %q missing title", id)
		}
		if a.Short == "" {
			return nil, fmt.Errorf("spec: action %q missing short label", id)
		}
		if a.Category != "monitor" && a.Category != "system" {
			return nil, fmt.Errorf("spec: action %q has invalid category %q", id, a.Category)
		}
		if len(a.Keys) == 0 {
			return nil, fmt.Errorf("spec: action %q has no keys", id)
		}
		for _, k := range a.Keys {
			if _, err := actionKeyByte(k); err != nil {
				return nil, fmt.Errorf("spec: action %q: %w", id, err)
			}
			if other, ok := seenKeys[k]; ok && other != id {
				return nil, fmt.Errorf("spec: key %q used by both action %q and %q", k, other, id)
			}
			seenKeys[k] = id
		}
	}
	return &s, nil
}

// actionKeyByte resolves one key token — a single printable character, or
// the <c-c>/<esc> control-key tokens — to the raw byte a raw-mode tty read
// produces for it.
func actionKeyByte(token string) (byte, error) {
	switch token {
	case "<c-c>":
		return 3, nil
	case "<esc>":
		return 27, nil
	default:
		if len(token) != 1 {
			return 0, fmt.Errorf("unsupported key token %q (want a single character or <c-c>/<esc>)", token)
		}
		return token[0], nil
	}
}

// KeyDispatch resolves every action's keys to raw tty bytes, returning a map
// from byte to action id. Panics only on a malformed embedded spec (parsed
// once at process start, and covered by spec/actions_test.go's
// TestLoadActions — this cannot happen in a build that passed `make check`).
func (s *ActionSpec) KeyDispatch() map[byte]string {
	dispatch := make(map[byte]string)
	for id, a := range s.Actions {
		for _, k := range a.Keys {
			b, err := actionKeyByte(k)
			if err != nil {
				panic(fmt.Sprintf("spec: action %q: %v", id, err))
			}
			dispatch[b] = id
		}
	}
	return dispatch
}
