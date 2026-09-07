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
	"unicode/utf8"
)

const replacementFileName = "replacements.json"

// Replacement is one exact heard-form to written-form correction.
type Replacement struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ReplacementPath returns the private user replacement dictionary path.
func ReplacementPath(home string) string {
	return filepath.Join(home, ".config", "voxi", replacementFileName)
}

// LoadReplacements reads and validates the replacement dictionary.
func LoadReplacements(path string) ([]Replacement, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read transcript replacements: %w", err)
	}
	var rules []Replacement
	if err := json.Unmarshal(b, &rules); err != nil {
		return nil, fmt.Errorf("parse transcript replacements: %w", err)
	}
	for i, rule := range rules {
		normalized, err := normalizeReplacement(rule.From, rule.To)
		if err != nil {
			return nil, fmt.Errorf("invalid stored replacement %d: %w", i+1, err)
		}
		rules[i] = normalized
	}
	return normalizeReplacements(rules), nil
}

// SaveReplacements atomically writes a deterministic, user-private dictionary.
func SaveReplacements(path string, rules []Replacement) error {
	rules = normalizeReplacements(rules)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create replacement directory: %w", err)
	}
	b, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return fmt.Errorf("encode transcript replacements: %w", err)
	}
	b = append(b, '\n')
	f, err := os.CreateTemp(filepath.Dir(path), ".replacements-*")
	if err != nil {
		return fmt.Errorf("create replacement file: %w", err)
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write transcript replacements: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace transcript replacements: %w", err)
	}
	return os.Chmod(path, 0600)
}

// AddReplacement adds a unique exact-case source mapping.
func AddReplacement(rules []Replacement, from, to string) ([]Replacement, Replacement, error) {
	rule, err := normalizeReplacement(from, to)
	if err != nil {
		return rules, Replacement{}, err
	}
	for _, existing := range rules {
		if existing.From == rule.From {
			return rules, Replacement{}, fmt.Errorf("replacement source %q is already mapped to %q", rule.From, existing.To)
		}
	}
	rules = append(rules, rule)
	return normalizeReplacements(rules), rule, nil
}

// RemoveReplacement removes an exact-case source mapping.
func RemoveReplacement(rules []Replacement, from string) ([]Replacement, Replacement, error) {
	from = normalizeSpace(from)
	for i, rule := range rules {
		if rule.From == from {
			out := append([]Replacement(nil), rules[:i]...)
			out = append(out, rules[i+1:]...)
			return normalizeReplacements(out), rule, nil
		}
	}
	return rules, Replacement{}, fmt.Errorf("replacement source %q was not found (matching is case-sensitive)", from)
}

func normalizeReplacement(from, to string) (Replacement, error) {
	from = normalizeSpace(from)
	to = strings.TrimSpace(to)
	if from == "" {
		return Replacement{}, fmt.Errorf("replacement source must not be empty")
	}
	if to == "" {
		return Replacement{}, fmt.Errorf("replacement target must not be empty")
	}
	if utf8.RuneCountInString(from) > 200 || utf8.RuneCountInString(to) > 200 {
		return Replacement{}, fmt.Errorf("replacement source and target must each be at most 200 characters")
	}
	for _, value := range []string{from, to} {
		for _, r := range value {
			if unicode.IsControl(r) {
				return Replacement{}, fmt.Errorf("replacement source and target must not contain control characters")
			}
		}
	}
	return Replacement{From: from, To: to}, nil
}

func normalizeSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

func normalizeReplacements(rules []Replacement) []Replacement {
	out := append([]Replacement(nil), rules...)
	sort.Slice(out, func(i, j int) bool { return out[i].From < out[j].From })
	return out
}

type replacementMatch struct {
	start, end int
	rule       Replacement
}

// ApplyReplacements applies all exact-case rules to the original text once.
// Phrase spaces match any non-empty Unicode whitespace run. Matches embedded
// in Unicode words are rejected; punctuation remains adjacent and unchanged.
func ApplyReplacements(text string, rules []Replacement) string {
	var matches []replacementMatch
	for _, rule := range rules {
		parts := strings.Split(rule.From, " ")
		for i := range parts {
			parts[i] = regexp.QuoteMeta(parts[i])
		}
		re := regexp.MustCompile(strings.Join(parts, `\s+`))
		for _, loc := range re.FindAllStringIndex(text, -1) {
			if replacementBoundary(text, loc[0], loc[1]) {
				matches = append(matches, replacementMatch{loc[0], loc[1], rule})
			}
		}
	}
	if len(matches) == 0 {
		return text
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].start != matches[j].start {
			return matches[i].start < matches[j].start
		}
		li, lj := matches[i].end-matches[i].start, matches[j].end-matches[j].start
		if li != lj {
			return li > lj
		}
		return matches[i].rule.From < matches[j].rule.From
	})
	var b strings.Builder
	pos := 0
	for _, match := range matches {
		if match.start < pos {
			continue
		}
		b.WriteString(text[pos:match.start])
		b.WriteString(match.rule.To)
		pos = match.end
	}
	b.WriteString(text[pos:])
	return b.String()
}

func replacementBoundary(text string, start, end int) bool {
	word := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' }
	first, _ := utf8.DecodeRuneInString(text[start:end])
	last, _ := utf8.DecodeLastRuneInString(text[start:end])
	if start > 0 && word(first) {
		before, _ := utf8.DecodeLastRuneInString(text[:start])
		if word(before) {
			return false
		}
	}
	if end < len(text) && word(last) {
		after, _ := utf8.DecodeRuneInString(text[end:])
		if word(after) {
			return false
		}
	}
	return true
}
