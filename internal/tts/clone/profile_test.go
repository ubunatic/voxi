package clone

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/sample"
)

func newCloneCommand(t *testing.T, home string) (*bytes.Buffer, func(args ...string) error) {
	t.Helper()
	out := &bytes.Buffer{}
	getenv := func(key string) string {
		if key == "HOME" {
			return home
		}
		return ""
	}
	run := func(args ...string) error {
		cmd := NewCloneCommand(deps.Dependencies{Stdout: out, Getenv: getenv})
		cmd.SetArgs(args)
		cmd.SetOut(out)
		cmd.SetErr(out)
		return cmd.Execute()
	}
	return out, run
}

func TestVoiceCloneInstallsSingleAllowlistedSample(t *testing.T) {
	home := t.TempDir()
	samples := filepath.Join(home, ".local", "share", "voxi", "samples")
	if err := os.MkdirAll(samples, 0700); err != nil {
		t.Fatal(err)
	}
	writeSampleData(t, samples, "only-one", sample.Voice, "Hello there.", []byte("RIFF-fake-reference-wav"))

	out, run := newCloneCommand(t, home)
	if err := run(); err != nil {
		t.Fatalf("voice clone: %v (output: %s)", err, out)
	}

	target := filepath.Join(home, ".local", "share", "voxi", "voices", "cloned.wav")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read installed profile: %v", err)
	}
	if string(data) != "RIFF-fake-reference-wav" {
		t.Errorf("installed WAV = %q, want copied reference bytes", data)
	}

	settings, err := config.LoadUserSettings(home)
	if err != nil {
		t.Fatal(err)
	}
	if settings.TTSVoiceReferenceWav != target {
		t.Errorf("tts_voice_reference_wav = %q, want %q", settings.TTSVoiceReferenceWav, target)
	}
}

func TestVoiceCloneRequiresSampleFlagWhenMultipleAllowlisted(t *testing.T) {
	home := t.TempDir()
	samples := filepath.Join(home, ".local", "share", "voxi", "samples")
	if err := os.MkdirAll(samples, 0700); err != nil {
		t.Fatal(err)
	}
	writeSampleData(t, samples, "first", sample.Voice, "First.", []byte("first-wav"))
	writeSampleData(t, samples, "second", sample.Voice, "Second.", []byte("second-wav"))

	_, run := newCloneCommand(t, home)
	if err := run(); err == nil {
		t.Fatal("expected an error requiring --sample when multiple voice samples exist")
	}

	if err := run("--sample", "second"); err != nil {
		t.Fatalf("voice clone --sample second: %v", err)
	}
	target := filepath.Join(home, ".local", "share", "voxi", "voices", "cloned.wav")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read installed profile: %v", err)
	}
	if string(data) != "second-wav" {
		t.Errorf("installed WAV = %q, want second-wav", data)
	}
}

func TestVoiceCloneRejectsUnknownSample(t *testing.T) {
	home := t.TempDir()
	samples := filepath.Join(home, ".local", "share", "voxi", "samples")
	if err := os.MkdirAll(samples, 0700); err != nil {
		t.Fatal(err)
	}
	writeSampleData(t, samples, "only-one", sample.Voice, "Hello there.", []byte("reference"))

	_, run := newCloneCommand(t, home)
	if err := run("--sample", "missing"); err == nil {
		t.Fatal("expected an error for an unknown --sample id")
	}
}
