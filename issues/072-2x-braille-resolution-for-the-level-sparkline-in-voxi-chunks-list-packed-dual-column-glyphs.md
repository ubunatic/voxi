# 072 — 2x Braille Resolution for the LEVEL Sparkline in voxi chunks list (Packed Dual-Column Glyphs)

**Status**: Proposed
**Priority**: P3 (Low)
**Severity**: Enhancement
**Category**: Feature
**Related**: [070 Show Recorded Volume (RMS) and a Speech-Level Sparkline in voxi chunks list](070-show-recorded-volume-rms-column-in-voxi-chunks-list.md), [071 Colorize the LEVEL Sparkline in voxi chunks list (--color flag, design proposal)](071-colorize-the-level-sparkline-in-voxi-chunks-list-color-flag-design-proposal.md), [internal/audio/audio.go](../internal/audio/audio.go), [internal/chunks/color.go](../internal/chunks/color.go)

---

## 1. Problem & Motivation

The user, looking at real `voxi chunks list` output with the `LEVEL`
sparkline column added by issue 070, observed: "the render resolution is
2x (two dots per time bucket and value) — braille chars can show two
values per cell — we need to 2x the data res (20 points for 10 chars)."

Confirmed against the actual code (`internal/audio/audio.go`): a Unicode
Braille Pattern codepoint (U+2800-U+28FF) encodes 8 independently
addressable dots in a 2-column x 4-row grid — dot1/2/3/7 form the left
column top-to-bottom, dot4/5/6/8 form the right column top-to-bottom.
`sparklineGlyph(level int) rune` (`internal/audio/audio.go:407-422`)
already uses exactly this dot layout — its four possible `dots` byte
values are `0xC0` (dots 7+8), `0xE4` (+3+6), `0xF6` (+2+5), `0xFF` (all
8) — but fills both columns *symmetrically from a single input level*,
so every glyph's left and right dot-columns always represent the same
value. `RenderVolumeSparkline` (`internal/audio/audio.go:342-374`) splits
the chunk's PCM into exactly `buckets` (10) time slices and emits exactly
one glyph per slice. This means the 10-character, 8-dots-per-cell LEVEL
column is currently only using 4 of its available height combinations
and half of each glyph's dot capacity: it encodes 1 data point per
character (10 buckets -> 10 characters) when the format could encode 2
independent data points per character (20 buckets -> 10 characters) at
the same fixed on-screen width.

## 2. Scope

Prescribed core mechanism (well-defined, per the user's specific
request):

- `RenderVolumeSparkline` splits the chunk's PCM into **20** equal time
  buckets instead of 10 (2x the buckets for the same fixed 10-character
  output width), computing each bucket's RMS and quantizing it via the
  existing `sparklineLevel` (`sparklineMinLevel..sparklineLevels`, i.e.
  1-4, logarithmic floor/ceiling calibration from issue 070 unchanged).
- A new glyph-construction function (replacing or extending
  `sparklineGlyph`) that fills the Braille cell's left dot-column
  (dots 1/2/3/7) from one bucket's level and the right dot-column
  (dots 4/5/6/8) from the *next* bucket's level independently — i.e.
  output character `i` is built from underlying buckets `2i` (left) and
  `2i+1` (right), producing 10 output characters from 20 underlying data
  points.
- The existing "no data" (literal ASCII space) semantics for a bucket
  beyond the end of a too-short buffer stay as the no-data signal for a
  *whole* bucket, but see Open Question 2 below for the doubled-bucket
  case where only one side of a pair has no data.

### Cross-ticket impact on issue 071's `--color` feature (needs rework)

Issue 071 added `--color=auto|always|never` and a `sparklineGlyphANSI`
lookup table (`internal/chunks/color.go:89-94`) keyed on the *exact rune
value* of the 4 glyphs `sparklineGlyph` can currently produce (`⣀ ⣤ ⣶
⣿`). With independent left/right levels, the glyph space grows to up to
4 x 4 = 16 distinct runes (not counting the existing no-data space and
whatever floor/asymmetry rules apply), so `sparklineGlyphANSI`'s
closed 4-entry map will silently fail to recognize most of the new
asymmetric glyphs and render them uncolored (the `colorizeSparkline`
fallback path already passes unrecognized runes through unchanged, so
this fails safe/silent rather than erroring — but it defeats the color
feature for most output). This ticket's scope explicitly flags this
lookup table as needing rework in the implementation that picks up this
ticket; see Open Question 1 below for what that rework should be — no
answer is prescribed here.

