package feedback

import (
	"fmt"
	"io"
	"strings"

	"ubunatic.com/voxi/spec"
)

// previewLimit bounds how many items each category prints before the
// summary falls back to "... and N more" plus a pointer to the full list
// subcommand.
const previewLimit = 5

// StopWordRow describes one active or disabled stop-word rule, combining
// built-in and user-added entries in the same order and shape used by
// `voxi feedback stop-word list`.
type StopWordRow struct {
	State   string // "built-in", "disabled", or "user"
	ID      string
	Pattern string
}

// StopWordRows returns built-in and user stop-word rules combined. It is the
// single read/format path shared by `voxi feedback stop-word list` and
// `voxi feedback status`.
func StopWordRows(o Overrides, builtins []spec.StopWord) []StopWordRow {
	disabled := map[string]bool{}
	for _, id := range o.Disabled {
		disabled[id] = true
	}
	rows := make([]StopWordRow, 0, len(builtins)+len(o.User))
	for _, r := range builtins {
		state := "built-in"
		if disabled[r.ID] {
			state = "disabled"
		}
		rows = append(rows, StopWordRow{State: state, ID: r.ID, Pattern: r.Pattern})
	}
	for _, p := range o.User {
		rows = append(rows, StopWordRow{State: "user", ID: "-", Pattern: p})
	}
	return rows
}

// VocabularySources describes where the speech-context vocabulary (on by
// default for small.en, disable per invocation with --speech-context=false)
// currently resolves from, in the priority order applied by
// speechcontext.Build: explicit file terms first, then the shipped static
// spec terms, then best-effort repository-derived terms.
type VocabularySources struct {
	ExplicitFilePresent bool
	ExplicitFileTerms   int
	StaticSpecTerms     int
}

// Summary aggregates the locally stored assistive-feedback state that
// `voxi feedback status` reports. It holds no file-reading logic itself —
// callers populate it from the existing per-category read functions
// (Load, speechcontext.LoadVocabulary) so no parsing is duplicated here.
type Summary struct {
	StopWords              []StopWordRow
	SilenceArtifacts       []string
	VocabularyTerms        []string
	SpeechContextDefaultOn bool
	VocabularySources      VocabularySources
}

// BuildSummary composes an already-loaded Overrides, builtin stop-word
// specs, and vocabulary state into a Summary. It performs no I/O.
func BuildSummary(o Overrides, builtins []spec.StopWord, vocabularyTerms []string, sources VocabularySources, speechContextDefaultOn bool) Summary {
	return Summary{
		StopWords:              StopWordRows(o, builtins),
		SilenceArtifacts:       append([]string(nil), o.SilenceArtifacts...),
		VocabularyTerms:        append([]string(nil), vocabularyTerms...),
		SpeechContextDefaultOn: speechContextDefaultOn,
		VocabularySources:      sources,
	}
}

// FormatSummary writes a compact, human-readable overview of s to w.
func FormatSummary(w io.Writer, s Summary) error {
	var b strings.Builder

	activeBuiltins, disabledBuiltins, userWords := 0, 0, 0
	for _, r := range s.StopWords {
		switch r.State {
		case "built-in":
			activeBuiltins++
		case "disabled":
			disabledBuiltins++
		case "user":
			userWords++
		}
	}
	total := activeBuiltins + userWords

	fmt.Fprintf(&b, "Stop-words: %d active (%d built-in, %d user), %d built-in disabled\n", total, activeBuiltins, userWords, disabledBuiltins)
	writePreview(&b, stopWordLines(s.StopWords), "voxi feedback stop-word list")

	fmt.Fprintf(&b, "\nSilence artifacts: %d\n", len(s.SilenceArtifacts))
	writePreview(&b, s.SilenceArtifacts, "voxi feedback silence-artifact list")

	fmt.Fprintf(&b, "\nVocabulary terms: %d\n", len(s.VocabularyTerms))
	writePreview(&b, s.VocabularyTerms, "voxi feedback vocabulary list")

	fmt.Fprintf(&b, "\nSpeech-context (--speech-context): %s by default for Whisper small.en; not used by default Cohere; disable per eager invocation with --speech-context=false\n", onOff(s.SpeechContextDefaultOn))
	fmt.Fprintln(&b, "  vocabulary resolves from, in priority order:")
	fmt.Fprintf(&b, "    1. explicit file (~/.config/voxi/vocabulary.txt): %s, %d term(s)\n", presence(s.VocabularySources.ExplicitFilePresent), s.VocabularySources.ExplicitFileTerms)
	fmt.Fprintf(&b, "    2. static spec terms (spec/models.yaml speech_context.terms): %d term(s) available\n", s.VocabularySources.StaticSpecTerms)
	fmt.Fprintln(&b, "    3. repository-derived (git repo/file basenames): computed live at eager runtime, not evaluated here")

	_, err := io.WriteString(w, b.String())
	return err
}

func stopWordLines(rows []StopWordRow) []string {
	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		lines = append(lines, fmt.Sprintf("%s\t%s\t%s", r.State, r.ID, r.Pattern))
	}
	return lines
}

func writePreview(b *strings.Builder, lines []string, listCmd string) {
	shown := lines
	truncated := false
	if len(shown) > previewLimit {
		shown = shown[:previewLimit]
		truncated = true
	}
	for _, line := range shown {
		fmt.Fprintf(b, "  %s\n", line)
	}
	if truncated {
		fmt.Fprintf(b, "  ... and %d more; see: %s\n", len(lines)-len(shown), listCmd)
	}
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func presence(v bool) string {
	if v {
		return "present"
	}
	return "absent"
}
