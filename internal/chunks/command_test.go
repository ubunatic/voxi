package chunks

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/deps"
)

func TestChunksCommandListAndShow(t *testing.T) {
	dir := t.TempDir()
	buf := NewBuffer(dir, 5)

	dummyPCM := make([]byte, 3200)
	c1 := Chunk{
		Index:                 1,
		Timestamp:             time.Now(),
		AudioDurationSecs:     1.5,
		TranscribeDurationSec: 0.3,
		RTF:                   0.2,
		MeanRMS:               842,
		PeakRMS:               1200,
		VolumeSparkline:       "⣶⣶⣶⣶⣶⠀⠀⠀⠀⠀",
		RawTranscript:         "hello world",
		CleanedTranscript:     "hello world",
		Accepted:              true,
	}
	c2 := Chunk{
		Index:                 2,
		Timestamp:             time.Now(),
		AudioDurationSecs:     0.8,
		TranscribeDurationSec: 0.2,
		RTF:                   0.25,
		MeanRMS:               95,
		PeakRMS:               140,
		VolumeSparkline:       "⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀",
		RawTranscript:         "bye.",
		CleanedTranscript:     "bye",
		Accepted:              false,
		RejectionReason:       "low_energy_transient",
	}

	if _, err := buf.Add(c1, dummyPCM, 16000); err != nil {
		t.Fatal(err)
	}
	if _, err := buf.Add(c2, dummyPCM, 16000); err != nil {
		t.Fatal(err)
	}

	// Test list command
	var out bytes.Buffer
	d := deps.Dependencies{Stdout: &out}
	cmd := NewCommand(d, buf)
	cmd.SetArgs([]string{"list"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("chunks list failed: %v", err)
	}

	outStr := out.String()
	if !strings.Contains(outStr, "#1") || !strings.Contains(outStr, "#2") {
		t.Fatalf("expected chunks in list output, got:\n%s", outStr)
	}
	if !strings.Contains(outStr, "rej:low_energy_transient") {
		t.Fatalf("expected rejection reason in list output, got:\n%s", outStr)
	}
	if !strings.Contains(outStr, "RMS") || !strings.Contains(outStr, "LEVEL") {
		t.Fatalf("expected RMS and LEVEL column headers in list output, got:\n%s", outStr)
	}
	// Accepted chunk (c1): mean RMS 842 and its loud-then-quiet sparkline.
	if !strings.Contains(outStr, "842") || !strings.Contains(outStr, "⣶⣶⣶⣶⣶⠀⠀⠀⠀⠀") {
		t.Fatalf("expected accepted chunk's RMS/sparkline in list output, got:\n%s", outStr)
	}
	// Rejected (low_energy_transient) chunk (c2): mean RMS 95 and its flat-low sparkline.
	if !strings.Contains(outStr, "95") || !strings.Contains(outStr, "⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀") {
		t.Fatalf("expected rejected chunk's RMS/sparkline in list output, got:\n%s", outStr)
	}

	// Test show last command
	out.Reset()
	cmd = NewCommand(d, buf)
	cmd.SetArgs([]string{"show", "last"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("chunks show last failed: %v", err)
	}
	if !strings.Contains(out.String(), `"index": 2`) || !strings.Contains(out.String(), `"rejection_reason": "low_energy_transient"`) {
		t.Fatalf("unexpected show output:\n%s", out.String())
	}

	// Test show index 1
	out.Reset()
	cmd = NewCommand(d, buf)
	cmd.SetArgs([]string{"show", "1"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("chunks show 1 failed: %v", err)
	}
	if !strings.Contains(out.String(), `"index": 1`) || !strings.Contains(out.String(), `"hello world"`) {
		t.Fatalf("unexpected show 1 output:\n%s", out.String())
	}
}

func TestChunksCommandPlay(t *testing.T) {
	dir := t.TempDir()
	buf := NewBuffer(dir, 5)

	dummyPCM := make([]byte, 3200)
	c1 := Chunk{Index: 1, Accepted: true}
	if _, err := buf.Add(c1, dummyPCM, 16000); err != nil {
		t.Fatal(err)
	}

	var played string
	d := deps.Dependencies{
		LookPath: func(name string) (string, error) {
			if name == "aplay" {
				return "/usr/bin/aplay", nil
			}
			return "", os.ErrNotExist
		},
		Run: func(ctx context.Context, name string, args ...string) error {
			played = name + " " + strings.Join(args, " ")
			return nil
		},
	}

	cmd := NewCommand(d, buf)
	cmd.SetArgs([]string{"play", "1"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("chunks play failed: %v", err)
	}
	if !strings.HasPrefix(played, "aplay ") || !strings.HasSuffix(played, "chunk_0001.wav") {
		t.Fatalf("unexpected play invocation: %q", played)
	}
}
