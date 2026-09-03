package feedback

import (
	"strings"
	"testing"

	"ubunatic.com/voxi/spec"
)

func testBuiltins() []spec.StopWord {
	return []spec.StopWord{
		{ID: "hallucination-1", Pattern: "thank you for watching"},
		{ID: "hallucination-2", Pattern: "subscribe"},
	}
}

func TestBuildSummaryEmptyState(t *testing.T) {
	s := BuildSummary(Overrides{}, testBuiltins(), nil, VocabularySources{}, false)
	if len(s.StopWords) != 2 {
		t.Fatalf("expected 2 built-in rows, got %d", len(s.StopWords))
	}
	if len(s.SilenceArtifacts) != 0 || len(s.VocabularyTerms) != 0 {
		t.Fatalf("expected empty silence artifacts and vocabulary, got %+v", s)
	}

	var b strings.Builder
	if err := FormatSummary(&b, s); err != nil {
		t.Fatalf("FormatSummary: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, "Stop-words: 2 active (2 built-in, 0 user), 0 built-in disabled") {
		t.Fatalf("missing stop-word summary line: %s", out)
	}
	if !strings.Contains(out, "Silence artifacts: 0") || !strings.Contains(out, "Vocabulary terms: 0") {
		t.Fatalf("missing zero-count lines: %s", out)
	}
	if !strings.Contains(out, "off by default") {
		t.Fatalf("expected speech-context reported off by default: %s", out)
	}
	if !strings.Contains(out, "explicit file (~/.config/voxi/vocabulary.txt): absent, 0 term(s)") {
		t.Fatalf("expected absent explicit vocabulary file: %s", out)
	}
}

func TestBuildSummaryPopulatedState(t *testing.T) {
	o := Overrides{
		User:             []string{"um", "uh"},
		Disabled:         []string{"hallucination-2"},
		SilenceArtifacts: []string{"thank you.", "bye."},
	}
	sources := VocabularySources{ExplicitFilePresent: true, ExplicitFileTerms: 3, StaticSpecTerms: 42}
	s := BuildSummary(o, testBuiltins(), []string{"kubectl", "grep", "yaml"}, sources, true)

	var b strings.Builder
	if err := FormatSummary(&b, s); err != nil {
		t.Fatalf("FormatSummary: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, "Stop-words: 3 active (1 built-in, 2 user), 1 built-in disabled") {
		t.Fatalf("unexpected stop-word summary: %s", out)
	}
	if !strings.Contains(out, "Silence artifacts: 2") {
		t.Fatalf("unexpected silence-artifact count: %s", out)
	}
	if !strings.Contains(out, "Vocabulary terms: 3") {
		t.Fatalf("unexpected vocabulary count: %s", out)
	}
	if !strings.Contains(out, "on by default") {
		t.Fatalf("expected speech-context reported on: %s", out)
	}
	if !strings.Contains(out, "explicit file (~/.config/voxi/vocabulary.txt): present, 3 term(s)") {
		t.Fatalf("expected present explicit vocabulary file: %s", out)
	}
	if !strings.Contains(out, "static spec terms (spec/models.yaml speech_context.terms): 42 term(s) available") {
		t.Fatalf("expected static spec term count: %s", out)
	}
}

func TestBuildSummaryPartialStateVocabularyWithoutStopWords(t *testing.T) {
	s := BuildSummary(Overrides{}, nil, []string{"postgres", "redis"}, VocabularySources{}, false)
	if len(s.StopWords) != 0 {
		t.Fatalf("expected no stop-word rows when no builtins/overrides, got %+v", s.StopWords)
	}

	var b strings.Builder
	if err := FormatSummary(&b, s); err != nil {
		t.Fatalf("FormatSummary: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, "Stop-words: 0 active (0 built-in, 0 user), 0 built-in disabled") {
		t.Fatalf("expected zero stop-word summary: %s", out)
	}
	if !strings.Contains(out, "Vocabulary terms: 2") {
		t.Fatalf("expected populated vocabulary count: %s", out)
	}
	if !strings.Contains(out, "postgres") || !strings.Contains(out, "redis") {
		t.Fatalf("expected vocabulary preview terms: %s", out)
	}
}

func TestFormatSummaryTruncatesPreview(t *testing.T) {
	artifacts := []string{"one", "two", "three", "four", "five", "six", "seven"}
	s := BuildSummary(Overrides{SilenceArtifacts: artifacts}, nil, nil, VocabularySources{}, false)

	var b strings.Builder
	if err := FormatSummary(&b, s); err != nil {
		t.Fatalf("FormatSummary: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, "... and 2 more; see: voxi feedback silence-artifact list") {
		t.Fatalf("expected truncated preview hint: %s", out)
	}
	if strings.Contains(out, "six") || strings.Contains(out, "seven") {
		t.Fatalf("expected only first %d artifacts previewed: %s", previewLimit, out)
	}
}

func TestStopWordRowsOrderAndState(t *testing.T) {
	o := Overrides{User: []string{"um"}, Disabled: []string{"hallucination-1"}}
	rows := StopWordRows(o, testBuiltins())
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d: %+v", len(rows), rows)
	}
	if rows[0].State != "disabled" || rows[0].ID != "hallucination-1" {
		t.Fatalf("expected first builtin disabled, got %+v", rows[0])
	}
	if rows[1].State != "built-in" || rows[1].ID != "hallucination-2" {
		t.Fatalf("expected second builtin active, got %+v", rows[1])
	}
	if rows[2].State != "user" || rows[2].Pattern != "um" {
		t.Fatalf("expected user row last, got %+v", rows[2])
	}
}
