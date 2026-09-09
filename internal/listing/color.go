// Package listing holds table-rendering and sparkline-colorizing code
// shared by `voxi chunks list` and `voxi feedback sample list` (issue 098).
// Both commands list recorded/stored audio items with transcripts and a
// Braille loudness sparkline; this package factors that shared rendering
// out of internal/chunks (its original, chunks-list-specific home) so both
// callers use one implementation instead of growing independent
// reimplementations.
package listing

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
	ColorAuto   = "auto"
	ColorAlways = "always"
	ColorNever  = "never"
)

// ValidColorModes lists the values --color accepts.
var ValidColorModes = []string{ColorAuto, ColorAlways, ColorNever}

// IsValidColorMode reports whether mode is one of ValidColorModes.
func IsValidColorMode(mode string) bool {
	for _, m := range ValidColorModes {
		if mode == m {
			return true
		}
	}
	return false
}

// StdoutIsTerminal reports whether w is a real interactive terminal worth
// colorizing for. Mirrors internal/devsample/lineedit.go's stdinIsTerminal
// but checks stdout instead of stdin: deps.Dependencies.Stdout is a plain
// io.Writer (a *bytes.Buffer in tests, a pipe when the caller redirects
// output), so only a genuine *os.File can ever be a terminal -- anything
// else (including a piped/redirected *os.File, which IsTerminal correctly
// reports as false) is treated as non-interactive, which is exactly the
// right behavior for "auto".
func StdoutIsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

// ShouldUseColor resolves the effective color-enabled decision from the
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
func ShouldUseColor(mode string, noColorEnv string, isTerminal bool) bool {
	if noColorEnv != "" {
		return false
	}
	switch mode {
	case ColorAlways:
		return true
	case ColorNever:
		return false
	default: // "auto" (and any value IsValidColorMode already rejected)
		return isTerminal
	}
}

// sparklineLevelANSI maps a single 1..4 loudness level (see
// internal/audio's sparklineMinLevel..sparklineLevels) to an ANSI color
// code, quietest to loudest -- a dim-to-hot ramp mirroring what glyph
// height already represents, so color reinforces rather than duplicates a
// new meaning. This used to be a map keyed on the exact rune each of the 4
// possible *symmetric* glyphs produced (internal/audio.sparklineGlyph). As
// of issue 072, RenderVolumeSparkline packs two independent levels (left
// dot-column, right dot-column) into each glyph, so up to ~16 distinct
// runes are possible -- far too many to enumerate as a closed rune table.
// Colorizing now decodes each glyph's dot pattern back into its two
// sub-levels (decodeSparklineGlyph) and colors the whole cell by
// whichever side is louder (max(left, right)): a terminal color escape
// applies to the whole cell, not a sub-glyph dot region, so *some*
// precision is unavoidably lost either way (see issue 072 Open Question
// 1); coloring by the max keeps the ramp's meaning intuitive ("this glyph
// contains at least this much loudness") and matches what a viewer's eye
// keys on first -- the brighter/taller-looking half of a mixed glyph --
// rather than a blended average that could land on a color matching
// neither actual sub-value, or an outcome-based scheme that would throw
// away the height-to-color correspondence issue 071 established.
var sparklineLevelANSI = map[int]string{
	1: "90",   // quietest audible level -- dim gray
	2: "36",   // cyan
	3: "33",   // yellow
	4: "31;1", // loudest -- bright red
}

// sparklineLeftDotMask and sparklineRightDotMask identify a Braille cell's
// left dot-column (dots 1/2/3/7) and right dot-column (dots 4/5/6/8)
// bits, mirroring internal/audio's leftColumnDots/rightColumnDots byte
// layout. Duplicated here as bit masks rather than importing audio's
// unexported helpers, since this is presentation-only and audio.go is on
// the live daemon's capture path (touching it would require a service
// restart to test, for a decision that has nothing to do with capture
// behavior). If audio's dot-column layout ever changes, this needs
// updating too.
const (
	sparklineLeftDotMask  = 0x47 // dots 1(0x01)+2(0x02)+3(0x04)+7(0x40)
	sparklineRightDotMask = 0xB8 // dots 4(0x08)+5(0x10)+6(0x20)+8(0x80)
)

// sparklineDotsToLevel reverse-maps one column's dot bits (already masked
// to just that column) back to the 1..4 level that produced them, or 0 if
// the bits are 0 (no data on that side) or don't match any level this
// package's glyphs actually produce.
var sparklineLeftDotsToLevel = map[byte]int{0x40: 1, 0x44: 2, 0x46: 3, 0x47: 4}
var sparklineRightDotsToLevel = map[byte]int{0x80: 1, 0xA0: 2, 0xB0: 3, 0xB8: 4}

// decodeSparklineGlyph reverse-maps a Braille Pattern rune (as produced by
// audio.RenderVolumeSparkline) back into its independent left/right
// sub-levels. ok is false for anything outside the Braille Patterns block
// (U+2800-U+28FF) -- including the "no data" space glyph and the
// surrounding brackets -- so ColorizeSparkline leaves those untouched, the
// same fail-safe behavior the old closed rune map had for anything it
// didn't recognize.
func decodeSparklineGlyph(r rune) (left, right int, ok bool) {
	if r < 0x2800 || r > 0x28FF {
		return 0, 0, false
	}
	dots := byte(r - 0x2800)
	left = sparklineLeftDotsToLevel[dots&sparklineLeftDotMask]
	right = sparklineRightDotsToLevel[dots&sparklineRightDotMask]
	return left, right, true
}

// ColorizeSparkline wraps each recognized loudness glyph in s with its ANSI
// color code, leaving brackets, the "no data" space glyph, and any padding
// spaces uncolored (and thus untouched in byte length beyond the added
// codes).
//
// s must already be at its final display width (i.e. already
// fixed-width-padded) before calling this: fmt measures a string's padding
// width by rune count, so colorizing before padding would make the
// invisible ANSI escape runes count as visible columns and break alignment
// with the other table columns. Padding first and colorizing second (rather
// than writing an ANSI-aware padder like internal/monitor/render.go's
// TruncateLineANSI) keeps this feature's footprint to a single small
// helper, since --color only ever touches one already-fixed-width column.
// FormatSparklineCell in table.go does the padding-then-colorizing for
// callers.
func ColorizeSparkline(s string) string {
	var sb strings.Builder
	for _, r := range s {
		left, right, ok := decodeSparklineGlyph(r)
		if !ok {
			sb.WriteRune(r)
			continue
		}
		level := left
		if right > level {
			level = right
		}
		code, ok := sparklineLevelANSI[level]
		if !ok {
			// Neither side decoded to a known level (e.g. a half-missing
			// pair where the only present side is 0, which shouldn't
			// happen since RenderVolumeSparkline substitutes a literal
			// space when both sides are 0, but stay safe/uncolored rather
			// than panicking on an unexpected dot pattern).
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
