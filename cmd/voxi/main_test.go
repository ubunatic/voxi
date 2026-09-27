package main

import (
	"testing"

	"ubunatic.com/voxi/internal/history"
)

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
