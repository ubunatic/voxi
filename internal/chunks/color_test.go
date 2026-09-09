package chunks

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/asr"
	"ubunatic.com/voxi/internal/deps"
)

// TestChunksListColorFlag exercises `chunks list --color` end to end through
// the cobra command. The underlying color-mode resolution and sparkline
// colorizing logic moved to internal/listing as part of issue 098 (shared
// with `sample list`); its unit tests now live in
// internal/listing/color_test.go, so this test only needs to confirm the
// command still wires --color/NO_COLOR through correctly.
func TestChunksListColorFlag(t *testing.T) {
	dir := t.TempDir()
	buf := NewBuffer(dir, 5)
	dummyPCM := make([]byte, 3200)
	c := Chunk{
		Index:             1,
		Timestamp:         time.Now(),
		AudioDurationSecs: 1.0,
		MeanRMS:           842,
		VolumeSparkline:   "⣶⣶⣶⣶⣶⣀⣀⣀⣀⣀",
		RawTranscript:     "hi",
		CleanedTranscript: "hi",
		Accepted:          true,
	}
	if _, err := buf.Add(c, dummyPCM, 16000); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) string {
		var out bytes.Buffer
		d := deps.Dependencies{Stdout: &out, Getenv: func(string) string { return "" }}
		cmd := NewCommand(d, buf)
		cmd.SetArgs(append([]string{"list"}, args...))
		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("chunks list %v failed: %v", args, err)
		}
		return out.String()
	}

	// Default (auto) against a *bytes.Buffer stdout: never a terminal, so no
	// color codes should appear.
	autoOut := run()
	if strings.Contains(autoOut, "\x1b[") {
		t.Fatalf("expected no ANSI codes in default (auto, non-tty) output, got:\n%q", autoOut)
	}

	// --color=always must force color on even though stdout isn't a tty.
	alwaysOut := run("--color=always")
	if !strings.Contains(alwaysOut, "\x1b[") {
		t.Fatalf("expected ANSI codes in --color=always output, got:\n%q", alwaysOut)
	}
	// Column alignment (STATUS/TRANSCRIPT) must be unaffected by the
	// invisible color bytes: stripping ANSI must reproduce the uncolored
	// rendering exactly.
	if stripped := asr.StripANSI(alwaysOut); stripped != autoOut {
		t.Fatalf("colorized output does not match plain output once ANSI-stripped.\nstripped: %q\nplain:    %q", stripped, autoOut)
	}

	// --color=never must force color off.
	neverOut := run("--color=never")
	if strings.Contains(neverOut, "\x1b[") {
		t.Fatalf("expected no ANSI codes in --color=never output, got:\n%q", neverOut)
	}

	// NO_COLOR must override --color=always.
	var out bytes.Buffer
	d := deps.Dependencies{Stdout: &out, Getenv: func(k string) string {
		if k == "NO_COLOR" {
			return "1"
		}
		return ""
	}}
	cmd := NewCommand(d, buf)
	cmd.SetArgs([]string{"list", "--color=always"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("chunks list --color=always with NO_COLOR failed: %v", err)
	}
	if strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("expected NO_COLOR to override --color=always, got:\n%q", out.String())
	}

	// Invalid --color values must be rejected.
	var errOut bytes.Buffer
	d = deps.Dependencies{Stdout: &errOut}
	cmd = NewCommand(d, buf)
	cmd.SetArgs([]string{"list", "--color=bogus"})
	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("expected an error for an invalid --color value")
	}
}
