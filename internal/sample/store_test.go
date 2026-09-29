package sample

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreRoundTripAndPrivatePermissions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	x := Sample{ID: "hello", Purpose: Dictation, Transcript: "hello there", Keyterms: []string{"hello"}, Created: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Source: "record"}
	if err = s.Add(x); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(x.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Transcript != x.Transcript || got.Source != x.Source || got.Purpose != Dictation {
		t.Fatalf("round trip: %+v", got)
	}
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("root mode %o", info.Mode().Perm())
	}
	info, err = os.Stat(filepath.Join(root, "dictation", "hello.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("sidecar mode %o", info.Mode().Perm())
	}
}

func TestDuplicateAcrossPurposesAndMoveDelete(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Add(Sample{ID: "same", Purpose: Noise}); err != nil {
		t.Fatal(err)
	}
	if err = s.Add(Sample{ID: "same", Purpose: Dictation}); err == nil {
		t.Fatal("duplicate id accepted")
	}
	if err = s.Move("same", Dictation); err != nil {
		t.Fatal(err)
	}
	if err = s.Delete("same"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get("same"); err == nil {
		t.Fatal("deleted sample remains")
	}
}

func TestCorruptSidecarDoesNotHideOtherSamples(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Add(Sample{ID: "valid", Purpose: Noise}); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(s.root, "noise", "broken.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	items, err := s.List(Noise)
	if err == nil || !strings.Contains(err.Error(), "broken.json") {
		t.Fatalf("List error = %v, want corrupt path", err)
	}
	if len(items) != 1 || items[0].ID != "valid" {
		t.Fatalf("valid items lost: %+v", items)
	}
}

func TestPublicPermissionsAndTSV(t *testing.T) {
	s, err := OpenPublic(filepath.Join(t.TempDir(), "public"))
	if err != nil {
		t.Fatal(err)
	}
	items := []Sample{{ID: "b", Purpose: Noise, Audio: "b.flac", Transcript: "a\tnoise", Keyterms: []string{"key", "term"}}, {ID: "a", Purpose: Dictation, Audio: "a.wav", Transcript: "speech"}}
	if err = s.Add(items[0]); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(s.root, "noise", "b.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("public sidecar mode %o", info.Mode().Perm())
	}
	want := "# id\twav file\texpected transcript\tkeyterms separated by |\na\ta.wav\tspeech\t\nb\tb.flac\ta noise\tkey|term\n"
	if got := string(ExportTSV(items)); got != want {
		t.Errorf("ExportTSV:\n%s\nwant:\n%s", got, want)
	}
}

func TestLoadLegacyTSV(t *testing.T) {
	dir := t.TempDir()
	data := "# header\none\tone.wav\tHi there\tHi|there\n"
	if err := os.WriteFile(filepath.Join(dir, "corpus.tsv"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadLegacyTSV(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "one" || got[0].Audio != "one.wav" || len(got[0].Keyterms) != 2 {
		t.Fatalf("legacy parse: %+v", got)
	}
}
