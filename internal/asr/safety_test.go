package asr

import (
	"strings"
	"testing"
	"ubunatic.com/voxi/spec"
)

func safetyLimits() spec.TranscriptSafetySpec {
	return spec.TranscriptSafetySpec{MaxChars: 2000, MaxTokenChars: 256, MaxRepeatUnitChars: 16, MinRepeatCount: 12, MinRepeatedChars: 96}
}

func TestTranscriptSafetyRejectsCapturedShape(t *testing.T) {
	r := CheckTranscriptSafety("Ubun"+strings.Repeat("tuk", 150), safetyLimits())
	if r.Reason != "pathological_repetition" || r.RepeatUnit != "tuk" || r.RepeatCount != 150 {
		t.Fatalf("unexpected result: %+v", r)
	}
}

func TestTranscriptSafetyFalsePositives(t *testing.T) {
	valid := []string{
		"pneumonoultramicroscopicsilicovolcanoconiosis",
		"https://example.com/a/really-long/path?token=abcdef0123456789",
		strings.Repeat("a", 64), "fmt.Println(\"hello\") && value != nil",
		"very very very very very very very very very very very very good",
		strings.Repeat("!", 40), "你好，世界 — Привет мир",
	}
	for _, text := range valid {
		if got := CheckTranscriptSafety(text, safetyLimits()); got.Reason != "" {
			t.Errorf("rejected %q: %+v", text, got)
		}
	}
}

func TestTranscriptSafetyBounds(t *testing.T) {
	if got := CheckTranscriptSafety(strings.Repeat("word ", 500), safetyLimits()); got.Reason != "output_too_long" {
		t.Fatalf("got %+v", got)
	}
	if got := CheckTranscriptSafety(strings.Repeat("x", 257), safetyLimits()); got.Reason != "pathological_repetition" {
		t.Fatalf("got %+v", got)
	}
}