### Test-update implications (scope item, not implemented here)

The following existing tests assert on the *current* 10-glyph/4-level
/symmetric-dot behavior and will need updating for whatever the packed-
glyph behavior a future implementation ticket settles on:

- `internal/audio/audio_test.go`: `TestRenderVolumeSparkline`,
  `TestSparklineLevelRealWorldCalibration`,
  `TestRenderVolumeSparklineNoData`.
- `internal/chunks/color_test.go`: any test asserting against the
  current closed 4-rune glyph set (e.g. `TestColorizeSparkline`).

## 3. Open Questions (genuinely unresolved — do not prescribe an answer)

1. **How does `--color` (issue 071) interact with independent left/right
   sub-glyph levels?** ANSI color escape codes apply to a whole
   terminal cell/rune, not to sub-glyph dot regions — there is no way to
   color only the left half or only the right half of one Braille
   character in a standard terminal. Candidate directions exist but none
   is chosen here: color the whole packed glyph by whichever of its two
   sub-values is louder (loses precision, "which side" info is dropped);
   color by some blend/average of the two levels (loses both
   endpoints' exact identity); drop per-glyph height coloring entirely
   for this column and fall back to a coarser scheme (e.g. outcome-based,
   per issue 071's own still-open Non-Goal); or replace the closed
   rune-keyed lookup table with a computed color from the decoded dot
   pattern instead of a fixed map. A follow-up ticket should decide.
2. **What does "no data" mean for one half of a packed pair?** The
   current "no data" signal (issue 070) is a whole blank/space glyph for
   a bucket with no underlying samples. With two independent sub-buckets
   packed into one glyph, it's possible for exactly one of a pair to have
   data and the other not (e.g. the buffer ends between bucket `2i` and
   bucket `2i+1`). A whole blank/space glyph can't represent "half real,
   half missing," and filling only one dot-column while leaving the other
   at its floor risks being misread as a real (if quiet) measurement
   rather than an absence. No semantics are prescribed here — a follow-up
   ticket should decide whether to special-case this, treat a
   half-missing pair as fully missing, or something else.

## 4. Acceptance Criteria

This ticket is filed for scoping and design purposes; a follow-up
implementation ticket should:

- Implement 20-bucket PCM splitting and independent left/right
  dot-column glyph packing in `internal/audio/audio.go`, producing a
  10-character `LEVEL` column driven by 20 underlying data points.
- Resolve Open Question 1 (color interaction with issue 071) and update
  `internal/chunks/color.go` accordingly.
- Resolve Open Question 2 (partial no-data semantics) and update
  `RenderVolumeSparkline`'s no-data handling accordingly.
- Update the test-update-implication tests listed in Section 2 to match
  whichever answers are chosen, with exact glyph-by-glyph assertions
  (consistent with existing test rigor in `audio_test.go`), not just
  "non-empty" checks.

## 5. Non-Goals

- Not implemented in this ticket — filing/scoping only, per the user's
  explicit instruction. No code changes.
- Not deciding either Open Question above; both are left for a follow-up
  implementation ticket.
- Not changing the sparkline's bucket count, floor/ceiling RMS
  calibration (`sparklineFloorRMS`=80, `sparklineCeilingRMS`=8192), or
  the 4-level height quantization scheme (`sparklineLevel`) — only how
  those existing per-bucket levels get packed into output characters.
- Not widening the `LEVEL` column's on-screen width — the whole point is
  doubling data resolution within the existing fixed 10-character width.

## 6. Background

Raised 2026-09-06, while the user was looking at real `voxi chunks list`
output from issues 070 (RMS/LEVEL columns) and 071 (`--color` flag), both
Implemented earlier the same day. The user's own words: "the render
resolution is 2x (two dots per time bucket and value) — braille chars
can show two values per cell — we need to 2x the data res (20 points for
10 chars)." Investigation (Section 1) confirmed the technical premise
against the actual `sparklineGlyph` dot-pattern bytes in
`internal/audio/audio.go` before this ticket was written.
