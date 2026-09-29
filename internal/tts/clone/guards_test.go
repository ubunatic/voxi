package clone

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ubunatic.com/voxi/internal/sample"
)

func TestPrepareNeverUsesDictationSamples(t *testing.T) {
	root := filepath.Join(t.TempDir(), "samples")
	writeSampleData(t, root, "dictated", sample.Dictation, "Not my voice.", wavBytes(SampleRate, channels, bits))
	_, err := Prepare(t.Context(), Options{StoreRoot: root, OutputDir: t.TempDir(), ConvertAudio: fakeConverter})
	if err == nil || !strings.Contains(err.Error(), "no voice samples") {
		t.Fatalf("Prepare = %v, want no voice samples", err)
	}
}

func TestPrepareRefusesVoiceSampleWithoutConsent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "samples")
	writeSampleData(t, root, "ok", sample.Voice, "Mine.", wavBytes(SampleRate, channels, bits))
	// A sidecar written outside the store API, without consent.
	dir := filepath.Join(root, "voice")
	writeFile(t, filepath.Join(dir, "sneaky.wav"), wavBytes(SampleRate, channels, bits))
	writeFile(t, filepath.Join(dir, "sneaky.json"), []byte(`{"id":"sneaky","transcript":"Someone else.","created":"2026-01-01T00:00:00Z","source":"test"}`))
	_, err := Prepare(t.Context(), Options{StoreRoot: root, OutputDir: t.TempDir(), ConvertAudio: fakeConverter})
	if err == nil || !strings.Contains(err.Error(), "sneaky") {
		t.Fatalf("Prepare = %v, want consent error naming sneaky", err)
	}
}

func TestVoiceCloneWarnsAboutLegacyAllowlist(t *testing.T) {
	home := t.TempDir()
	samples := filepath.Join(home, ".local", "share", "voxi", "samples")
	writeSampleData(t, samples, "only-one", sample.Voice, "Hello there.", []byte("RIFF"))
	legacy := filepath.Join(home, ".config", "voxi", "samples", "voice-training.txt")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, legacy, []byte("dictated\n"))
	out, run := newCloneCommand(t, home)
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "voice-training.txt is ignored") {
		t.Fatalf("missing legacy allowlist note in %q", out.String())
	}
}
