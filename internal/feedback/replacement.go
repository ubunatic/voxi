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

// Replacement is one heard-form to written-form correction. Matching is
// case-insensitive on From. Unless FixedCase is set, the written form's
// case is adapted at apply time to match how it was heard (lower/UPPER/Title
// Case); FixedCase opts a rule out of that adaptation for targets that must
// always render exactly as stored regardless of context, such as domain
// names ("ubunatic.com" must stay lowercase even after a capitalized
// sentence-initial "Ubunatic.com").
type Replacement struct {
	From      string `json:"from"`
	To        string `json:"to"`
	FixedCase bool   `json:"fixed_case,omitempty"`
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
		normalized.FixedCase = rule.FixedCase
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

// AddReplacement adds a unique source mapping. Matching is case-insensitive,
// so "Voxy", "voxy", and "VOXY" are the same source: one rule covers every
// casing it is heard in (its written form adapts to match, unless
// fixedCase is set), and a case-only variant is rejected as a duplicate
// rather than needing its own entry.
func AddReplacement(rules []Replacement, from, to string, fixedCase bool) ([]Replacement, Replacement, error) {
	rule, err := normalizeReplacement(from, to)
	if err != nil {
		return rules, Replacement{}, err
	}
	rule.FixedCase = fixedCase
	for _, existing := range rules {
		if strings.EqualFold(existing.From, rule.From) {
			return rules, Replacement{}, fmt.Errorf("replacement source %q is already mapped to %q (as %q; matching is case-insensitive)", rule.From, existing.To, existing.From)
		}
	}
	rules = append(rules, rule)
	return normalizeReplacements(rules), rule, nil
}

// RemoveReplacement removes a source mapping. Matching is case-insensitive.
func RemoveReplacement(rules []Replacement, from string) ([]Replacement, Replacement, error) {
	from = normalizeSpace(from)
	for i, rule := range rules {
		if strings.EqualFold(rule.From, from) {
			out := append([]Replacement(nil), rules[:i]...)
			out = append(out, rules[i+1:]...)
			return normalizeReplacements(out), rule, nil
		}
	}
	return rules, Replacement{}, fmt.Errorf("replacement source %q was not found", from)
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

// DedupedReplacement records one duplicate dropped by DedupeReplacements.
type DedupedReplacement struct {
	Kept     Replacement // the entry retained in its place
	Dropped  Replacement // the case-variant entry removed
	Conflict bool        // true if Dropped.To differs from Kept.To (information was lost)
}

// DedupeReplacements collapses rules that now match the same source under
// case-insensitive matching (issue: dictionaries built before adaptive
// casing may hold "Voxy"->"Voxi" and "voxy"->"voxi" as separate, now-
// redundant entries -- one adaptive rule reproduces both outputs). Rules are
// grouped by case-insensitive From; within a group, entries whose To agrees
// case-insensitively are pure case variants and collapse for free (the
// survivor is the most informative casing: Title Case beats a flat
// upper/lower form, which beats an unclassifiable mixed form). If a group
// has more than one such To value, the larger class wins and the rest are
// reported as Conflict, since those entries disagree on the actual target,
// not just its casing.
func DedupeReplacements(rules []Replacement) (kept []Replacement, dropped []DedupedReplacement) {
	sorted := normalizeReplacements(rules)
	var order []string
	groups := make(map[string][]Replacement)
	for _, r := range sorted {
		key := strings.ToLower(r.From)
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], r)
	}
	for _, key := range order {
		group := groups[key]
		if len(group) == 1 {
			kept = append(kept, group[0])
			continue
		}
		counts := make(map[string]int, len(group))
		for _, r := range group {
			counts[strings.ToLower(r.To)]++
		}
		winnerClass := strings.ToLower(group[0].To)
		for _, r := range group {
			if c := strings.ToLower(r.To); counts[c] > counts[winnerClass] {
				winnerClass = c
			}
		}
		winner := -1
		for i, r := range group {
			if strings.ToLower(r.To) != winnerClass {
				continue
			}
			if winner == -1 || caseShapeOf(r.To) > caseShapeOf(group[winner].To) {
				winner = i
			}
		}
		kept = append(kept, group[winner])
		for i, r := range group {
			if i == winner {
				continue
			}
			dropped = append(dropped, DedupedReplacement{Kept: group[winner], Dropped: r, Conflict: strings.ToLower(r.To) != winnerClass})
		}
	}
	return kept, dropped
}

func normalizeReplacements(rules []Replacement) []Replacement {
	out := append([]Replacement(nil), rules...)
	sort.Slice(out, func(i, j int) bool { return out[i].From < out[j].From })
	return out
}

// caseShape classifies the letter-casing pattern of a matched phrase so its
// replacement can be re-cased to match.
type caseShape int

const (
	caseUnknown caseShape = iota // no letters, or a pattern that doesn't fit lower/upper/title
	caseFlat                     // all-lowercase or all-UPPERCASE
	caseTitle                    // Each Word Capitalized
)

