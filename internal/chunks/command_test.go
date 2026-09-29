package chunks

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
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
		VolumeSparkline:       "⣶⣶⣶⣶⣶⣀⣀⣀⣀⣀",
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
		VolumeSparkline:       "⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀",
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
	if !strings.Contains(outStr, "(low_energy_transient)") {
		t.Fatalf("expected rejection reason in list output, got:\n%s", outStr)
	}
	if !strings.Contains(outStr, "RMS") || !strings.Contains(outStr, "LEVEL") || !strings.Contains(outStr, "TRANSCRIPT / REASON") {
		t.Fatalf("expected RMS, LEVEL, and TRANSCRIPT / REASON column headers in list output, got:\n%s", outStr)
	}
	// Accepted chunk (c1): mean RMS 842 and its loud-then-quiet sparkline,
	// bracketed so the LEVEL column reads as a bounded meter.
	if !strings.Contains(outStr, "842") || !strings.Contains(outStr, "[⣶⣶⣶⣶⣶⣀⣀⣀⣀⣀]") {
		t.Fatalf("expected accepted chunk's RMS/bracketed sparkline in list output, got:\n%s", outStr)
	}
	// Rejected (low_energy_transient) chunk (c2): mean RMS 95 and its flat-low sparkline.
	if !strings.Contains(outStr, "95") || !strings.Contains(outStr, "[⣀⣀⣀⣀⣀⣀⣀⣀⣀⣀]") {
		t.Fatalf("expected rejected chunk's RMS/bracketed sparkline in list output, got:\n%s", outStr)
	}

	// Test show last command (default json format)
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

	// Test show text format
	out.Reset()
	cmd = NewCommand(d, buf)
	cmd.SetArgs([]string{"show", "1", "--format", "text"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("chunks show 1 --format text failed: %v", err)
	}
	textShow := out.String()
	if !strings.Contains(textShow, "Chunk #1 Diagnostics & Pipeline Summary:") ||
		!strings.Contains(textShow, "Pipeline Transformations:") ||
		!strings.Contains(textShow, `1. Raw ASR Output:      "hello world"`) ||
		!strings.Contains(textShow, `4. Final Committed:     "hello world"`) {
		t.Fatalf("unexpected show text trace output:\n%s", textShow)
	}
}

