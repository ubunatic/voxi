package devsample

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/audio"
	"ubunatic.com/voxi/internal/deps"
)

func fakeCapture(pcm []byte, err error) func(context.Context, deps.Dependencies, *bufio.Reader) ([]byte, time.Duration, error) {
	return func(context.Context, deps.Dependencies, *bufio.Reader) ([]byte, time.Duration, error) {
		if err != nil {
			return nil, 0, err
		}
		return pcm, time.Second, nil
	}
}

func TestRecordPersistsWAVAndManifestAtomically(t *testing.T) {
	home := t.TempDir()
	orig := captureFn
	captureFn = fakeCapture(bytes.Repeat([]byte{0, 1}, 8000), nil)
	defer func() { captureFn = orig }()

	out := &bytes.Buffer{}
	stdin := strings.NewReader("This is the corrected transcript.\n")
	d := deps.Dependencies{Stdout: out, Stdin: stdin}

	if err := Record(context.Background(), d, home, "greeting", false); err != nil {
		t.Fatalf("Record: %v", err)
	}

	wavPath := WAVPath(home, "greeting")
	info, err := os.Stat(wavPath)
	if err != nil {
		t.Fatalf("stat wav: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("wav perm = %v, want 0600", info.Mode().Perm())
	}

	samples, err := LoadManifest(home)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	s, ok := Find(samples, "greeting")
	if !ok {
		t.Fatal("recorded sample missing from manifest")
	}
	if s.Text != "This is the corrected transcript." {
		t.Errorf("manifest text = %q", s.Text)
	}
	if s.WAVFile != "greeting.wav" {
		t.Errorf("manifest wav file = %q", s.WAVFile)
	}

	// No leftover .tmp files.
	entries, _ := os.ReadDir(SamplesDir(home))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

func TestRecordRejectsEmptyTranscript(t *testing.T) {
	home := t.TempDir()
	orig := captureFn
	captureFn = fakeCapture([]byte{0, 1, 2, 3}, nil)
	defer func() { captureFn = orig }()

	d := deps.Dependencies{Stdout: &bytes.Buffer{}, Stdin: strings.NewReader("\n")}
	err := Record(context.Background(), d, home, "empty", false)
	if err == nil {
		t.Fatal("expected error for empty transcript")
	}
	if _, statErr := os.Stat(WAVPath(home, "empty")); !os.IsNotExist(statErr) {
		t.Error("no half-written sample should be left behind when the transcript is rejected")
	}
}

func TestRecordCaptureFailureLeavesNoFiles(t *testing.T) {
	home := t.TempDir()
	orig := captureFn
	captureFn = fakeCapture(nil, errNotFound("no audio device"))
	defer func() { captureFn = orig }()

	d := deps.Dependencies{Stdout: &bytes.Buffer{}, Stdin: strings.NewReader("text\n")}
	err := Record(context.Background(), d, home, "sample1", false)
	if err == nil {
		t.Fatal("expected capture error to propagate")
	}
	if _, statErr := os.Stat(SamplesDir(home)); statErr == nil {
		entries, _ := os.ReadDir(SamplesDir(home))
		if len(entries) != 0 {
			t.Errorf("expected no files after failed capture, got %v", entries)
		}
	}
}

func TestRecordOverwriteRequiresConfirmation(t *testing.T) {
	home := t.TempDir()
	orig := captureFn
	captureFn = fakeCapture([]byte{0, 1, 2, 3}, nil)
	defer func() { captureFn = orig }()

	// First recording succeeds.
	d := deps.Dependencies{Stdout: &bytes.Buffer{}, Stdin: strings.NewReader("first text\n")}
	if err := Record(context.Background(), d, home, "dup", false); err != nil {
		t.Fatalf("first Record: %v", err)
	}

	// Second attempt without confirmation ("n") must be rejected and must
	// not touch the existing sample.
	d = deps.Dependencies{Stdout: &bytes.Buffer{}, Stdin: strings.NewReader("n\n")}
	if err := Record(context.Background(), d, home, "dup", false); err == nil {
		t.Fatal("expected overwrite to be rejected without confirmation")
	}
	samples, _ := LoadManifest(home)
	s, _ := Find(samples, "dup")
	if s.Text != "first text" {
		t.Errorf("unconfirmed overwrite modified sample: %q", s.Text)
	}

	// Confirmed overwrite ("y" then new transcript) succeeds.
	d = deps.Dependencies{Stdout: &bytes.Buffer{}, Stdin: strings.NewReader("y\nsecond text\n")}
	if err := Record(context.Background(), d, home, "dup", false); err != nil {
		t.Fatalf("confirmed overwrite Record: %v", err)
	}
	samples, _ = LoadManifest(home)
	s, _ = Find(samples, "dup")
	if s.Text != "second text" {
		t.Errorf("confirmed overwrite text = %q, want %q", s.Text, "second text")
	}

	// force=true skips confirmation entirely.
	d = deps.Dependencies{Stdout: &bytes.Buffer{}, Stdin: strings.NewReader("third text\n")}
	if err := Record(context.Background(), d, home, "dup", true); err != nil {
		t.Fatalf("forced overwrite Record: %v", err)
	}
	samples, _ = LoadManifest(home)
	s, _ = Find(samples, "dup")
	if s.Text != "third text" {
		t.Errorf("forced overwrite text = %q, want %q", s.Text, "third text")
	}
}

func TestRemoveDeletesWAVAndManifestEntry(t *testing.T) {
	home := t.TempDir()
	orig := captureFn
	captureFn = fakeCapture([]byte{0, 1, 2, 3}, nil)
	defer func() { captureFn = orig }()

	d := deps.Dependencies{Stdout: &bytes.Buffer{}, Stdin: strings.NewReader("text\n")}
	if err := Record(context.Background(), d, home, "toremove", false); err != nil {
		t.Fatalf("Record: %v", err)
	}

	if err := Remove(home, "toremove"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(WAVPath(home, "toremove")); !os.IsNotExist(err) {
		t.Error("wav file should be deleted")
	}
	samples, _ := LoadManifest(home)
	if _, ok := Find(samples, "toremove"); ok {
		t.Error("manifest entry should be removed")
	}
}

func TestRemoveMissingSampleErrors(t *testing.T) {
	home := t.TempDir()
	if err := Remove(home, "nope"); err == nil {
		t.Fatal("expected error removing a nonexistent sample")
	}
}

func TestPlayMissingSampleErrors(t *testing.T) {
	home := t.TempDir()
	d := deps.Dependencies{}
	if err := Play(context.Background(), d, home, "nope"); err == nil {
		t.Fatal("expected error playing a nonexistent sample")
	}
}

func TestPlayerCommandNoneAvailable(t *testing.T) {
	d := deps.Dependencies{LookPath: func(string) (string, error) { return "", errNotFound("not found") }}
	_, _, err := PlayerCommand(d)
	if err == nil {
		t.Fatal("expected error when no player is available")
	}
}

func TestPromptTextUnEditedEnterAcceptsRawDefault(t *testing.T) {
	out := &bytes.Buffer{}
	in := bufio.NewReader(strings.NewReader("\n"))
	got, err := promptText(out, in, nil, "prompt: ", "raw asr guess")
	if err != nil {
		t.Fatalf("promptText: %v", err)
	}
	if got != "raw asr guess" {
		t.Errorf("promptText() = %q, want raw default", got)
	}
	if !strings.Contains(out.String(), "raw asr guess") {
		t.Errorf("promptText() did not display the raw default: %q", out.String())
	}
}

func TestPromptTextTypedLineOverridesRawDefault(t *testing.T) {
	in := bufio.NewReader(strings.NewReader("a real correction\n"))
	got, err := promptText(&bytes.Buffer{}, in, nil, "prompt: ", "raw asr guess")
	if err != nil {
		t.Fatalf("promptText: %v", err)
	}
	if got != "a real correction" {
		t.Errorf("promptText() = %q, want typed override", got)
	}
}

func TestPromptTextNoDefaultStillRequiresInput(t *testing.T) {
	in := bufio.NewReader(strings.NewReader("\n"))
	_, err := promptText(&bytes.Buffer{}, in, nil, "prompt: ", "")
	if err == nil {
		t.Fatal("expected error for blank input with no raw default, matching pre-045 behavior")
	}
}

func TestPromptKeytermsBlankAcceptsSuggestion(t *testing.T) {
	in := bufio.NewReader(strings.NewReader("\n"))
	got, err := promptKeyterms(&bytes.Buffer{}, in, nil, "Voxi|dotool")
	if err != nil {
		t.Fatalf("promptKeyterms: %v", err)
	}
	if got != "Voxi|dotool" {
		t.Errorf("promptKeyterms() = %q, want suggestion", got)
	}
}

func TestPromptKeytermsTypedLineReplacesSuggestion(t *testing.T) {
	in := bufio.NewReader(strings.NewReader("custom, terms\n"))
	got, err := promptKeyterms(&bytes.Buffer{}, in, nil, "Voxi|dotool")
	if err != nil {
		t.Fatalf("promptKeyterms: %v", err)
	}
	if got != "custom|terms" {
		t.Errorf("promptKeyterms() = %q, want %q", got, "custom|terms")
	}
}

func TestPromptKeytermsEmptySuggestionAndBlankInputStaysEmpty(t *testing.T) {
	in := bufio.NewReader(strings.NewReader("\n"))
	got, err := promptKeyterms(&bytes.Buffer{}, in, nil, "")
	if err != nil {
		t.Fatalf("promptKeyterms: %v", err)
	}
	if got != "" {
		t.Errorf("promptKeyterms() = %q, want empty (unchanged fallback behavior)", got)
	}
}

func TestRecordUsesRawTranscriptAndKeytermSuggestions(t *testing.T) {
	home := t.TempDir()
	origCapture := captureFn
	captureFn = fakeCapture(bytes.Repeat([]byte{0, 1}, 8000), nil)
	defer func() { captureFn = origCapture }()

	origTranscribe := transcribeFn
	transcribeFn = func(context.Context, deps.Dependencies, []byte) (string, error) {
		return "Voxi uses voxtype with dotool.", nil
	}
	defer func() { transcribeFn = origTranscribe }()

	out := &bytes.Buffer{}
	// Blank line accepts the raw transcript verbatim; blank second line
	// accepts the suggested keyterms.
	stdin := strings.NewReader("\n\n")
	d := deps.Dependencies{Stdout: out, Stdin: stdin}

	if err := Record(context.Background(), d, home, "keyterm-sample", false); err != nil {
		t.Fatalf("Record: %v", err)
	}

	samples, err := LoadManifest(home)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	s, ok := Find(samples, "keyterm-sample")
	if !ok {
		t.Fatal("recorded sample missing from manifest")
	}
	if s.Text != "Voxi uses voxtype with dotool." {
		t.Errorf("manifest text = %q, want raw transcript accepted verbatim", s.Text)
	}
	if s.Keyterms == "" {
		t.Fatal("expected suggested keyterms to be populated, got empty")
	}
	for _, want := range []string{"Voxi", "voxtype", "dotool"} {
		if !strings.Contains(s.Keyterms, want) {
			t.Errorf("manifest keyterms %q missing suggested term %q", s.Keyterms, want)
		}
	}
}

func TestRecordFallsBackToBlankTranscriptWhenTranscribeUnavailable(t *testing.T) {
	home := t.TempDir()
	origCapture := captureFn
	captureFn = fakeCapture([]byte{0, 1, 2, 3}, nil)
	defer func() { captureFn = origCapture }()

	origTranscribe := transcribeFn
	transcribeFn = func(context.Context, deps.Dependencies, []byte) (string, error) {
		return "", errNotFound("voxtype not installed")
	}
	defer func() { transcribeFn = origTranscribe }()

	out := &bytes.Buffer{}
	d := deps.Dependencies{Stdout: out, Stdin: strings.NewReader("manual transcript\n\n")}
	if err := Record(context.Background(), d, home, "no-raw", false); err != nil {
		t.Fatalf("Record: %v", err)
	}
	samples, _ := LoadManifest(home)
	s, ok := Find(samples, "no-raw")
	if !ok {
		t.Fatal("recorded sample missing from manifest")
	}
	if s.Text != "manual transcript" {
		t.Errorf("manifest text = %q, want manually typed text", s.Text)
	}
	if !strings.Contains(out.String(), "raw ASR transcript unavailable") {
		t.Errorf("expected a note about the unavailable raw transcript, got: %q", out.String())
	}
}

func TestSanitizeTextCollapsesWhitespace(t *testing.T) {
	got := sanitizeText("hello\tworld\r\nfoo\nbar  ")
	if strings.ContainsAny(got, "\t\r\n") {
		t.Errorf("sanitizeText left raw whitespace: %q", got)
	}
}

func TestPathsAreUnderConfigVoxiSamples(t *testing.T) {
	home := t.TempDir()
	want := filepath.Join(home, ".config", "voxi", "samples")
	if SamplesDir(home) != want {
		t.Errorf("SamplesDir = %q, want %q", SamplesDir(home), want)
	}
}

func TestPromoteMovesSampleToPublicCorpusAsFLAC(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}

	home := t.TempDir()
	publicDir := filepath.Join(t.TempDir(), "noise-samples")

	if err := os.MkdirAll(SamplesDir(home), 0o700); err != nil {
		t.Fatalf("create private samples dir: %v", err)
	}
	if err := audio.WriteWAVAudio(WAVPath(home, "clack-1"), bytes.Repeat([]byte{0, 1}, 8000), 16000); err != nil {
		t.Fatalf("write fixture wav: %v", err)
	}
	original := Sample{Name: "clack-1", WAVFile: "clack-1.wav", Text: "[keyboard noise]", Timestamp: time.Now()}
	if err := SaveManifest(home, []Sample{original}); err != nil {
		t.Fatalf("save private manifest: %v", err)
	}

	if err := Promote(context.Background(), home, publicDir, "clack-1"); err != nil {
		t.Fatalf("Promote: %v", err)
	}

	if _, err := os.Stat(WAVPath(home, "clack-1")); !os.IsNotExist(err) {
		t.Errorf("private wav still present after promote (err=%v)", err)
	}
	private, err := LoadManifest(home)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if _, ok := Find(private, "clack-1"); ok {
		t.Error("promoted sample still present in private manifest")
	}

	public, err := LoadManifestIn(publicDir)
	if err != nil {
		t.Fatalf("LoadManifestIn(public): %v", err)
	}
	s, ok := Find(public, "clack-1")
	if !ok {
		t.Fatal("promoted sample missing from public manifest")
	}
	if s.WAVFile != "clack-1.flac" {
		t.Errorf("public WAVFile = %q, want clack-1.flac", s.WAVFile)
	}
	if s.Text != "[keyboard noise]" {
		t.Errorf("public Text = %q, want original text preserved", s.Text)
	}
	if _, err := os.Stat(filepath.Join(publicDir, "clack-1.flac")); err != nil {
		t.Errorf("public flac file missing: %v", err)
	}
}
