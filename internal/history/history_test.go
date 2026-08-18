package history

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAppendAndListHistory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	t1 := time.Date(2026, 8, 18, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 8, 18, 10, 5, 0, 0, time.UTC)
	t3 := time.Date(2026, 8, 18, 10, 10, 0, 0, time.UTC)

	// 1. Append entries with limit=2
	e1, err := AppendHistory(path, "First phrase", 2, t1)
	if err != nil {
		t.Fatal(err)
	}
	if e1.Text != "First phrase" {
		t.Fatalf("unexpected entry text: %s", e1.Text)
	}

	e2, err := AppendHistory(path, "Second phrase", 2, t2)
	if err != nil {
		t.Fatal(err)
	}

	entries, err := ListHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].ID != e2.ID || entries[1].ID != e1.ID {
		t.Fatalf("expected [e2, e1], got: %+v", entries)
	}

	// 2. Append third phrase (should trim first phrase)
	e3, err := AppendHistory(path, "Third phrase", 2, t3)
	if err != nil {
		t.Fatal(err)
	}

	entries, err = ListHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].ID != e3.ID || entries[1].ID != e2.ID {
		t.Fatalf("expected [e3, e2], got: %+v", entries)
	}

	// 3. Find history entry
	found, err := FindHistoryEntry(path, e3.ID)
	if err != nil || found.Text != "Third phrase" {
		t.Fatalf("find entry error: %v, found: %+v", err, found)
	}

	// 4. Clear history
	if err := ClearHistory(path); err != nil {
		t.Fatal(err)
	}
	entries, _ = ListHistory(path)
	if len(entries) != 0 {
		t.Fatalf("expected empty after clear, got %d", len(entries))
	}
}
