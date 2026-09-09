package feedback

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/chunks"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/spec"
)

var rules = []spec.StopWord{{ID: "bye", Pattern: "bye"}, {ID: "thanks", Pattern: "thank you"}}

func TestSaveLoadAtomicPermissionsAndNormalize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "stop-words.json")
	o, err := Add(Overrides{}, " Bye ")
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(path, o); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("permissions = %o, want 0600", got)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.User) != 1 || got.User[0] != "Bye" {
		t.Fatalf("loaded %#v", got)
	}
}

func TestAddRemoveAndBuiltinState(t *testing.T) {
	o, err := Add(Overrides{}, "bye")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Add(o, "BYE"); err == nil {
		t.Fatal("duplicate add succeeded")
	}
	o, err = SetBuiltin(o, "bye", false, rules)
	if err != nil {
		t.Fatal(err)
	}
	if got := ActivePatterns(rules, o); len(got) != 2 || got[0] != "thank you" {
		t.Fatalf("active patterns %#v", got)
	}
	o, err = SetBuiltin(o, "bye", true, rules)
	if err != nil {
		t.Fatal(err)
	}
	o, err = Remove(o, "BYE")
	if err != nil {
		t.Fatal(err)
	}
	if len(o.User) != 0 || len(o.Disabled) != 0 {
		t.Fatalf("unexpected overrides %#v", o)
	}
}

func TestRejectInvalidAndMalformed(t *testing.T) {
	if _, err := Add(Overrides{}, "\x00"); err == nil {
		t.Fatal("control character accepted")
	}
	if _, err := Add(Overrides{}, strings.Repeat("x", 201)); err == nil {
		t.Fatal("long phrase accepted")
	}
	path := filepath.Join(t.TempDir(), "stop-words.json")
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("malformed file accepted")
	}
}

