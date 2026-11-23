package bench

import (
	"fmt"

	"ubunatic.com/voxi/internal/feedback"
)

// Replacement modes for Options.Replacements. They select which transcript
// corrections (voxi feedback replacement) are applied to each result's text.
const (
	ReplacementsNone   = "none"
	ReplacementsJargon = "jargon"
	ReplacementsAll    = "all"
)

// NormalizeReplacementMode returns the replacement mode for mode. The empty
// string means ReplacementsNone; any other value must be exactly one of the
// named modes.
func NormalizeReplacementMode(mode string) (string, error) {
	switch mode {
	case "", ReplacementsNone:
		return ReplacementsNone, nil
	case ReplacementsJargon, ReplacementsAll:
		return mode, nil
	}
	return "", fmt.Errorf("invalid --replacements %q: use %q, %q, or %q", mode, ReplacementsNone, ReplacementsJargon, ReplacementsAll)
}

// loadReplacementRules returns the rules a mode may apply. ReplacementsNone
// loads nothing, so raw-ASR baselines never read the replacement dictionary.
func loadReplacementRules(mode, home string) ([]feedback.Replacement, error) {
	if mode == ReplacementsNone {
		return nil, nil
	}
	if home == "" {
		return nil, fmt.Errorf("--replacements %s needs a home directory to read replacements.json", mode)
	}
	rules, err := feedback.LoadReplacements(feedback.ReplacementPath(home))
	if err != nil {
		return nil, fmt.Errorf("load replacements: %w", err)
	}
	return rules, nil
}

// applyReplacementMode returns text with the mode's rules applied.
// ReplacementsJargon applies only jargon-category rules, so repair and
// uncategorized rules are skipped. ReplacementsAll applies every rule, as
// live dictation does. ReplacementsNone returns text unchanged.
func applyReplacementMode(text, mode string, rules []feedback.Replacement) string {
	switch mode {
	case ReplacementsJargon:
		out, _ := feedback.ApplyReplacementsByCategory(text, rules, feedback.CategoryJargon)
		return out
	case ReplacementsAll:
		out, _ := feedback.ApplyReplacementsWithAudit(text, rules)
		return out
	}
	return text
}

// applyReplacementsToReport rewrites the Text of every successful result.
// Failed rows have no transcript and are left alone.
func applyReplacementsToReport(report *Report, mode string, rules []feedback.Replacement) {
	if mode == ReplacementsNone {
		return
	}
	for i := range report.Results {
		if report.Results[i].Error == "" {
			report.Results[i].Text = applyReplacementMode(report.Results[i].Text, mode, rules)
		}
	}
}
