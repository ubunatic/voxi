package bench

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ubunatic.com/voxi/internal/audio"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/feedback"
)

// benchReplacementRules mixes the three categories so each mode's selection
// is observable: taya is jargon, heiko is repair, jewel is uncategorized.
func benchReplacementRules() []feedback.Replacement {
	return []feedback.Replacement{
		{From: "taya", To: "taia", Category: feedback.CategoryJargon},
		{From: "heiko", To: "haiku", Category: feedback.CategoryRepair},
		{From: "jewel", To: "jules"},
	}
}

func TestNormalizeReplacementMode(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", ReplacementsNone},
		{ReplacementsNone, ReplacementsNone},
		{ReplacementsJargon, ReplacementsJargon},
		{ReplacementsAll, ReplacementsAll},
	} {
		got, err := NormalizeReplacementMode(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("NormalizeReplacementMode(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	for _, bad := range []string{"JARGON", " jargon", "repair", "typo"} {
		if _, err := NormalizeReplacementMode(bad); err == nil || !strings.Contains(err.Error(), "invalid --replacements") {
			t.Errorf("NormalizeReplacementMode(%q) error = %v, want invalid --replacements", bad, err)
		}
	}
}

func TestApplyReplacementModeNoneLeavesTextUntouched(t *testing.T) {
	const text = "taya heiko jewel"
	if got := applyReplacementMode(text, ReplacementsNone, benchReplacementRules()); got != text {
		t.Fatalf("none changed text: %q", got)
	}
}

func TestApplyReplacementModeJargonSkipsRepairAndUncategorized(t *testing.T) {
	got := applyReplacementMode("taya heiko jewel", ReplacementsJargon, benchReplacementRules())
	if want := "taia heiko jewel"; got != want {
		t.Fatalf("jargon = %q, want %q", got, want)
	}
}

func TestApplyReplacementModeAllAppliesEveryCategory(t *testing.T) {
	got := applyReplacementMode("taya heiko jewel", ReplacementsAll, benchReplacementRules())
	if want := "taia haiku jules"; got != want {
		t.Fatalf("all = %q, want %q", got, want)
	}
}

func TestApplyReplacementsToReportSkipsFailedRows(t *testing.T) {
	report := &Report{Results: []Result{
		{Model: "ok", Text: "taya heiko"},
		{Model: "failed", Text: "taya heiko", Error: "boom"},
	}}
	applyReplacementsToReport(report, ReplacementsAll, benchReplacementRules())
	if report.Results[0].Text != "taia haiku" {
		t.Errorf("successful row text = %q", report.Results[0].Text)
	}
	if report.Results[1].Text != "taya heiko" {
		t.Errorf("failed row text changed: %q", report.Results[1].Text)
	}
}

func TestRunRejectsInvalidReplacementModeBeforeWork(t *testing.T) {
	d := deps.Dependencies{
		Stdout:   &bytes.Buffer{},
		LookPath: func(string) (string, error) { return "", errors.New("not found") },
	}
	_, err := Run(context.Background(), d, Options{Replacements: "typo"})
	if err == nil || !strings.Contains(err.Error(), "invalid --replacements") {
		t.Fatalf("Run(typo) error = %v", err)
	}
}

func TestRunJargonNeedsHomeToReadDictionary(t *testing.T) {
	d := deps.Dependencies{
		Stdout:   &bytes.Buffer{},
		LookPath: func(string) (string, error) { return "", errors.New("not found") },
	}
	_, err := Run(context.Background(), d, Options{Replacements: ReplacementsJargon})
	if err == nil || !strings.Contains(err.Error(), "home directory") {
		t.Fatalf("Run(jargon, no home) error = %v", err)
	}
}

// TestRunReplacementModesEndToEnd drives Run with a fake voxtype whose
// transcript contains one jargon and one repair term, and checks the text in
// the report for each mode. The dictionary lives in a temp Home.
func TestRunReplacementModesEndToEnd(t *testing.T) {
	binDir := t.TempDir()
	voxtypeScript := filepath.Join(binDir, "voxtype")
	script := "#!/bin/sh\necho \"whisper_backend_init_gpu: using Vulkan0 backend\"\necho \"Shipping taya heiko now.\"\nexit 0\n"
	if err := os.WriteFile(voxtypeScript, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	wavPath := filepath.Join(t.TempDir(), "test.wav")
	if err := audio.WriteWAVAudio(wavPath, make([]byte, 16000*2), 16000); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	if err := feedback.SaveReplacements(feedback.ReplacementPath(home), benchReplacementRules()); err != nil {
		t.Fatal(err)
	}
	d := deps.Dependencies{
		Stdout: &bytes.Buffer{},
		LookPath: func(file string) (string, error) {
			if file == "voxtype" {
				return voxtypeScript, nil
			}
			return "", errors.New("not found")
		},
	}
	for _, tc := range []struct {
		mode, home, wantText string
	}{
		{ReplacementsNone, "", "taya heiko"},
		{ReplacementsJargon, home, "taia heiko"},
		{ReplacementsAll, home, "taia haiku"},
	} {
		report, err := Run(context.Background(), d, Options{
			WavFile: wavPath, Models: []string{"base.en"}, Backends: []string{"cpu"}, Threads: 1,
			Replacements: tc.mode, Home: tc.home,
		})
		if err != nil {
			t.Fatalf("Run(%s) error: %v", tc.mode, err)
		}
		if report.Replacements != tc.mode {
			t.Errorf("report.Replacements = %q, want %q", report.Replacements, tc.mode)
		}
		if len(report.Results) != 1 || report.Results[0].Error != "" {
			t.Fatalf("Run(%s) results = %+v", tc.mode, report.Results)
		}
		if got := report.Results[0].Text; !strings.Contains(got, tc.wantText) {
			t.Errorf("Run(%s) text = %q, want it to contain %q", tc.mode, got, tc.wantText)
		}
	}
}
