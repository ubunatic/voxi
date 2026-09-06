package chunks

import (
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// --color flag values, matching the git/ls/grep tri-state convention: "auto"
// (default) colorizes only when stdout is a real interactive terminal,
// "always" and "never" force the decision regardless of terminal detection.
// A simpler bool flag was considered (see issue 071) but cobra's plain
// BoolVar can't cleanly distinguish "flag not passed" (auto-detect) from
// "flag explicitly passed as false" (force off) without inspecting
// Flag.Changed, which is more surprising to callers than a tri-state string
// virtually every color-capable CLI already trains users on.
const (
	colorAuto   = "auto"
	colorAlways = "always"
	colorNever  = "never"
)

// validColorModes lists the values --color accepts.
var validColorModes = []string{colorAuto, colorAlways, colorNever}

// isValidColorMode reports whether mode is one of validColorModes.
func isValidColorMode(mode string) bool {
	for _, m := range validColorModes {
		if mode == m {
			return true
		}
	}
	return false
}

// stdoutIsTerminal reports whether w is a real interactive terminal worth
// colorizing for. Mirrors internal/devsample/lineedit.go's stdinIsTerminal
// but checks stdout instead of stdin: deps.Dependencies.Stdout is a plain
// io.Writer (a *bytes.Buffer in tests, a pipe when the caller redirects
// output), so only a genuine *os.File can ever be a terminal -- anything
// else (including a piped/redirected *os.File, which IsTerminal correctly
// reports as false) is treated as non-interactive, which is exactly the
// right behavior for "auto".
func stdoutIsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

// shouldUseColor resolves the effective color-enabled decision from the
// --color flag value, the NO_COLOR env var, and whether stdout is a real
// terminal (consulted only for "auto").
//
// Per the well-known https://no-color.org convention (already precedented
// in this repo -- internal/eager/eager.go and internal/devsample/transcribe.go
// set NO_COLOR=1 when invoking the voxtype/whisper subprocess), any
// non-empty NO_COLOR value forces color off. This deliberately overrides
// even an explicit --color=always: NO_COLOR is normally an environment-wide
// opt-out the user (or their terminal/CI environment) sets once, so honoring
// it unconditionally is safer than letting a leftover --color=always in a
// script or alias silently defeat it.
func shouldUseColor(mode string, noColorEnv string, isTerminal bool) bool {
	if noColorEnv != "" {
		return false
	}
	switch mode {
	case colorAlways:
		return true
	case colorNever:
		return false
	default: // "auto" (and any value isValidColorMode already rejected)
		return isTerminal
	}
}

// sparklineGlyphANSI maps the four Braille height glyphs
// audio.RenderVolumeSparkline can emit (via its unexported sparklineGlyph,
// internal/audio/audio.go) to an ANSI color code, quietest to loudest --
// a dim-to-hot ramp mirroring what the glyph height already represents, so
// color reinforces rather than duplicates a new meaning. The rune values
// are the closed 4-glyph set that function's dot-pattern bytes (0xC0, 0xE4,
// 0xF6, 0xFF) produce; deliberately duplicated here as rune literals rather
// than importing audio's unexported bits, since this is presentation-only
// and audio.go is on the live daemon's capture path (touching it would
// require a service restart to test, for a decision that has nothing to do
// with capture behavior). If audio.sparklineGlyph's dot patterns ever
// change, this map needs updating too.
var sparklineGlyphANSI = map[rune]string{
	'⣀': "90",   // dots 7+8 only: quietest audible level -- dim gray
	'⣤': "36",   // + dots 3+6: cyan
	'⣶': "33",   // + dots 2+5: yellow
	'⣿': "31;1", // all 8 dots: loudest -- bright red
}

// colorizeSparkline wraps each recognized loudness glyph in s with its ANSI
// color code, leaving brackets, the "no data" space glyph, and any padding
// spaces uncolored (and thus untouched in byte length beyond the added
// codes).
//
// s must already be at its final display width (i.e. already
// fixed-width-padded via fmt's "%-Ns") before calling this: fmt measures a
// string's padding width by rune count, so colorizing before padding would
// make the invisible ANSI escape runes count as visible columns and break
// alignment with the other table columns. Padding first and colorizing
// second (rather than writing an ANSI-aware padder like
// internal/monitor/render.go's TruncateLineANSI) keeps this feature's
// footprint to a single small helper, since --color only ever touches one
// already-fixed-width column.
func colorizeSparkline(s string) string {
	var sb strings.Builder
	for _, r := range s {
		code, ok := sparklineGlyphANSI[r]
		if !ok {
			sb.WriteRune(r)
			continue
		}
		sb.WriteString("\x1b[")
		sb.WriteString(code)
		sb.WriteString("m")
		sb.WriteRune(r)
		sb.WriteString("\x1b[0m")
	}
	return sb.String()
}
