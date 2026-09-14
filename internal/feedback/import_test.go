package feedback

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/audio"
	"ubunatic.com/voxi/internal/devsample"
)

const testMaxTermChars = 64

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestImportFreshMerge(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()

	writeFile(t, filepath.Join(src, fileName), `{"user":["banana"],"disabled_builtin":["um"],"silence_artifacts":["uh"]}`)
	writeFile(t, filepath.Join(src, replacementFileName), `[{"from":"voxy","to":"voxi"}]`)
	writeFile(t, filepath.Join(src, vocabularyFileName), "kubectl\nterraform\n")

	out := &bytes.Buffer{}
	if err := Import(home, src, ImportOptions{}, testMaxTermChars, out); err != nil {
		t.Fatalf("Import: %v\noutput:\n%s", err, out.String())
	}

	o, err := Load(Path(home))
	if err != nil {
		t.Fatalf("Load stop-words: %v", err)
	}
	if len(o.User) != 1 || o.User[0] != "banana" {
		t.Errorf("stop-words User = %v, want [banana]", o.User)
	}
	if len(o.Disabled) != 1 || o.Disabled[0] != "um" {
		t.Errorf("stop-words Disabled = %v, want [um]", o.Disabled)
	}
	if len(o.SilenceArtifacts) != 1 || o.SilenceArtifacts[0] != "uh" {
		t.Errorf("stop-words SilenceArtifacts = %v, want [uh]", o.SilenceArtifacts)
	}

	rules, err := LoadReplacements(ReplacementPath(home))
	if err != nil {
		t.Fatalf("LoadReplacements: %v", err)
	}
	if len(rules) != 1 || rules[0].From != "voxy" || rules[0].To != "voxi" {
		t.Errorf("replacements = %+v, want [{voxy voxi false}]", rules)
	}

	if !strings.Contains(out.String(), "stop-words: 3 imported, 0 skipped, 0 failed") {
		t.Errorf("output missing stop-words summary:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "replacements: 1 imported, 0 skipped, 0 failed") {
		t.Errorf("output missing replacements summary:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "vocabulary: 2 imported, 0 skipped, 0 failed") {
		t.Errorf("output missing vocabulary summary:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "no samples/ found") {
		t.Errorf("output missing samples skip notice:\n%s", out.String())
	}
}

func TestImportStopWordCollisionSkipped(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()

	if err := Save(Path(home), Overrides{User: []string{"Banana"}}); err != nil {
		t.Fatalf("seed dest: %v", err)
	}
	writeFile(t, filepath.Join(src, fileName), `{"user":["banana","kiwi"]}`)

	out := &bytes.Buffer{}
	if err := Import(home, src, ImportOptions{}, testMaxTermChars, out); err != nil {
		t.Fatalf("Import: %v", err)
	}

	o, err := Load(Path(home))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(o.User) != 2 {
		t.Fatalf("User = %v, want 2 entries (banana kept once, kiwi added)", o.User)
	}
	if !strings.Contains(out.String(), "stop-words: 1 imported, 1 skipped, 0 failed") {
		t.Errorf("output missing collision summary:\n%s", out.String())
	}
}

func TestImportReplacementCollisionSkipByDefault(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()

	if err := SaveReplacements(ReplacementPath(home), []Replacement{{From: "voxy", To: "voxi"}}); err != nil {
		t.Fatalf("seed dest: %v", err)
	}
	writeFile(t, filepath.Join(src, replacementFileName), `[{"from":"Voxy","to":"changed"}]`)

	out := &bytes.Buffer{}
	if err := Import(home, src, ImportOptions{}, testMaxTermChars, out); err != nil {
		t.Fatalf("Import: %v", err)
	}

	rules, err := LoadReplacements(ReplacementPath(home))
	if err != nil {
		t.Fatalf("LoadReplacements: %v", err)
	}
	if len(rules) != 1 || rules[0].To != "voxi" {
		t.Fatalf("rules = %+v, want local entry preserved (To=voxi)", rules)
	}
	if !strings.Contains(out.String(), "replacements: 0 imported, 1 skipped, 0 failed") {
		t.Errorf("output missing skip summary:\n%s", out.String())
	}
}

func TestImportReplacementCollisionOverwrite(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()

	if err := SaveReplacements(ReplacementPath(home), []Replacement{{From: "voxy", To: "voxi"}}); err != nil {
		t.Fatalf("seed dest: %v", err)
	}
	writeFile(t, filepath.Join(src, replacementFileName), `[{"from":"Voxy","to":"changed"}]`)

	out := &bytes.Buffer{}
	if err := Import(home, src, ImportOptions{Overwrite: true}, testMaxTermChars, out); err != nil {
		t.Fatalf("Import: %v", err)
	}

	rules, err := LoadReplacements(ReplacementPath(home))
	if err != nil {
		t.Fatalf("LoadReplacements: %v", err)
	}
	if len(rules) != 1 || rules[0].To != "changed" {
		t.Fatalf("rules = %+v, want overwritten entry (To=changed)", rules)
	}
	if !strings.Contains(out.String(), "replacements: 1 imported, 0 skipped, 0 failed") {
		t.Errorf("output missing overwrite summary:\n%s", out.String())
	}
}

func TestImportMalformedStopWordsRejectedWhole(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()

	writeFile(t, filepath.Join(src, fileName), `{not valid json`)
	writeFile(t, filepath.Join(src, replacementFileName), `[{"from":"voxy","to":"voxi"}]`)

	out := &bytes.Buffer{}
	err := Import(home, src, ImportOptions{}, testMaxTermChars, out)
	if err == nil {
		t.Fatal("Import: want error for malformed stop-words.json, got nil")
	}

	// Nothing should have been written locally for the malformed area.
	if _, statErr := os.Stat(Path(home)); !os.IsNotExist(statErr) {
		t.Errorf("stop-words.json should not have been created locally, stat err = %v", statErr)
	}

	// The other area must still have imported despite the malformed file.
	rules, loadErr := LoadReplacements(ReplacementPath(home))
	if loadErr != nil {
		t.Fatalf("LoadReplacements: %v", loadErr)
	}
	if len(rules) != 1 {
		t.Fatalf("replacements = %+v, want 1 entry imported despite stop-words failure", rules)
	}
	if !strings.Contains(out.String(), "stop-words: import failed") {
		t.Errorf("output missing stop-words failure notice:\n%s", out.String())
	}
}

func TestImportVocabularyInvalidLineSkippedNotAborted(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()

	writeFile(t, filepath.Join(src, vocabularyFileName), "kubectl\n!!!\nterraform\n")

	out := &bytes.Buffer{}
	if err := Import(home, src, ImportOptions{}, testMaxTermChars, out); err != nil {
		t.Fatalf("Import: %v\noutput:\n%s", err, out.String())
	}

	terms, err := loadVocabularyForTest(home)
	if err != nil {
		t.Fatalf("load vocabulary: %v", err)
	}
	if len(terms) != 2 {
		t.Fatalf("terms = %v, want 2 (kubectl, terraform)", terms)
	}
	if !strings.Contains(out.String(), "vocabulary: 2 imported, 0 skipped, 1 failed") {
		t.Errorf("output missing vocabulary failure count:\n%s", out.String())
	}
}

func TestImportOnlyFiltersAreas(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()

	writeFile(t, filepath.Join(src, fileName), `{"user":["banana"]}`)
	writeFile(t, filepath.Join(src, replacementFileName), `[{"from":"voxy","to":"voxi"}]`)

	out := &bytes.Buffer{}
	if err := Import(home, src, ImportOptions{Only: []string{AreaReplacements}}, testMaxTermChars, out); err != nil {
		t.Fatalf("Import: %v", err)
	}

	if _, statErr := os.Stat(Path(home)); !os.IsNotExist(statErr) {
		t.Errorf("stop-words should not have been imported when --only=replacements, stat err = %v", statErr)
	}
	rules, err := LoadReplacements(ReplacementPath(home))
	if err != nil {
		t.Fatalf("LoadReplacements: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("replacements = %+v, want 1 entry", rules)
	}
	if strings.Contains(out.String(), "stop-words:") {
		t.Errorf("output should not mention stop-words when filtered out:\n%s", out.String())
	}
}

func TestImportUnknownOnlyAreaRejected(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()
	out := &bytes.Buffer{}
	err := Import(home, src, ImportOptions{Only: []string{"bogus"}}, testMaxTermChars, out)
	if err == nil {
		t.Fatal("Import: want error for unknown --only area, got nil")
	}
}

func TestImportSamplesDelegatesToDevsample(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()

	ts := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	sourceSamplesDir := filepath.Join(src, "samples")
	if err := os.MkdirAll(sourceSamplesDir, 0o700); err != nil {
		t.Fatalf("mkdir source samples dir: %v", err)
	}
	if err := audio.WriteWAVAudio(devsample.WAVPathIn(sourceSamplesDir, "greeting"), bytes.Repeat([]byte{0, 1}, 8000), 16000); err != nil {
		t.Fatalf("write fixture wav: %v", err)
	}
	entry := devsample.Sample{Name: "greeting", WAVFile: "greeting.wav", Text: "hello there", Timestamp: ts}
	if err := devsample.SaveManifestIn(sourceSamplesDir, []devsample.Sample{entry}); err != nil {
		t.Fatalf("save source manifest: %v", err)
	}

	out := &bytes.Buffer{}
	if err := Import(home, src, ImportOptions{}, testMaxTermChars, out); err != nil {
		t.Fatalf("Import: %v\noutput:\n%s", err, out.String())
	}

	samples, err := devsample.LoadManifest(home)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	got, ok := devsample.Find(samples, "greeting")
	if !ok {
		t.Fatal("sample not imported into destination manifest")
	}
	if got.Text != "hello there" {
		t.Errorf("Text = %q, want %q", got.Text, "hello there")
	}
	// devsample.Import prints its own per-entry + summary lines; Import must
	// not silently swallow them.
	if !strings.Contains(out.String(), "imported \"greeting\"") {
		t.Errorf("output missing devsample's own import line:\n%s", out.String())
	}
}

// loadVocabularyForTest reads back the vocabulary file the same way
// speechcontext.LoadVocabulary would, without importing speechcontext
// directly into every assertion above.
func loadVocabularyForTest(home string) ([]string, error) {
	path := filepath.Join(home, ".config", "voxi", "vocabulary.txt")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var terms []string
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if line != "" {
			terms = append(terms, line)
		}
	}
	return terms, nil
}
