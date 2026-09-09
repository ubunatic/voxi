package eager

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"ubunatic.com/voxi/internal/deps"
)

// TestTranscribeCohereWAV exercises the standalone spot-check entry point
// (issue 098's `sample list --process`) end to end against a fake crispasr
// binary that echoes a known transcript, mirroring
// TestRunEagerDaemonReachesReadyWithoutVoxtype's cached-weights/fake-binary
// setup without spinning up a real daemon.
func TestTranscribeCohereWAV(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))
	weightsPath, err := cohereWeightsPath()
	if err != nil {
		t.Fatalf("cohereWeightsPath(): %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(weightsPath), 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(weightsPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(cohereGGUFMinBytes); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	fakeCrispasr := filepath.Join(tmp, "fake-crispasr.sh")
	script := "#!/bin/sh\necho 'hello from crispasr'\n"
	if err := os.WriteFile(fakeCrispasr, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	d := deps.Dependencies{
		LookPath: func(name string) (string, error) {
			if name == crispasrBinary {
				return fakeCrispasr, nil
			}
			return "", fmt.Errorf("exec: %q: executable file not found in $PATH", name)
		},
		Stdout: io.Discard,
	}

	wavPath := filepath.Join(tmp, "sample.wav")
	if err := os.WriteFile(wavPath, []byte("not real audio, never read by the fake binary"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := TranscribeCohereWAV(context.Background(), d, wavPath)
	if err != nil {
		t.Fatalf("TranscribeCohereWAV: %v", err)
	}
	if got != "hello from crispasr\n" {
		t.Fatalf("TranscribeCohereWAV() = %q, want %q", got, "hello from crispasr\n")
	}
}

// TestTranscribeCohereWAVMissingBinary confirms a clear error (not a panic
// or silent empty result) when crispasr isn't on PATH.
func TestTranscribeCohereWAVMissingBinary(t *testing.T) {
	tmp := t.TempDir()
	d := deps.Dependencies{
		LookPath: func(name string) (string, error) {
			return "", fmt.Errorf("exec: %q: executable file not found in $PATH", name)
		},
		Stdout: io.Discard,
	}
	if _, err := TranscribeCohereWAV(context.Background(), d, filepath.Join(tmp, "sample.wav")); err == nil {
		t.Fatal("expected an error when crispasr is not on PATH")
	}
}