// caseShapeOf ranks how informative a casing pattern is when picking a
// canonical survivor among duplicate replacements: Title Case preserves the
// most structure, a flat upper/lower form less, and an unclassifiable mixed
// form (e.g. "macOS") the least, since re-deriving it from scratch is unsafe.
func caseShapeOf(s string) caseShape { return detectCaseShape(s) }

// detectCaseShape reports whether s is all-lowercase, all-UPPERCASE, Title
// Case (each word's first letter capitalized, the rest lowercase), or none
// of those.
func detectCaseShape(s string) caseShape {
	hasLetter, allUpper, allLower := false, true, true
	for _, r := range s {
		if !unicode.IsLetter(r) {
			continue
		}
		hasLetter = true
		if unicode.IsUpper(r) {
			allLower = false
		}
		if unicode.IsLower(r) {
			allUpper = false
		}
	}
	if !hasLetter {
		return caseUnknown
	}
	if allUpper || allLower {
		return caseFlat
	}
	if isTitleCase(s) {
		return caseTitle
	}
	return caseUnknown
}

func isTitleCase(s string) bool {
	atWordStart := true
	for _, r := range s {
		if !unicode.IsLetter(r) {
			atWordStart = true
			continue
		}
		if atWordStart {
			if !unicode.IsUpper(r) {
				return false
			}
		} else if !unicode.IsLower(r) {
			return false
		}
		atWordStart = false
	}
	return true
}

// adaptCase re-cases to (the stored, canonical written form) to match the
// casing pattern detected in heard (the matched text as it actually
// appeared). Flat and Title patterns are reproduced exactly; an
// unclassifiable heard pattern falls back to to's own stored casing.
func adaptCase(to, heard string) string {
	hasUpper, hasLower := false, false
	for _, r := range heard {
		if unicode.IsUpper(r) {
			hasUpper = true
		}
		if unicode.IsLower(r) {
			hasLower = true
		}
	}
	switch detectCaseShape(heard) {
	case caseFlat:
		if hasUpper && !hasLower {
			return strings.ToUpper(to)
		}
		return strings.ToLower(to)
	case caseTitle:
		return titleCase(to)
	default:
		return to
	}
}

// titleCase capitalizes the first letter of every word in s and lowercases
// the rest, preserving s's own punctuation and spacing.
func titleCase(s string) string {
	var b strings.Builder
	atWordStart := true
	for _, r := range s {
		if !unicode.IsLetter(r) {
			b.WriteRune(r)
			atWordStart = true
			continue
		}
		if atWordStart {
			b.WriteRune(unicode.ToUpper(r))
		} else {
			b.WriteRune(unicode.ToLower(r))
		}
		atWordStart = false
	}
	return b.String()
}

type replacementMatch struct {
	start, end int
	rule       Replacement
}

// ApplyReplacements applies all rules to the original text once, matching
// the From phrase case-insensitively (so one stored rule covers any casing
// it is heard in). Unless a rule has FixedCase set, To's own casing is
// re-derived to match how the phrase was actually heard -- "voxy" and
// "Voxy" against a "voxy"->"voxi" rule become "voxi" and "Voxi"
// respectively -- so a single rule reproduces what used to require a
// separate entry per casing. FixedCase rules (domains and the like) always
// emit To exactly as stored. Phrase spaces match any non-empty Unicode
// whitespace run. Matches embedded in Unicode words are rejected;
// punctuation remains adjacent and unchanged.
func ApplyReplacements(text string, rules []Replacement) string {
	res, _ := ApplyReplacementsWithAudit(text, rules)
	return res
}

// ApplyReplacementsWithAudit behaves like ApplyReplacements, but additionally
// returns the slice of rules that actually matched and were applied to the text.
func ApplyReplacementsWithAudit(text string, rules []Replacement) (string, []Replacement) {
	var matches []replacementMatch
	for _, rule := range rules {
		parts := strings.Split(rule.From, " ")
		for i := range parts {
			parts[i] = regexp.QuoteMeta(parts[i])
		}
		re := regexp.MustCompile(`(?i)` + strings.Join(parts, `\s+`))
		for _, loc := range re.FindAllStringIndex(text, -1) {
			if replacementBoundary(text, loc[0], loc[1]) {
				matches = append(matches, replacementMatch{loc[0], loc[1], rule})
			}
		}
	}
	if len(matches) == 0 {
		return text, nil
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
	var applied []Replacement
	for _, match := range matches {
		if match.start < pos {
			continue
		}
		b.WriteString(text[pos:match.start])
		repl := match.rule.To
		if !match.rule.FixedCase {
			repl = adaptCase(repl, text[match.start:match.end])
		}
		b.WriteString(repl)
		pos = match.end
		applied = append(applied, match.rule)
	}
	b.WriteString(text[pos:])
	return b.String(), applied
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
