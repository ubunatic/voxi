# Chunk Diagnostics: `voxi chunks` Ring Buffer, RMS, and the LEVEL Sparkline

Reference for `internal/chunks`, `internal/audio`'s sparkline/level code, and the
`voxi chunks list`/`show` CLI. Read this before touching the sparkline rendering,
the acoustic-gate RMS fields, or the `--color` flag — the calibration and glyph-packing
choices here are non-obvious and were arrived at by trial, not derived from a formula
you can re-derive from first principles.

## 1. Purpose

`voxi chunks` (issue 053) keeps a bounded ring buffer (default 10) of the most
recently recorded audio segments and their transcription/acoustic-gate outcome, so a
user or agent can inspect *why* a chunk was accepted or rejected without re-recording.
Storage lives at `$XDG_RUNTIME_DIR/voxi/chunks` (or an OS-appropriate fallback);
`internal/chunks/chunks.go`'s `Chunk` struct is the persisted record, `internal/chunks/command.go`
implements `list`/`show`/`play`.

## 2. The acoustic gate, and which fields diagnose it

`internal/audio.AudioSegmenter` (issue 054) accepts or rejects a candidate segment
based on `SegmenterOptions` thresholds — `ThresholdRMS` (per-frame voice/silence cut,
default 150) and `MinMeanRMS` (default 120, gates the whole segment's average). A
rejection reason of `low_energy_transient` means the segment's overall or per-frame
RMS never cleared these; `unvoiced_transient` means too few frames were voiced at all.

Two fields on `Chunk` carry the raw evidence: `MeanRMS` and `PeakRMS` (populated in
`internal/eager/eager.go` from `audio.AnalyzePCM`). `voxi chunks show` prints them
directly (`Mean / Peak RMS: %d / %d`). `voxi chunks list` shows `MeanRMS` in its `RMS`
column — a single aggregate number, useful for comparing chunks against the gate's own
thresholds, but it cannot show *where in the recording* the energy was, which is what
motivated the LEVEL sparkline below (issue 070: a chunk with one brief loud syllable and
otherwise-silence has the same mean as one that's uniformly moderate, but the two are
acoustically very different, and `low_energy_transient`'s own name promises exactly this
"transient" distinction that a single mean number can't show).

## 3. The LEVEL sparkline

`internal/audio.RenderVolumeSparkline(pcmData, buckets)` renders a fixed-width Braille
string showing RMS energy over the chunk's duration. It's computed once, at
chunk-finalize time (alongside `MeanRMS`/`PeakRMS`, in `internal/eager/eager.go`), and
stored as `Chunk.VolumeSparkline` — never recomputed on read, matching the existing
precompute-and-store pattern for the other RMS fields. `voxi chunks list` displays it
in a `LEVEL` column, bracketed (`[...]`) so it reads as a bounded meter rather than a
stray glyph string floating in whitespace.

### 3.1 Dual-column packing (2x resolution, issue 072)

A Unicode Braille Pattern character (U+2800-U+28FF) is a 2-column × 4-row grid of 8
independently-addressable dots — it can encode **two** independent values per
character, not one. `RenderVolumeSparkline` exploits this: for a `buckets`-character
output (default 10), it computes `2*buckets` (20) time sub-buckets, and packs each
consecutive pair into one glyph — the earlier sub-bucket fills the **left** column, the
later fills the **right** — giving double the time resolution in the same on-screen
width.

Dot-bit layout (standard Braille Patterns numbering, as used by this code):

| Column | Dots (top→bottom) | Bit mask |
|---|---|---|
| Left  | 1, 2, 3, 7 | `0x47` (`0x01\|0x02\|0x04\|0x40`) |
| Right | 4, 5, 6, 8 | `0xB8` (`0x08\|0x10\|0x20\|0x80`) |

Each column independently fills bottom-up across 4 levels (`sparklineMinLevel`=1 to
`sparklineLevels`=4) — `leftColumnDots`/`rightColumnDots` in `internal/audio/audio.go`
hold the exact per-level byte values (derived by masking the four whole-glyph bytes the
original, pre-072 symmetric encoder used — `0xC0`/`0xE4`/`0xF6`/`0xFF` — against each
column's mask). `packedSparklineGlyph(left, right)` ORs the two column bytes together
(the masks are disjoint, so this never collides) and adds `0x2800`.

### 3.2 "No data" vs. "measured and silent" — never conflate these

A literal ASCII space means **no sample exists for this bucket at all** (the buffer was
too short to fill every requested sub-bucket — only possible for very short, sub-100ms
recordings). This is a *different* fact from "a bucket was measured and found quiet",
which always renders as at least the minimum audible glyph, `⣀` (dots 7+8, i.e. the
bottom row of both columns) — never blank. `sparklineLevel` never returns below
`sparklineMinLevel` (1) for real measured audio; a 0-level only reaches the
glyph-construction code via the explicit no-data path. Conflating the two (an earlier,
now-fixed version of this code rendered *both* as blank Braille) makes a genuinely
quiet-but-real recording indistinguishable from a gap where nothing was ever recorded —
exactly the ambiguity a diagnostic tool must not have.

When a packed pair has data on only one side (rare — only near a buffer's short-buffer
edge case), the missing side renders as 0 dots in its column while the present side
keeps its real level; only a *fully* missing pair renders as a space.

### 3.3 RMS-to-level calibration: logarithmic, floor 80, ceiling 8192

`sparklineLevel(rms)` maps a raw RMS value to 1..4 via a **logarithmic**, not linear,
scale between `sparklineFloorRMS` (80) and `sparklineCeilingRMS` (8192).

**Why logarithmic:** human speech RMS commonly spans tens to low-thousands. A linear
scale against any single ceiling either crushes ordinary speech to the minimum glyph
(ceiling too high) or saturates everything to the maximum glyph (ceiling too low) — there
is no single linear ceiling that works across that range.

**Why floor 80:** sits just below the acoustic gate's own `MinMeanRMS` (120), so a
`low_energy_transient`-rejected chunk reads as visibly flat-and-low, not by coincidence
but by design — the sparkline's floor is deliberately tied to the same threshold that
drives real accept/reject decisions.

**Why ceiling 8192, not 2048 (the value first shipped):** *this is the pitfall to
remember.* The first calibration was tuned only against synthetic test tones (fixed-RMS
sine waves at convenient round numbers like 20/184/3000) and looked fine in unit tests.
Real recorded chunks told a different story: per-bucket RMS in ordinary conversational
speech routinely spikes to 600-1500, and a ceiling of 2048 was already mapping that into
2-3 out of 4 dots on **every** recording — every chunk looked "loud" regardless of
actual volume, because there was almost no headroom left above typical speech. This was
only caught when a user looked at real `voxi chunks list` output and said the levels
looked too high for how quiet the recordings actually were. The fix (raising the
ceiling to 8192, 25% of int16 full-scale headroom) was verified by extracting real
per-bucket RMS from actual stored `.wav` chunk files and locking those exact values into
a regression test (`TestSparklineLevelRealWorldCalibration` in
`internal/audio/audio_test.go`) asserting they land below the maximum level.

**Lesson for future calibration work in this codebase (or similar quantization/binning
of real-world sensor data anywhere in voxi): synthetic unit tests with hand-picked
values are necessary but not sufficient.** They confirm the code does what you told it
to; they cannot tell you whether what you told it to do matches reality. Before
considering a calibrated scale "done," pull real captured data through the same code
path and sanity-check the *distribution* of outputs it produces, not just a couple of
edge cases. (This generalizes the existing anti-pattern documented in
`~/.claude/docs/AgenticLoop.md` §"Unit-Test-Only Confidence for Hook/Environment
Features" — the same failure mode, here in an audio-quantization feature rather than a
hook/environment-resolution one.)

## 4. Color (`--color`, issue 071)

`voxi chunks list --color=auto|always|never` (default `auto`) colorizes the `LEVEL`
column by loudness — a dim-to-hot ANSI ramp (`internal/chunks/color.go`,
`sparklineLevelANSI`: gray→cyan→yellow→bright-red for levels 1-4).

- **TTY detection**: `stdoutIsTerminal` type-asserts the output writer to `*os.File`
  and calls `golang.org/x/term.IsTerminal` on its file descriptor — already a direct
  dependency (used elsewhere for stdin), so no new dependency was needed. Anything that
  isn't a real `*os.File` (a `bytes.Buffer` in tests, a pipe) is correctly treated as
  non-interactive.
- **`NO_COLOR`** (any non-empty value) forces color off, overriding even
  `--color=always` — this repo already sets `NO_COLOR` for subprocess invocations
  elsewhere (`internal/eager`, `internal/devsample`), so respecting it here too is
  consistent with existing precedent, not a new convention.
- **Padding-before-coloring**: the `LEVEL` string is padded to its fixed display width
  *first* (`fmt.Sprintf("%-12s", ...)`), then colorized — ANSI escape bytes are added
  only after width is already fixed, so they can never be miscounted as visible columns
  by `fmt`'s width-padding (which counts runes, and would count invisible escape-sequence
  runes as real width if color were applied first).
- **Per-glyph coloring after 072's dual-column packing**: with two independent
  sub-values packed per glyph, up to ~16 distinct glyph runes are possible (not just the
  original 4 symmetric ones), so a closed rune→color lookup table doesn't scale.
  `decodeSparklineGlyph` reverse-maps a glyph's dot bits back to its `(left, right)`
  sub-levels (masking against the same `0x47`/`0xB8` column masks §3.1 uses), and
  `colorizeSparkline` colors the whole glyph by `max(left, right)`. This is a real,
  unavoidable trade-off: **an ANSI color escape applies to a whole terminal cell, not to
  a sub-glyph dot region** — you cannot color the left and right dot-columns of one
  character independently. Coloring by the louder side was chosen over a blended average
  (which can land on a color matching neither actual sub-value) and over dropping
  per-glyph color precision entirely (which throws away information for no gain, since
  "one color per cell" is the unavoidable constraint either way).

## 5. Where the code lives

| Concern | File |
|---|---|
| `Chunk` struct, ring buffer | `internal/chunks/chunks.go` |
| `list`/`show`/`play` commands, column layout | `internal/chunks/command.go` |
| `--color` flag, TTY/`NO_COLOR` resolution, glyph decode+colorize | `internal/chunks/color.go` |
| Acoustic gate (`AudioSegmenter`, thresholds, rejection reasons) | `internal/audio/audio.go` (`SegmenterOptions`, `CheckCandidateAcoustics`) |
| Sparkline rendering, level quantization, glyph packing | `internal/audio/audio.go` (`RenderVolumeSparkline`, `sparklineLevel`, `packedSparklineGlyph`, `leftColumnDots`/`rightColumnDots`) |
| Populating `MeanRMS`/`PeakRMS`/`VolumeSparkline` at finalize time | `internal/eager/eager.go` |

Tests: `internal/audio/audio_test.go` (sparkline rendering, calibration regression,
no-data handling), `internal/chunks/command_test.go` (table rendering),
`internal/chunks/color_test.go` (color resolution logic, ANSI round-tripping via
`internal/asr.StripANSI`).

## 6. Related issues

[053](../issues/053-ring-buffer-recent-audio-chunks-and-transcripts.md) (ring buffer),
[054](../issues/054-short-pause-acoustic-gating-and-context-priming.md) (acoustic gate),
[070](../issues/070-show-recorded-volume-rms-column-in-voxi-chunks-list.md) (RMS column +
sparkline, including the three post-implementation calibration fixes §3.3 draws on),
[071](../issues/071-colorize-the-level-sparkline-in-voxi-chunks-list-color-flag-design-proposal.md)
(`--color`), [072](../issues/072-2x-braille-resolution-for-the-level-sparkline-in-voxi-chunks-list-packed-dual-column-glyphs.md)
(dual-column resolution doubling).
