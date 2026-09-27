package clone

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/deps"
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
	samples := filepath.Join(home, ".config", "voxi", "samples")
	if err := os.MkdirAll(samples, 0700); err != nil {
		t.Fatal(err)
	}
	writeCorpus(t, samples, "only-one\tonly-one.wav\tHello there.\t\n")
	writeFile(t, filepath.Join(samples, "only-one.wav"), []byte("RIFF-fake-reference-wav"))

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
	if settings.TTSServeReferenceWav != target {
		t.Errorf("tts_serve_reference_wav = %q, want %q", settings.TTSServeReferenceWav, target)
	}
}

func TestVoiceCloneRequiresSampleFlagWhenMultipleAllowlisted(t *testing.T) {
	home := t.TempDir()
	samples := filepath.Join(home, ".config", "voxi", "samples")
	if err := os.MkdirAll(samples, 0700); err != nil {
		t.Fatal(err)
	}
	writeCorpus(t, samples, "first\tfirst.wav\tFirst.\t\nsecond\tsecond.wav\tSecond.\t\n")
	writeFile(t, filepath.Join(samples, "first.wav"), []byte("first-wav"))
	writeFile(t, filepath.Join(samples, "second.wav"), []byte("second-wav"))

	_, run := newCloneCommand(t, home)
	if err := run(); err == nil {
		t.Fatal("expected an error requiring --sample when multiple samples are allowlisted")
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
	samples := filepath.Join(home, ".config", "voxi", "samples")
	if err := os.MkdirAll(samples, 0700); err != nil {
		t.Fatal(err)
	}
	writeCorpus(t, samples, "only-one\tonly-one.wav\tHello there.\t\n")
	writeFile(t, filepath.Join(samples, "only-one.wav"), []byte("reference"))

	_, run := newCloneCommand(t, home)
	if err := run("--sample", "missing"); err == nil {
		t.Fatal("expected an error for an unknown --sample id")
	}
}
