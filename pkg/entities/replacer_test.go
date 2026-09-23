package entities

import (
	"testing"
)

func TestReplaceEntitiesBasic(t *testing.T) {
	result := &NeedleResult{
		Entities: []EntityResolution{
			{RawToken: "andja", IsPerson: true, MatchedName: "Anja", Confidence: 0.95},
			{RawToken: "uwe", IsPerson: true, MatchedName: "Uwe", Confidence: 0.88},
		},
	}

	tests := []struct {
		input     string
		threshold float64
		expected  string
	}{
		{
			input:     "hallo andja",
			threshold: 0.65,
			expected:  "hallo Anja",
		},
		{
			input:     "ich spreche mit uwe.",
			threshold: 0.65,
			expected:  "ich spreche mit Uwe.",
		},
		{
			input:     "oowe und andja gehen spazieren!",
			threshold: 0.65,
			expected:  "oowe und Anja gehen spazieren!",
		},
	}

	for _, tc := range tests {
		actual := ReplaceEntities(tc.input, result, tc.threshold)
		if actual != tc.expected {
			t.Errorf("input %q: expected %q, got %q", tc.input, tc.expected, actual)
		}
	}
}

func TestReplaceEntitiesPunctuationAndWhitespace(t *testing.T) {
	result := &NeedleResult{
		Entities: []EntityResolution{
			{RawToken: "oowe", IsPerson: true, MatchedName: "Uwe", Confidence: 0.90},
			{RawToken: "ania", IsPerson: true, MatchedName: "Anja", Confidence: 0.90},
			{RawToken: "gunter", IsPerson: true, MatchedName: "Guenther", Confidence: 0.90},
		},
	}

	input := "Hallo, oowe! Sind ania und gunter da?"
	expected := "Hallo, Uwe! Sind Anja und Guenther da?"

	actual := ReplaceEntities(input, result, 0.65)
	if actual != expected {
		t.Errorf("expected %q, got %q", expected, actual)
	}
}

func TestReplaceEntitiesConfidenceThreshold(t *testing.T) {
	result := &NeedleResult{
		Entities: []EntityResolution{
			{RawToken: "andja", IsPerson: true, MatchedName: "Anja", Confidence: 0.50}, // below 0.65
		},
	}

	input := "hallo andja"
	actual := ReplaceEntities(input, result, 0.65)
	if actual != input {
		t.Errorf("expected unmutated %q, got %q", input, actual)
	}
}

func TestReplaceEntitiesWordBoundaries(t *testing.T) {
	result := &NeedleResult{
		Entities: []EntityResolution{
			{RawToken: "andja", IsPerson: true, MatchedName: "Anja", Confidence: 0.95},
		},
	}

	// Token "andja" inside larger word "handjabd" must NOT be replaced
	input := "handjabd"
	actual := ReplaceEntities(input, result, 0.65)
	if actual != input {
		t.Errorf("expected unmutated %q, got %q", input, actual)
	}
}

func TestReplaceEntitiesUmlauts(t *testing.T) {
	result := &NeedleResult{
		Entities: []EntityResolution{
			{RawToken: "günther", IsPerson: true, MatchedName: "Guenther", Confidence: 0.92},
		},
	}

	input := "Hallo günther, wie geht es dir?"
	expected := "Hallo Guenther, wie geht es dir?"

	actual := ReplaceEntities(input, result, 0.65)
	if actual != expected {
		t.Errorf("expected %q, got %q", expected, actual)
	}
}

func TestReplaceEntitiesNonPersonIgnored(t *testing.T) {
	result := &NeedleResult{
		Entities: []EntityResolution{
			{RawToken: "andja", IsPerson: false, MatchedName: "Anja", Confidence: 0.95},
		},
	}

	input := "hallo andja"
	actual := ReplaceEntities(input, result, 0.65)
	if actual != input {
		t.Errorf("expected unmutated %q, got %q", input, actual)
	}
}
