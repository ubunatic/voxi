package listing

import (
	"bytes"
	"strings"
	"testing"

	"ubunatic.com/voxi/internal/asr"
)

func TestShouldUseColor(t *testing.T) {
	cases := []struct {
		name       string
		mode       string
		noColorEnv string
		isTerminal bool
		want       bool
	}{
		{"auto+tty", ColorAuto, "", true, true},
		{"auto+non-tty", ColorAuto, "", false, false},
		{"always+non-tty", ColorAlways, "", false, true},
		{"always+tty", ColorAlways, "", true, true},
		{"never+tty", ColorNever, "", true, false},
		{"never+non-tty", ColorNever, "", false, false},
		{"NO_COLOR overrides always", ColorAlways, "1", true, false},
		{"NO_COLOR overrides auto+tty", ColorAuto, "1", true, false},
		{"NO_COLOR is a no-op on never", ColorNever, "1", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ShouldUseColor(tc.mode, tc.noColorEnv, tc.isTerminal)
			if got != tc.want {
				t.Fatalf("ShouldUseColor(%q, %q, %v) = %v, want %v", tc.mode, tc.noColorEnv, tc.isTerminal, got, tc.want)
			}
		})
	}
}

func TestIsValidColorMode(t *testing.T) {
	for _, mode := range []string{"auto", "always", "never"} {
		if !IsValidColorMode(mode) {
			t.Fatalf("expected %q to be a valid color mode", mode)
		}
	}
	for _, mode := range []string{"", "yes", "ALWAYS", "on"} {
		if IsValidColorMode(mode) {
			t.Fatalf("expected %q to be rejected as an invalid color mode", mode)
		}
	}
}

func TestStdoutIsTerminal(t *testing.T) {
	// A *bytes.Buffer (what tests and non-terminal deps.Dependencies use) is
	// never a terminal -- this is also the correct behavior for redirected
	// or piped real output, since only a genuine *os.File can be a tty.
	if StdoutIsTerminal(&bytes.Buffer{}) {
		t.Fatal("expected a *bytes.Buffer to never be reported as a terminal")
	}
}

func TestColorizeSparkline(t *testing.T) {
	// ⣀⣤⣶⣿ are still valid (now symmetric-by-coincidence) packed glyphs --
	// e.g. ⣤ = leftColumnDots(2)|rightColumnDots(2) = 0x44|0xA0 = 0xE4 -- so
	// they still exercise the quietest..loudest ramp end to end.
	plain := "[⣀⣤⣶⣿ ]"
	colored := ColorizeSparkline(plain)

	if colored == plain {
		t.Fatalf("expected ColorizeSparkline to change the string, got unchanged: %q", colored)
	}
	// Stripping ANSI must recover exactly the original visible content --
	// color must be visually neutral, per issue 071's acceptance criteria.
	if got := asr.StripANSI(colored); got != plain {
		t.Fatalf("ColorizeSparkline changed visible content: got %q, want %q", got, plain)
	}
	// Brackets, the "no data" space glyph, and the trailing padding space
	// must stay uncolored (ColorizeSparkline only wraps recognized loudness
	// glyphs).
	if strings.Contains(colored, "\x1b[") && strings.HasPrefix(colored, "\x1b[") {
		t.Fatalf("expected the leading '[' to stay uncolored, got %q", colored)
	}
}

// TestDecodeSparklineGlyph locks in the reverse dot-pattern-to-level mapping
// ColorizeSparkline relies on (issue 072): a rune outside the Braille
// Patterns block decodes as not-ok, and a packed asymmetric glyph decodes
// back to its exact original left/right levels.
func TestDecodeSparklineGlyph(t *testing.T) {
	if _, _, ok := decodeSparklineGlyph('['); ok {
		t.Fatalf("expected '[' to not decode as a sparkline glyph")
	}
	if _, _, ok := decodeSparklineGlyph(' '); ok {
		t.Fatalf("expected the no-data space to not decode as a sparkline glyph")
	}

	// leftColumnDots(4)=0x47, rightColumnDots(1)=0x80 -> dots 0xC7.
	mixed := rune(0x2800 + 0xC7)
	left, right, ok := decodeSparklineGlyph(mixed)
	if !ok || left != 4 || right != 1 {
		t.Fatalf("decodeSparklineGlyph(%q) = (%d, %d, %v), want (4, 1, true)", string(mixed), left, right, ok)
	}

	// A partial-pair glyph (issue 072 Open Question 2): left real (level 3),
	// right missing (0 dots).
	partial := rune(0x2800 + 0x46) // leftColumnDots(3), right = 0x00
	left, right, ok = decodeSparklineGlyph(partial)
	if !ok || left != 3 || right != 0 {
		t.Fatalf("decodeSparklineGlyph(%q) = (%d, %d, %v), want (3, 0, true)", string(partial), left, right, ok)
	}
}

// TestColorizeSparklineAsymmetricGlyph ensures a packed glyph whose two
// sub-columns hold *different* levels still gets colored (issue 072 Open
// Question 1: color by the louder of the two sides) rather than silently
// falling through uncolored the way the old closed 4-rune lookup table
// would have for any of the ~16 new asymmetric glyphs.
func TestColorizeSparklineAsymmetricGlyph(t *testing.T) {
	// left = level 1 (quiet), right = level 4 (loud) -> colored by the
	// louder side (4, bright red "31;1").
	mixed := rune(0x2800 + 0x40 + 0xB8) // leftColumnDots(1) | rightColumnDots(4)
	plain := "[" + string(mixed) + "]"
	colored := ColorizeSparkline(plain)

	if !strings.Contains(colored, "\x1b[31;1m"+string(mixed)+"\x1b[0m") {
		t.Fatalf("expected asymmetric glyph %q to be colored by its louder (level 4) side, got %q", string(mixed), colored)
	}
	if got := asr.StripANSI(colored); got != plain {
		t.Fatalf("ColorizeSparkline changed visible content for asymmetric glyph: got %q, want %q", got, plain)
	}

	// left = level 4 (loud), right = level 1 (quiet) -- same louder-side
	// color regardless of which column carries it.
	mixed2 := rune(0x2800 + 0x47 + 0x80) // leftColumnDots(4) | rightColumnDots(1)
	plain2 := "[" + string(mixed2) + "]"
	colored2 := ColorizeSparkline(plain2)
	if !strings.Contains(colored2, "\x1b[31;1m"+string(mixed2)+"\x1b[0m") {
		t.Fatalf("expected asymmetric glyph %q to be colored by its louder (level 4) side, got %q", string(mixed2), colored2)
	}
	if got := asr.StripANSI(colored2); got != plain2 {
		t.Fatalf("ColorizeSparkline changed visible content for asymmetric glyph: got %q, want %q", got, plain2)
	}
}

// TestColorizeSparklinePartialNoData ensures a partial-pair glyph (one side
// real, one side 0 dots for "no data", per issue 072 Open Question 2) still
// colors by its one real, present level rather than falling through
// uncolored or erroring.
func TestColorizeSparklinePartialNoData(t *testing.T) {
	partial := rune(0x2800 + 0x46) // leftColumnDots(3), right side missing
	plain := "[" + string(partial) + "]"
	colored := ColorizeSparkline(plain)

	if !strings.Contains(colored, "\x1b[33m"+string(partial)+"\x1b[0m") {
		t.Fatalf("expected partial-pair glyph %q to be colored by its present level (3, yellow), got %q", string(partial), colored)
	}
	if got := asr.StripANSI(colored); got != plain {
		t.Fatalf("ColorizeSparkline changed visible content for partial-pair glyph: got %q, want %q", got, plain)
	}
}
