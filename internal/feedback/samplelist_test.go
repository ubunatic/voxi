package feedback

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/audio"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/devsample"
)

// writeFixtureSample writes a minimal private-corpus sample: a WAV file plus
// a manifest entry, mirroring devsample.Record's on-disk shape without
// exercising the interactive capture/prompt flow.
func writeFixtureSample(t *testing.T, home, name, text string) {
	t.Helper()
	if err := os.MkdirAll(devsample.SamplesDir(home), 0o700); err != nil {
		t.Fatalf("create samples dir: %v", err)
	}
	pcm := bytes.Repeat([]byte{0, 1, 0, 2}, 4000) // 8000 samples @ 16-bit, ~0.5s @ 16kHz
	if err := audio.WriteWAVAudio(devsample.WAVPath(home, name), pcm, 16000); err != nil {
		t.Fatalf("write fixture wav: %v", err)
	}
	existing, err := devsample.LoadManifest(home)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	s := devsample.Sample{Name: name, WAVFile: name + ".wav", Text: text, Timestamp: time.Now()}
	if err := devsample.SaveManifest(home, devsample.Upsert(existing, s)); err != nil {
		t.Fatalf("save manifest: %v", err)
	}
}

func TestSampleListShortIsCompactAndOmitsSourceByDefault(t *testing.T) {
	home := t.TempDir()
	writeFixtureSample(t, home, "one", "hello there")

	var out bytes.Buffer
	cmd := NewCommand(&out, home, rules, 64, nil, deps.Dependencies{Stdout: &out}, nil)
	cmd.SetArgs([]string{"sample", "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sample list failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "one") || !strings.Contains(got, "hello there") {
		t.Fatalf("expected sample name and text in short output, got:\n%s", got)
	}
	if strings.Contains(got, "SOURCE") {
		t.Fatalf("expected no SOURCE column without --all, got:\n%s", got)
	}
	if strings.Contains(got, "DURATION") || strings.Contains(got, "LEVEL") {
		t.Fatalf("expected no acoustic-stats columns in --short output, got:\n%s", got)
	}
}

func TestSampleListAllMergesPrivateAndPublicWithSourceColumn(t *testing.T) {
	home := t.TempDir()
	writeFixtureSample(t, home, "priv-one", "private text")

	// This test's cwd stays the repo root by construction (no os.Chdir), so
	// devsample.PublicSamplesDir("") resolves relative to it -- point the
	// public corpus somewhere isolated instead by overriding via a second
	// private-style write into a temp "public" dir and confirming through
	// the same loader used in production would require cwd control we don't
	// want in a unit test, so this test only exercises the merge/label
	// behavior directly at the loadListedSamples level.
	publicDir := t.TempDir()
	pcm := bytes.Repeat([]byte{0, 1}, 4000)
	if err := audio.WriteWAVAudio(filepath.Join(publicDir, "pub-one.wav"), pcm, 16000); err != nil {
		t.Fatalf("write public fixture wav: %v", err)
	}
	pub := devsample.Sample{Name: "pub-one", WAVFile: "pub-one.wav", Text: "public text", Timestamp: time.Now()}
	if err := devsample.SaveManifestIn(publicDir, []devsample.Sample{pub}); err != nil {
		t.Fatalf("save public manifest: %v", err)
	}

	private, err := devsample.LoadManifest(home)
	if err != nil {
		t.Fatalf("load private: %v", err)
	}
	public, err := devsample.LoadManifestIn(publicDir)
	if err != nil {
		t.Fatalf("load public: %v", err)
	}
	if len(private) != 1 || len(public) != 1 {
		t.Fatalf("expected one private and one public sample, got %d/%d", len(private), len(public))
	}

	var out bytes.Buffer
	if err := writeShortSampleTableForTest(&out, private, public); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "SOURCE") {
		t.Fatalf("expected SOURCE column when merging corpora, got:\n%s", got)
	}
	if !strings.Contains(got, sourcePrivate) || !strings.Contains(got, sourcePublic) {
		t.Fatalf("expected both source labels present, got:\n%s", got)
	}
}

// writeShortSampleTableForTest exercises writeShortSampleTable's SOURCE-column
// behavior directly against hand-built private/public sample slices, without
// depending on devsample.PublicSamplesDir("")'s repo-root-relative
// resolution (which a unit test shouldn't rely on the process cwd for).
func writeShortSampleTableForTest(out *bytes.Buffer, private, public []devsample.Sample) error {
	samples := make([]listedSample, 0, len(private)+len(public))
	for _, s := range private {
		samples = append(samples, listedSample{Sample: s, Source: sourcePrivate})
	}
	for _, s := range public {
		samples = append(samples, listedSample{Sample: s, Source: sourcePublic})
	}
	writeShortSampleTable(out, samples, true)
	return nil
}

func TestSampleListFullShowsOnTheFlyAcousticStats(t *testing.T) {
	home := t.TempDir()
	writeFixtureSample(t, home, "one", "hello there")

	var out bytes.Buffer
	cmd := NewCommand(&out, home, rules, 64, nil, deps.Dependencies{Stdout: &out}, nil)
	cmd.SetArgs([]string{"sample", "list", "--full"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sample list --full failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "DURATION") || !strings.Contains(got, "RMS") || !strings.Contains(got, "LEVEL") {
		t.Fatalf("expected acoustic-stats columns in --full output, got:\n%s", got)
	}
	if !strings.Contains(got, "0.5s") {
		t.Fatalf("expected computed duration (~0.5s for the 8000-sample fixture), got:\n%s", got)
	}
}

func TestSampleListProcessRequiresFull(t *testing.T) {
	home := t.TempDir()
	writeFixtureSample(t, home, "one", "hello there")

	var out bytes.Buffer
	cmd := NewCommand(&out, home, rules, 64, nil, deps.Dependencies{Stdout: &out}, nil)
	cmd.SetArgs([]string{"sample", "list", "--process"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error when --process is used without --full")
	}
}

func TestSampleListProcessWithoutTranscribeFuncErrors(t *testing.T) {
	home := t.TempDir()
	writeFixtureSample(t, home, "one", "hello there")

	var out bytes.Buffer
	cmd := NewCommand(&out, home, rules, 64, nil, deps.Dependencies{Stdout: &out}, nil)
	cmd.SetArgs([]string{"sample", "list", "--full", "--process"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error when --process is used with no transcribe engine wired up")
	}
}

func TestSampleListProcessShowsFreshTranscriptNextToGroundTruth(t *testing.T) {
	home := t.TempDir()
	writeFixtureSample(t, home, "one", "hello there")

	stub := func(_ context.Context, _ deps.Dependencies, wavPath string) (string, error) {
		if !strings.HasSuffix(wavPath, "one.wav") {
			t.Fatalf("unexpected wav path passed to transcribe stub: %s", wavPath)
		}
		return "fresh transcript\n", nil
	}

	var out bytes.Buffer
	cmd := NewCommand(&out, home, rules, 64, nil, deps.Dependencies{Stdout: &out}, stub)
	cmd.SetArgs([]string{"sample", "list", "--full", "--process"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sample list --full --process failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "PROCESSED") {
		t.Fatalf("expected PROCESSED column header, got:\n%s", got)
	}
	if !strings.Contains(got, "fresh transcript") {
		t.Fatalf("expected the stub's fresh transcript in output, got:\n%s", got)
	}
	if !strings.Contains(got, "hello there") {
		t.Fatalf("expected stored ground-truth text still shown, got:\n%s", got)
	}
}
