package main

import (
	"math"
	"testing"
)

func TestWordErrorRate(t *testing.T) {
	if got := wordErrorRate("Voxi uses dotool", "Voxi uses dotool"); got != 0 {
		t.Fatalf("exact WER = %v, want 0", got)
	}
	if got := wordErrorRate("Voxi uses dotool", "Moxie uses a tool"); math.Abs(got-1) > 0.0001 {
		t.Fatalf("replacement/insertion WER = %v, want 1", got)
	}
}

func TestContainsTermUsesWholeNormalizedTerms(t *testing.T) {
	if !containsTerm("Use PipeWire and models.yaml.", "PipeWire") || !containsTerm("Use PipeWire and models.yaml.", "models.yaml") {
		t.Fatal("expected exact technical terms to be found")
	}
	if containsTerm("goodbye", "bye") {
		t.Fatal("substring counted as exact keyterm")
	}
}

func TestAdjacentRepeats(t *testing.T) {
	if got := adjacentRepeats("use use Voxi now now now"); got != 3 {
		t.Fatalf("adjacentRepeats() = %d, want 3", got)
	}
}

func TestSummarize(t *testing.T) {
	got := summarize([]result{
		{Prompted: true, WER: 0.5, KeytermsFound: 1, KeytermsTotal: 2, LatencyMS: 30, AdjacentRepeats: 1},
		{Prompted: false, WER: 1, LatencyMS: 5},
		{Prompted: true, WER: 0, KeytermsFound: 2, KeytermsTotal: 2, LatencyMS: 10},
	}, true)
	if got.MeanWER != 0.25 || got.KeytermRecall != 0.75 || got.MedianLatencyMS != 20 || got.AdjacentRepeats != 1 {
		t.Fatalf("summarize() = %+v", got)
	}
}