func TestChunksCommandDelete(t *testing.T) {
	newBuffer := func(t *testing.T) *Buffer {
		t.Helper()
		buf := NewBuffer(t.TempDir(), 10)
		for i := 0; i < 3; i++ {
			if _, err := buf.Add(Chunk{CleanedTranscript: string(rune('a' + i))}, []byte{1, 2, 3}, 16000); err != nil {
				t.Fatal(err)
			}
		}
		return buf
	}

	t.Run("single and last", func(t *testing.T) {
		buf := newBuffer(t)
		for i, selector := range []string{"2", "last"} {
			cmd := NewCommand(deps.Dependencies{Stdout: &bytes.Buffer{}}, buf)
			cmd.SetArgs([]string{"delete", selector})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("delete %s: %v", selector, err)
			}
			if i == 0 {
				got, err := buf.List(false)
				if err != nil {
					t.Fatal(err)
				}
				if len(got) != 2 || got[0].Index != 1 || got[1].Index != 3 {
					t.Fatalf("list after deleting index 2 = %#v, want indices 1 and 3", got)
				}
			}
		}
		got, err := buf.List(false)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].Index != 1 {
			t.Fatalf("remaining chunks = %#v, want only index 1", got)
		}
		for _, index := range []int{2, 3} {
			for _, suffix := range []string{"wav", "json"} {
				if _, err := os.Stat(filepath.Join(buf.Dir(), fmt.Sprintf("chunk_%04d.%s", index, suffix))); !os.IsNotExist(err) {
					t.Errorf("chunk %d %s remains or stat failed: %v", index, suffix, err)
				}
			}
		}
	})

	t.Run("all", func(t *testing.T) {
		buf := newBuffer(t)
		cmd := NewCommand(deps.Dependencies{Stdout: &bytes.Buffer{}}, buf)
		cmd.SetArgs([]string{"delete", "--all", "--yes"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		got, err := buf.List(false)
		if err != nil || len(got) != 0 {
			t.Fatalf("remaining chunks = %d, err = %v; want empty", len(got), err)
		}
	})

	t.Run("bad index", func(t *testing.T) {
		buf := newBuffer(t)
		cmd := NewCommand(deps.Dependencies{Stdout: &bytes.Buffer{}}, buf)
		cmd.SetArgs([]string{"delete", "99"})
		if err := cmd.Execute(); err == nil {
			t.Fatal("delete bad index succeeded")
		}
		got, _ := buf.List(false)
		if len(got) != 3 {
			t.Fatalf("bad index changed store: %d chunks remain", len(got))
		}
	})

	t.Run("empty store", func(t *testing.T) {
		cmd := NewCommand(deps.Dependencies{Stdout: &bytes.Buffer{}}, NewBuffer(t.TempDir(), 10))
		cmd.SetArgs([]string{"delete", "last"})
		if err := cmd.Execute(); err == nil {
			t.Fatal("delete from empty store succeeded")
		}
	})

	t.Run("selector required", func(t *testing.T) {
		buf := newBuffer(t)
		cmd := NewCommand(deps.Dependencies{Stdout: &bytes.Buffer{}}, buf)
		cmd.SetArgs([]string{"delete"})
		if err := cmd.Execute(); err == nil {
			t.Fatal("delete without selector succeeded")
		}
		got, _ := buf.List(false)
		if len(got) != 3 {
			t.Fatalf("missing selector changed store: %d chunks remain", len(got))
		}
	})
}

func TestFormatStatusBadges(t *testing.T) {
	tests := []struct {
		name  string
		chunk Chunk
		want  string
	}{
		{
			name: "accepted cohere with llm modified",
			chunk: Chunk{
				Accepted:              true,
				Engine:                "cohere-transcribe",
				Model:                 "cohere-transcribe-03-2026",
				TranscribeDurationSec: 0.42,
				LLMCleanup:            &LLMCleanupRecord{Enabled: true, Modified: true, Output: "cleaned"},
			},
			want: "✓ ⚡ 🤖",
		},
		{
			name: "accepted cohere with replacement",
			chunk: Chunk{
				Accepted:              true,
				Engine:                "cohere-transcribe",
				Model:                 "cohere-transcribe-03-2026",
				TranscribeDurationSec: 0.39,
				AppliedReplacements:   []ReplacementSummary{{From: "Voxy", To: "voxi"}},
			},
			want: "✓ ⚡ ⇄",
		},
		{
			name: "accepted plain whisper",
			chunk: Chunk{
				Accepted:              true,
				Engine:                "whisper",
				Model:                 "small.en",
				TranscribeDurationSec: 0.44,
			},
			want: "✓ 👂",
		},
		{
			name: "rejected unvoiced transient (no engine ran)",
			chunk: Chunk{
				Accepted:        false,
				Engine:          "cohere-transcribe",
				Model:           "cohere-transcribe-03-2026",
				RejectionReason: "low_energy_transient",
			},
			want: "✗",
		},
		{
			name: "rejected stop word on cohere",
			chunk: Chunk{
				Accepted:              false,
				Engine:                "cohere-transcribe",
				Model:                 "cohere-transcribe-03-2026",
				TranscribeDurationSec: 0.40,
				StopWordsMatched:      []string{"thanks for watching"},
				RejectionReason:       "stop_word_matched: thanks-for-watching",
			},
			want: "✗ ⚡ ✂",
		},
		{
			name: "rejected safety circuit breaker",
			chunk: Chunk{
				Accepted:              false,
				Engine:                "whisper",
				Model:                 "base.en",
				TranscribeDurationSec: 0.50,
				RejectionReason:       "pathological_repetition",
			},
			want: "✗ 👂 🛡",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatStatusBadges(tt.chunk)
			if got != tt.want {
				t.Errorf("FormatStatusBadges() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatChunkDetailsTrace(t *testing.T) {
	chunk := Chunk{
		Index:                 209,
		Timestamp:             time.Date(2026, 9, 13, 17, 7, 26, 0, time.Local),
		AudioDurationSecs:     2.36,
		RTF:                   0.44,
		Model:                 "cohere-transcribe-03-2026",
		Engine:                "cohere-transcribe",
		Accepted:              true,
		RawTranscript:         "This is Chunk One with Voxy.",
		CleanedTranscript:     "This is Chunk 1 with Voxi.",
		AppliedReplacements:   []ReplacementSummary{{From: "Voxy", To: "voxi"}},
		LLMCleanup:            &LLMCleanupRecord{Enabled: true, Model: "qwen3-4b-instruct-2507-q4", Output: "This is Chunk 1 with Voxi.", Modified: true},
		TranscribeDurationSec: 1.04,
		TypingStartedAt:       time.Unix(100, 0),
		TypingEndedAt:         time.Unix(100, 8*int64(time.Millisecond)),
	}

	var buf bytes.Buffer
	FormatChunkDetails(&buf, chunk)
	out := buf.String()

	expectedSubstrings := []string{
		"Chunk #209 Diagnostics & Pipeline Summary:",
		"ASR Engine:             cohere-transcribe-03-2026 (via crispasr)",
		"Status:                 ACCEPTED",
		"Pipeline Transformations:",
		`1. Raw ASR Output:      "This is Chunk One with Voxy."`,
		`2. Replacements:        "Voxy" -> "voxi"`,
		"3. LLM Cleanup:         qwen3-4b-instruct-2507-q4 (via lmcoder)",
		`   LLM Output:          "This is Chunk 1 with Voxi."`,
		`4. Final Committed:     "This is Chunk 1 with Voxi."`,
		"Destination:            Focused Window via dotool (type_delay_ms = 0ms)",
		"Latency:                1.04s transcribe + 8ms typing = 1.05s total",
	}

	for _, s := range expectedSubstrings {
		if !strings.Contains(out, s) {
			t.Errorf("FormatChunkDetails missing %q, got:\n%s", s, out)
		}
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
