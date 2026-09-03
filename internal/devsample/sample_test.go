package devsample

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSanitizeName(t *testing.T) {
	cases := []struct {
		raw     string
		want    string
		wantErr bool
	}{
		{raw: "hello", want: "hello"},
		{raw: "  hello world  ", want: "hello-world"},
		{raw: "", wantErr: true},
		{raw: "   ", wantErr: true},
		{raw: "../etc/passwd", wantErr: true},
		{raw: "a/b", wantErr: true},
		{raw: `a\b`, wantErr: true},
		{raw: "..", wantErr: true},
		{raw: "name\x00withcontrol", want: "name-withcontrol"},
		{raw: strings.Repeat("a", 200), wantErr: true},
	}
	for _, c := range cases {
		got, err := SanitizeName(c.raw)
		if c.wantErr {
			if err == nil {
				t.Errorf("SanitizeName(%q) = %q, want error", c.raw, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("SanitizeName(%q) unexpected error: %v", c.raw, err)
			continue
		}
		if got != c.want {
			t.Errorf("SanitizeName(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}

func TestManifestRoundTrip(t *testing.T) {
	ts := time.Date(2026, 9, 1, 12, 30, 0, 0, time.UTC)
	samples := []Sample{
		{Name: "jfk", WAVFile: "jfk.wav", Text: "Ask not what your country can do", Timestamp: ts},
		{Name: "greeting", WAVFile: "greeting.wav", Text: "Hello there, general text with\ttabs\nand newlines", Timestamp: ts.Add(time.Hour)},
	}
	data := FormatManifest(samples)
	got, err := ParseManifest(data)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d samples, want 2", len(got))
	}
	byName := map[string]Sample{}
	for _, s := range got {
		byName[s.Name] = s
	}
	jfk, ok := byName["jfk"]
	if !ok {
		t.Fatal("missing jfk sample")
	}
	if jfk.Text != "Ask not what your country can do" {
		t.Errorf("jfk text = %q", jfk.Text)
	}
	if !jfk.Timestamp.Equal(ts) {
		t.Errorf("jfk timestamp = %v, want %v", jfk.Timestamp, ts)
	}
	greeting := byName["greeting"]
	if strings.ContainsAny(greeting.Text, "\t\n") {
		t.Errorf("greeting text retains raw whitespace: %q", greeting.Text)
	}

	// Comment lines (including #ts) must stay invisible to a plain
	// corpus.tsv-style reader that just skips '#'-prefixed lines, keeping
	// this manifest usable unmodified by scripts/speech_context_bench.
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 4 {
			t.Errorf("data entry line not corpus.tsv 4-field shape: %q", line)
		}
	}
}

func TestParseManifestMissingIsEmpty(t *testing.T) {
	home := t.TempDir()
	samples, err := LoadManifest(home)
	if err != nil {
		t.Fatalf("LoadManifest on missing file: %v", err)
	}
	if samples != nil {
		t.Fatalf("expected nil samples, got %v", samples)
	}
}

func TestSaveLoadManifestAtomicAndPrivate(t *testing.T) {
	home := t.TempDir()
	samples := []Sample{{Name: "a", WAVFile: "a.wav", Text: "hello", Timestamp: time.Now()}}
	if err := SaveManifest(home, samples); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}

	dirInfo, err := os.Stat(SamplesDir(home))
	if err != nil {
		t.Fatalf("stat samples dir: %v", err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Errorf("samples dir perm = %v, want 0700", dirInfo.Mode().Perm())
	}

	fileInfo, err := os.Stat(ManifestPath(home))
	if err != nil {
		t.Fatalf("stat manifest: %v", err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Errorf("manifest perm = %v, want 0600", fileInfo.Mode().Perm())
	}

	got, err := LoadManifest(home)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if len(got) != 1 || got[0].Name != "a" {
		t.Fatalf("LoadManifest roundtrip = %+v", got)
	}

	// No stray temp files left behind.
	entries, err := os.ReadDir(SamplesDir(home))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".corpus-") {
			t.Errorf("leftover temp manifest file: %s", e.Name())
		}
	}
}

func TestFindUpsertRemoveEntry(t *testing.T) {
	samples := []Sample{{Name: "a", Text: "first"}}
	if _, ok := Find(samples, "missing"); ok {
		t.Fatal("Find found a nonexistent sample")
	}
	s, ok := Find(samples, "a")
	if !ok || s.Text != "first" {
		t.Fatalf("Find(a) = %+v, %v", s, ok)
	}

	samples = Upsert(samples, Sample{Name: "a", Text: "updated"})
	if len(samples) != 1 {
		t.Fatalf("Upsert should replace, got %d entries", len(samples))
	}
	s, _ = Find(samples, "a")
	if s.Text != "updated" {
		t.Errorf("Upsert did not replace text: %q", s.Text)
	}

	samples = Upsert(samples, Sample{Name: "b", Text: "second"})
	if len(samples) != 2 {
		t.Fatalf("Upsert should add new entries, got %d", len(samples))
	}

	samples, found := RemoveEntry(samples, "a")
	if !found {
		t.Fatal("RemoveEntry did not find a")
	}
	if len(samples) != 1 || samples[0].Name != "b" {
		t.Fatalf("RemoveEntry left %+v", samples)
	}
	if _, found := RemoveEntry(samples, "missing"); found {
		t.Fatal("RemoveEntry reported found for missing name")
	}
}

func TestPathHelpers(t *testing.T) {
	home := "/home/example"
	if got := SamplesDir(home); got != filepath.Join(home, ".config", "voxi", "samples") {
		t.Errorf("SamplesDir = %q", got)
	}
	if got := WAVPath(home, "sample1"); got != filepath.Join(SamplesDir(home), "sample1.wav") {
		t.Errorf("WAVPath = %q", got)
	}
}

func TestPreview(t *testing.T) {
	s := Sample{Text: "hello"}
	if got := s.Preview(10); got != "hello" {
		t.Errorf("Preview short text = %q", got)
	}
	s = Sample{Text: strings.Repeat("x", 20)}
	got := s.Preview(5)
	if got != "xxxxx..." {
		t.Errorf("Preview truncated text = %q", got)
	}
}
