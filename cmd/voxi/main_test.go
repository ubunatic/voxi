package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/history"
)

func TestHTMLManualUsesAttachedWebsitePaths(t *testing.T) {
	root := &cobra.Command{Use: "voxi"}
	cmd := newManCmd(root)
	cmd.SetArgs([]string{"--html"})
	var output bytes.Buffer
	cmd.SetOut(&output)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("render HTML manual: %v", err)
	}
	page := output.String()
	for _, want := range []string{
		`href="../assets/docs.css"`,
		`src="../assets/logo.svg"`,
		`href="../#top"`,
		`href="../#manual"`,
		`&copy; 2026 Uwe Jugel`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("manual lacks %s", want)
		}
	}
	for _, old := range []string{"../index.css", "../index.js", "../logo.svg", "../index.html"} {
		if strings.Contains(page, old) {
			t.Errorf("manual retains old website path %s", old)
		}
	}
}

func TestRecordHistoryEntryDisabledWritesNoEntry(t *testing.T) {
	path := history.HistoryPath(t.TempDir())
	if err := recordHistoryEntry(path, "private transcript", false); err != nil {
		t.Fatalf("recordHistoryEntry: %v", err)
	}
	entries, err := history.ListHistory(path)
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("history entries = %d, want 0", len(entries))
	}
}