func TestLiteralPatternsAndCommand(t *testing.T) {
	p := literalPattern(`a.+(x)`)
	if !strings.Contains(p, `a\.\+\(x\)`) {
		t.Fatalf("pattern %q did not quote literal", p)
	}
	var out bytes.Buffer
	cmd := NewCommand(&out, t.TempDir(), rules, 64, nil, deps.Dependencies{}, nil)
	cmd.SetArgs([]string{"stop-word", "add", "bye"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "remove") {
		t.Fatalf("add output %q lacks reversal", out.String())
	}
	out.Reset()
	cmd.SetArgs([]string{"stop-word", "disable", "bye"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	cmd.SetArgs([]string{"stop-word", "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "disabled\tbye") || !strings.Contains(out.String(), "user\t-\tbye") {
		t.Fatalf("list output %q", out.String())
	}
}

func TestSilenceArtifactCommandAndPersistence(t *testing.T) {
	var out bytes.Buffer
	cmd := NewCommand(&out, t.TempDir(), rules, 64, nil, deps.Dependencies{}, nil)
	cmd.SetArgs([]string{"silence-artifact", "add", " bye! "})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "whole utterance") || !strings.Contains(out.String(), "remove") {
		t.Fatalf("add output %q", out.String())
	}
	out.Reset()
	cmd.SetArgs([]string{"silence-artifact", "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "silence-artifact\tbye\n" {
		t.Fatalf("list = %q", got)
	}
	out.Reset()
	cmd.SetArgs([]string{"silence-artifact", "remove", "Bye"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Add it again") {
		t.Fatalf("remove output %q", out.String())
	}
}

func TestIsSilenceArtifactMatchesOnlyWholeNormalizedUtterance(t *testing.T) {
	o, err := AddSilenceArtifact(Overrides{}, "bye")
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"bye", "Bye!", "  bye  "} {
		if !IsSilenceArtifact(text, o.SilenceArtifacts) {
			t.Errorf("%q was not rejected", text)
		}
	}
	for _, text := range []string{"goodbye", "hello bye", "bye for now", "say goodbye"} {
		if IsSilenceArtifact(text, o.SilenceArtifacts) {
			t.Errorf("%q was incorrectly rejected", text)
		}
	}
}

func TestIsSilenceArtifactMatchesEitherSideOfPunctuation(t *testing.T) {
	o, err := AddSilenceArtifact(Overrides{}, ".com")
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{".com", "com.", "Com!", "  com  "} {
		if !IsSilenceArtifact(text, o.SilenceArtifacts) {
			t.Errorf("%q was not rejected", text)
		}
	}
	if IsSilenceArtifact("yamal.com", o.SilenceArtifacts) {
		t.Error("yamal.com was incorrectly rejected")
	}
}

func TestVocabularyCommandAddListRemove(t *testing.T) {
	home := t.TempDir()
	var out bytes.Buffer

	run := func(args ...string) error {
		cmd := NewCommand(&out, home, rules, 64, nil, deps.Dependencies{}, nil)
		cmd.SetArgs(args)
		return cmd.Execute()
	}
	if err := run("vocabulary", "add", " Pipe|Wire "); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, `Added vocabulary term "Pipe Wire"`) || !strings.Contains(got, "--speech-context") {
		t.Fatalf("add output = %q", got)
	}
	if err := run("vocabulary", "add", "TLDR"); err != nil {
		t.Fatal(err)
	}
	if err := run("vocabulary", "add", "tldr"); err == nil {
		t.Fatal("case-insensitive duplicate add succeeded")
	}
	out.Reset()
	if err := run("vocabulary", "list"); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "Pipe Wire\nTLDR\n" {
		t.Fatalf("list output = %q", got)
	}
	out.Reset()
	if err := run("vocabulary", "remove", "PIPE WIRE"); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, `Removed vocabulary term "Pipe Wire"`) || !strings.Contains(got, "add") {
		t.Fatalf("remove output = %q", got)
	}
	data, err := os.ReadFile(filepath.Join(home, ".config", "voxi", "vocabulary.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "TLDR\n" {
		t.Fatalf("persisted vocabulary = %q", got)
	}
}

func TestSampleSaveLast(t *testing.T) {
	home := t.TempDir()
	xdgRuntime := t.TempDir()

	// Prepare a chunk in the chunk ring buffer
	chunkBuf := chunks.NewBuffer(chunks.StorageDir(xdgRuntime, home), 5)
	dummyPCM := make([]byte, 3200)
	c := chunks.Chunk{
		Index:                 1,
		Timestamp:             time.Now(),
		AudioDurationSecs:     1.0,
		TranscribeDurationSec: 0.2,
		RTF:                   0.2,
		RawTranscript:         "testing save last",
		CleanedTranscript:     "testing save last",
		Accepted:              true,
	}
	if _, err := chunkBuf.Add(c, dummyPCM, 16000); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	// Stdin provides blank newline for transcript confirmation and blank newline for keyterms
	stdin := strings.NewReader("\n\n")
	d := deps.Dependencies{
		Stdout: &out,
		Stdin:  stdin,
		Getenv: func(key string) string {
			if key == "HOME" {
				return home
			}
			if key == "XDG_RUNTIME_DIR" {
				return xdgRuntime
			}
			return ""
		},
	}

	cmd := NewCommand(&out, home, rules, 64, nil, d, nil)
	cmd.SetArgs([]string{"sample", "save-last", "test-sample"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sample save-last failed: %v", err)
	}

	// Verify sample was saved in ~/.config/voxi/samples
	sampleWAV := filepath.Join(home, ".config", "voxi", "samples", "test-sample.wav")
	if _, err := os.Stat(sampleWAV); err != nil {
		t.Fatalf("expected sample WAV to exist: %v", err)
	}

	manifestBytes, err := os.ReadFile(filepath.Join(home, ".config", "voxi", "samples", "corpus.tsv"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !strings.Contains(string(manifestBytes), "test-sample\ttest-sample.wav\ttesting save last") {
		t.Fatalf("unexpected corpus manifest: %s", string(manifestBytes))
	}
}

func TestSampleSaveChunk(t *testing.T) {
	home := t.TempDir()
	xdgRuntime := t.TempDir()

	chunkBuf := chunks.NewBuffer(chunks.StorageDir(xdgRuntime, home), 5)
	dummyPCM := make([]byte, 3200)

	// Add chunk 1
	c1 := chunks.Chunk{
		Index:                 1,
		Timestamp:             time.Now(),
		AudioDurationSecs:     1.0,
		TranscribeDurationSec: 0.2,
		RTF:                   0.2,
		RawTranscript:         "raw chunk one",
		CleanedTranscript:     "cleaned chunk one",
		Accepted:              true,
	}
	if _, err := chunkBuf.Add(c1, dummyPCM, 16000); err != nil {
		t.Fatal(err)
	}

	// Add chunk 2 (with empty cleaned transcript to test fallback to raw)
	c2 := chunks.Chunk{
		Index:                 2,
		Timestamp:             time.Now(),
		AudioDurationSecs:     1.0,
		TranscribeDurationSec: 0.2,
		RTF:                   0.2,
		RawTranscript:         "raw chunk two only",
		CleanedTranscript:     "",
		Accepted:              true,
	}
	if _, err := chunkBuf.Add(c2, dummyPCM, 16000); err != nil {
		t.Fatal(err)
	}

	depsFor := func(stdinStr string, out *bytes.Buffer) deps.Dependencies {
		return deps.Dependencies{
			Stdout: out,
			Stdin:  strings.NewReader(stdinStr),
			Getenv: func(key string) string {
				if key == "HOME" {
					return home
				}
				if key == "XDG_RUNTIME_DIR" {
					return xdgRuntime
				}
				return ""
			},
		}
	}

	// 1. Test save-chunk with 2 args: specific index (1) and name
	{
		var out bytes.Buffer
		d := depsFor("\n\n", &out)
		cmd := NewCommand(&out, home, rules, 64, nil, d, nil)
		cmd.SetArgs([]string{"sample", "save-chunk", "1", "sample-chunk-1"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("save-chunk 1 sample-chunk-1 failed: %v", err)
		}

		sampleWAV := filepath.Join(home, ".config", "voxi", "samples", "sample-chunk-1.wav")
		if _, err := os.Stat(sampleWAV); err != nil {
			t.Fatalf("expected sample WAV to exist: %v", err)
		}

		manifestBytes, err := os.ReadFile(filepath.Join(home, ".config", "voxi", "samples", "corpus.tsv"))
		if err != nil {
			t.Fatalf("read manifest: %v", err)
		}
		if !strings.Contains(string(manifestBytes), "sample-chunk-1\tsample-chunk-1.wav\tcleaned chunk one") {
			t.Fatalf("unexpected corpus manifest: %s", string(manifestBytes))
		}
	}

	// 2. Test save-chunk with 1 arg: defaults to "last" (chunk 2, raw fallback)
	{
		var out bytes.Buffer
		d := depsFor("\n\n", &out)
		cmd := NewCommand(&out, home, rules, 64, nil, d, nil)
		cmd.SetArgs([]string{"sample", "save-chunk", "sample-chunk-last"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("save-chunk sample-chunk-last failed: %v", err)
		}

		sampleWAV := filepath.Join(home, ".config", "voxi", "samples", "sample-chunk-last.wav")
		if _, err := os.Stat(sampleWAV); err != nil {
			t.Fatalf("expected sample WAV to exist: %v", err)
		}

		manifestBytes, err := os.ReadFile(filepath.Join(home, ".config", "voxi", "samples", "corpus.tsv"))
		if err != nil {
			t.Fatalf("read manifest: %v", err)
		}
		if !strings.Contains(string(manifestBytes), "sample-chunk-last\tsample-chunk-last.wav\traw chunk two only") {
			t.Fatalf("unexpected corpus manifest: %s", string(manifestBytes))
		}
	}

	// 3. Test save-chunk with 2 args: selector "last" explicitly
	{
		var out bytes.Buffer
		d := depsFor("\n\n", &out)
		cmd := NewCommand(&out, home, rules, 64, nil, d, nil)
		cmd.SetArgs([]string{"sample", "save-chunk", "last", "sample-chunk-last-explicit"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("save-chunk last sample-chunk-last-explicit failed: %v", err)
		}

		manifestBytes, err := os.ReadFile(filepath.Join(home, ".config", "voxi", "samples", "corpus.tsv"))
		if err != nil {
			t.Fatalf("read manifest: %v", err)
		}
		if !strings.Contains(string(manifestBytes), "sample-chunk-last-explicit\tsample-chunk-last-explicit.wav\traw chunk two only") {
			t.Fatalf("unexpected corpus manifest: %s", string(manifestBytes))
		}
	}

	// 4. Test save-chunk with nonexistent index: should return clear error
	{
		var out bytes.Buffer
		d := depsFor("\n\n", &out)
		cmd := NewCommand(&out, home, rules, 64, nil, d, nil)
		cmd.SetArgs([]string{"sample", "save-chunk", "99", "nonexistent"})
		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected error for nonexistent chunk index")
		}
		if !strings.Contains(err.Error(), "chunk 99 not found in recent chunks buffer") {
			t.Fatalf("unexpected error message: %v", err)
		}
	}
}
