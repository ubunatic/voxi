package speechcontext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeTermMatchesPromptSanitizer(t *testing.T) {
	term, err := NormalizeTerm(" /private/path/Pipe|Wire ", 64)
	if err != nil {
		t.Fatal(err)
	}
	if term != "Pipe Wire" {
		t.Fatalf("NormalizeTerm() = %q, want %q", term, "Pipe Wire")
	}
	for _, raw := range []string{"", "!!!", strings.Repeat("x", 65)} {
		if _, err := NormalizeTerm(raw, 64); err == nil {
			t.Errorf("NormalizeTerm(%q) accepted invalid term", raw)
		}
	}
}

func TestVocabularyAddRemoveAndDeterministicOrder(t *testing.T) {
	terms, added, err := AddVocabulary([]string{"Voxi"}, " pipe|wire ", 64)
	if err != nil {
		t.Fatal(err)
	}
	if added != "pipe wire" || strings.Join(terms, "|") != "pipe wire|Voxi" {
		t.Fatalf("AddVocabulary() = %#v, %q", terms, added)
	}
	if _, _, err := AddVocabulary(terms, "VOXI", 64); err == nil {
		t.Fatal("case-insensitive duplicate add succeeded")
	}
	terms, removed, err := RemoveVocabulary(terms, "PIPE WIRE", 64)
	if err != nil {
		t.Fatal(err)
	}
	if removed != "pipe wire" || len(terms) != 1 || terms[0] != "Voxi" {
		t.Fatalf("RemoveVocabulary() = %#v, %q", terms, removed)
	}
	if _, _, err := RemoveVocabulary(terms, "missing", 64); err == nil {
		t.Fatal("missing remove succeeded")
	}
}

func TestVocabularyPersistenceIsAtomicPrivateAndNormalized(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config", "voxi", "vocabulary.txt")
	if err := SaveVocabulary(path, []string{"Voxi", "dotool", "DOTool"}, 64); err != nil {
		t.Fatal(err)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("directory permissions = %o, want 700", got)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("file permissions = %o, want 600", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "dotool\nVoxi\n" {
		t.Fatalf("file contents = %q", got)
	}
	terms, err := LoadVocabulary(path, 64)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(terms, "|") != "dotool|Voxi" {
		t.Fatalf("LoadVocabulary() = %#v", terms)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".vocabulary-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files remain: %#v", matches)
	}
}

func TestLoadVocabularyRejectsInvalidStoredTerm(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vocabulary.txt")
	if err := os.WriteFile(path, []byte("!!!\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadVocabulary(path, 64); err == nil {
		t.Fatal("invalid stored vocabulary was accepted")
	}
}
