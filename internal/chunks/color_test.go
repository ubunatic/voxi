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

func TestShouldUseColor(t *testing.T) {
	cases := []struct {
		name       string
		mode       string
		noColorEnv string
		isTerminal bool
		want       bool
	}{
		{"auto+tty", colorAuto, "", true, true},
		{"auto+non-tty", colorAuto, "", false, false},
		{"always+non-tty", colorAlways, "", false, true},
		{"always+tty", colorAlways, "", true, true},
		{"never+tty", colorNever, "", true, false},
		{"never+non-tty", colorNever, "", false, false},
		{"NO_COLOR overrides always", colorAlways, "1", true, false},
		{"NO_COLOR overrides auto+tty", colorAuto, "1", true, false},
		{"NO_COLOR is a no-op on never", colorNever, "1", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldUseColor(tc.mode, tc.noColorEnv, tc.isTerminal)
			if got != tc.want {
				t.Fatalf("shouldUseColor(%q, %q, %v) = %v, want %v", tc.mode, tc.noColorEnv, tc.isTerminal, got, tc.want)
			}
		})
	}
}

func TestIsValidColorMode(t *testing.T) {
	for _, mode := range []string{"auto", "always", "never"} {
		if !isValidColorMode(mode) {
			t.Fatalf("expected %q to be a valid color mode", mode)
		}
	}
	for _, mode := range []string{"", "yes", "ALWAYS", "on"} {
		if isValidColorMode(mode) {
			t.Fatalf("expected %q to be rejected as an invalid color mode", mode)
		}
	}
}

func TestStdoutIsTerminal(t *testing.T) {
	// A *bytes.Buffer (what tests and non-terminal deps.Dependencies use) is
	// never a terminal -- this is also the correct behavior for redirected
	// or piped real output, since only a genuine *os.File can be a tty.
	if stdoutIsTerminal(&bytes.Buffer{}) {
		t.Fatal("expected a *bytes.Buffer to never be reported as a terminal")
	}
}

func TestColorizeSparkline(t *testing.T) {
	plain := "[⣀⣤⣶⣿ ]"
	colored := colorizeSparkline(plain)

	if colored == plain {
		t.Fatalf("expected colorizeSparkline to change the string, got unchanged: %q", colored)
	}
	// Stripping ANSI must recover exactly the original visible content --
	// color must be visually neutral, per issue 071's acceptance criteria.
	if got := asr.StripANSI(colored); got != plain {
		t.Fatalf("colorizeSparkline changed visible content: got %q, want %q", got, plain)
	}
	// Brackets, the "no data" space glyph, and the trailing padding space
	// must stay uncolored (colorizeSparkline only wraps recognized loudness
	// glyphs).
	if strings.Contains(colored, "\x1b[") && strings.HasPrefix(colored, "\x1b[") {
		t.Fatalf("expected the leading '[' to stay uncolored, got %q", colored)
	}
}

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
