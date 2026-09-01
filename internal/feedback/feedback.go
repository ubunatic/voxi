// Package feedback persists local, user-controlled ASR stop-word overrides.
package feedback

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"ubunatic.com/voxi/spec"
)

const fileName = "stop-words.json"

type Overrides struct {
	User     []string `json:"user,omitempty"`
	Disabled []string `json:"disabled_builtin,omitempty"`
}

func Path(home string) string { return filepath.Join(home, ".config", "voxi", fileName) }

func Load(path string) (Overrides, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Overrides{}, nil
	}
	if err != nil {
		return Overrides{}, fmt.Errorf("read stop-word feedback: %w", err)
	}
	var o Overrides
	if err := json.Unmarshal(b, &o); err != nil {
		return Overrides{}, fmt.Errorf("parse stop-word feedback: %w", err)
	}
	for _, phrase := range o.User {
		if err := validate(phrase); err != nil {
			return Overrides{}, fmt.Errorf("invalid stored stop word: %w", err)
		}
	}
	return normalize(o), nil
}

func Save(path string, o Overrides) error {
	o = normalize(o)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create feedback directory: %w", err)
	}
	b, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	f, err := os.CreateTemp(filepath.Dir(path), ".stop-words-*")
	if err != nil {
		return fmt.Errorf("create feedback file: %w", err)
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write feedback file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace feedback file: %w", err)
	}
	return os.Chmod(path, 0600)
}

func Add(o Overrides, phrase string) (Overrides, error) {
	if err := validate(phrase); err != nil {
		return o, err
	}
	phrase = strings.TrimSpace(phrase)
	for _, p := range o.User {
		if strings.EqualFold(p, phrase) {
			return o, fmt.Errorf("stop word %q is already present", phrase)
		}
	}
	o.User = append(o.User, phrase)
	return normalize(o), nil
}
func Remove(o Overrides, phrase string) (Overrides, error) {
	phrase = strings.TrimSpace(phrase)
	found := false
	out := o.User[:0]
	for _, p := range o.User {
		if strings.EqualFold(p, phrase) {
			found = true
		} else {
			out = append(out, p)
		}
	}
	if !found {
		return o, fmt.Errorf("user stop word %q was not found", phrase)
	}
	o.User = out
	return normalize(o), nil
}
func SetBuiltin(o Overrides, id string, enabled bool, builtins []spec.StopWord) (Overrides, error) {
	found := false
	for _, r := range builtins {
		if r.ID == id {
			found = true
			break
		}
	}
	if !found {
		return o, fmt.Errorf("built-in stop-word rule %q was not found", id)
	}
	if enabled {
		o.Disabled = removeFold(o.Disabled, id)
	} else {
		for _, x := range o.Disabled {
			if x == id {
				return o, nil
			}
		}
		o.Disabled = append(o.Disabled, id)
	}
	return normalize(o), nil
}
func ActivePatterns(builtins []spec.StopWord, o Overrides) []string {
	disabled := map[string]bool{}
	for _, id := range o.Disabled {
		disabled[id] = true
	}
	result := []string{}
	for _, r := range builtins {
		if !disabled[r.ID] {
			result = append(result, r.Pattern)
		}
	}
	for _, phrase := range o.User {
		result = append(result, literalPattern(phrase))
	}
	return result
}
func literalPattern(phrase string) string {
	// Go's RE2 engine has no look-ahead. Consume the conservative trailing
	// boundary instead; the ASR cleaner removes only matched trailing text.
	return `(?:^|[\s\p{P}])` + regexp.QuoteMeta(phrase) + `(?:$|[\s\p{P}])`
}
func validate(phrase string) error {
	phrase = strings.TrimSpace(phrase)
	if phrase == "" {
		return fmt.Errorf("stop word must not be empty")
	}
	if len([]rune(phrase)) > 200 {
		return fmt.Errorf("stop word must be at most 200 characters")
	}
	for _, r := range phrase {
		if unicode.IsControl(r) {
			return fmt.Errorf("stop word must not contain control characters")
		}
	}
	return nil
}
func normalize(o Overrides) Overrides {
	o.User = uniqueSorted(o.User, true)
	o.Disabled = uniqueSorted(o.Disabled, false)
	return o
}
func uniqueSorted(in []string, fold bool) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		key := s
		if fold {
			key = strings.ToLower(s)
		}
		if s != "" && !seen[key] {
			seen[key] = true
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}
func removeFold(in []string, target string) []string {
	out := in[:0]
	for _, x := range in {
		if x != target {
			out = append(out, x)
		}
	}
	return out
}
