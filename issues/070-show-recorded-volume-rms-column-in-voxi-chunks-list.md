# 070: Show Recorded Volume (RMS) and a Speech-Level Sparkline in `voxi chunks list`

**Status**: Proposed
**Priority**: P3 (Low)
**Severity**: Enhancement
**Category**: Enhancement
**Related**: [053 Ring buffer of recent audio chunks and transcripts](053-ring-buffer-recent-audio-chunks-and-transcripts.md), [054 Short-pause acoustic gating and context priming](054-short-pause-acoustic-gating-and-context-priming.md), [internal/chunks/command.go](../internal/chunks/command.go), [internal/audio/audio.go](../internal/audio/audio.go)

---

## 1. Problem & Motivation

`voxi chunks list` prints INDEX / TIMESTAMP / AUDIO / RTF / STATUS /
TRANSCRIPT for the last 10 recorded chunks. Several rows show
`rej:low_energy_transient` (the acoustic-gating rejection added in issue
054), but the table gives no actual volume number, so the user cannot tell
*how* quiet a rejected chunk was, nor whether an *accepted* chunk was
recorded unusually quiet vs. normal — useful for noticing mic/gain
problems before they cause a string of rejections.

This is a small, low-risk **"surface an existing value"** change, not new
instrumentation. The metric is already computed and already stored per
chunk — it's just not printed by `list`:

- `Chunk.MeanRMS` and `Chunk.PeakRMS` (`internal/chunks/chunks.go:38-39`)
  are populated from `AudioStats.MeanRMS` / `AudioStats.PeakRMS`
  (`internal/eager/eager.go:358-359` and `:421-422`), which come from
  `audio.AnalyzePCM` (`internal/audio/audio.go:329-367`).
- `voxi chunks show` (`internal/chunks/command.go:125`) already prints
  `Mean / Peak RMS: %d / %d` for a single chunk — `list` just never grew
  the equivalent column.
- The exact rejection this ticket is about,
  `low_energy_transient`, is decided in
  `internal/audio/audio.go:126-134` by comparing `stats.MeanRMS` against
  `opts.MinMeanRMS` (default 120) and the per-frame `ThresholdRMS`
  (default 150, `internal/audio/audio.go:26,33`) — i.e. the gate itself
  thresholds on **raw linear RMS amplitude**, not dBFS or a percentage.

## 2. Metric/Unit Choice

Display the same raw integer RMS units the acoustic gate already
thresholds on (`MeanRMS`, optionally `PeakRMS`), not a converted dBFS or
0-100 value:

- It's the number that actually drove the accept/reject decision, so a
  displayed value the user can compare directly against the known
  threshold (150 peak-trigger / 120 mean-gate) is more diagnostically
  useful than a re-scaled unit that doesn't map cleanly onto Voxi's own
  gating logic.
- `voxi chunks show` already establishes this convention (`Mean / Peak
  RMS: %d / %d`) — the list column should match it rather than introduce
  a second unit/format for the same underlying stat.
- No new conversion code (dBFS log-scale math, normalization) is needed,
  keeping this a pure "surface an existing field" change.

## 3. Scope

- Add a `VOL` (or `RMS`) column to the `voxi chunks list` text-format
  table in `internal/chunks/command.go`, showing `c.MeanRMS` (mean RMS)
  for every listed chunk, accepted or rejected. Keep the header
  abbreviation terse to match the existing style (`INDEX`/`AUDIO`/`RTF`/
  `STATUS`).
- Widen the printf format string
  (`internal/chunks/command.go:45,63`) to add the new column, positioned
  before STATUS (RTF and volume are both acoustic/perf diagnostics, kept
  together ahead of the outcome columns).
- `--format json` output is unaffected — `Chunk.MeanRMS`/`PeakRMS` are
  already serialized JSON fields (`chunks.go:38-39`), so `voxi chunks list
  --format json` already exposes this; only the human-readable table needs
  the new column.
- **Also add a per-chunk speech-level sparkline** (amended 2026-09-06, at
  the user's request): a compact, fixed-width (~10 character) visual
  strip showing how volume/speech-energy varied *across the duration of
  the chunk*, using Unicode Braille block glyphs for sub-character
  vertical resolution — e.g. `⣠⣦⣀⣠⣀⣤⣀⣠⣦⣀` — rather than a flat single-value
  bar. This is a genuinely new visualization, not just surfacing an
  existing field:
  - `audio.RenderAudioLevelMeter` (`internal/audio/audio.go:271-287`)
    already exists but renders a single instantaneous level as a 10-char
    `■`/`·` bar, used live during recording (`internal/eager/eager.go:531-532`)
    — it has no time axis and is the wrong shape for "level over the
    chunk's duration." This ticket needs a *new* renderer, not a reuse of
    that one.
  - Requires splitting the chunk's PCM samples into N (e.g. 10, one per
    sparkline character) equal time buckets and computing a per-bucket
    RMS (or reusing whatever windowing `audio.AnalyzePCM`
    (`internal/audio/audio.go:329-367`) already does internally, if its
    windows are compatible/reusable), then mapping each bucket's level to
    one of the Braille glyphs' available height steps (Braille cells give
    a 2-wide-by-4-tall dot grid per character — enough steps for a
    reasonable "quiet → loud" gradient per bucket without needing a full
    terminal-graphics library).
  - Where this is computed matters: the chunk's raw PCM is available
    while the chunk is being finalized (`internal/eager/eager.go` around
    where `MeanRMS`/`PeakRMS` are already populated, :358-359/:421-422)
    but chunks only persist a `.wav` file (`Chunk.WAVFile`) plus
    aggregate stats afterward — no raw PCM is retained in the manifest.
    Two options, to be decided at implementation time: (a) compute the
    sparkline once at finalize time (while PCM is still in memory) and
    store it as a new `Chunk.VolumeSparkline string` JSON field
    (cheapest, keeps `list` fast, mirrors how `MeanRMS`/`PeakRMS` are
    already precomputed-and-stored rather than recomputed on read), or
    (b) recompute on demand by re-reading the chunk's `.wav` file at
    `list` time (avoids a new persisted field but re-decodes audio on
    every listing). Option (a) is recommended for consistency with the
    existing MeanRMS/PeakRMS pattern.
  - The sparkline should sit adjacent to the new mean-RMS number (e.g.
    `RMS` numeric column followed by a `LEVEL` sparkline column) so the
    user gets both an exact number and an at-a-glance shape — useful for
    telling a truncated/clipped utterance apart from a uniformly-quiet
    one, which a single mean number can't distinguish.
- Out of scope: changing `voxi chunks show`'s existing "Mean / Peak RMS"
  line (though adding the sparkline there too, if implemented, would be a
  natural small extension — not required for this ticket's acceptance),
  changing the acoustic gate's thresholds or behavior.

## 4. Acceptance Criteria

- `voxi chunks list` gains a volume/level column (mean RMS) for every
  listed chunk, including rejected ones, positioned in the table between
  RTF and STATUS.
- `voxi chunks list` also gains a fixed-width Braille speech-level
  sparkline column next to it, rendering a distinguishably different
  pattern for a chunk that was loud throughout vs. one that trails off to
  silence vs. one that was uniformly quiet (`rej:low_energy_transient`
  cases should visibly read as "flat and low").
- A test in `internal/chunks/command_test.go` asserts the new RMS column
  renders the expected value for both an accepted chunk and a rejected
  (`rej:low_energy_transient`) chunk, and that the header row contains
  the new column labels.
- A unit test for the sparkline renderer itself (wherever it lands, e.g.
  `internal/audio`) asserts that a synthetic loud-then-quiet PCM buffer
  and a synthetic uniformly-quiet buffer produce visibly different
  sparkline strings (exact glyph-by-glyph assertions, not just "non-empty").
- `voxi chunks list --format json` is unchanged for the RMS fields; if
  `Chunk.VolumeSparkline` is added as a new stored field (implementation
  option (a) above), it is included in JSON output like every other
  `Chunk` field.

## 5. Non-Goals

- Converting RMS to dBFS or a normalized percentage display.
- Reworking or replacing `audio.RenderAudioLevelMeter`'s live single-value
  bar — it serves a different purpose (real-time feedback during active
  recording) and stays as-is.
- Changing acoustic gating thresholds or `low_energy_transient` detection
  logic (issue 054's domain, already Implemented).

## 6. Background

Raised 2026-09-06 at the user's request after running `voxi chunks list`
and seeing several `rej:low_energy_transient` rows with no way to gauge
how quiet the recording actually was, or to compare rejected chunks
against accepted ones for volume. Investigation confirmed the underlying
`MeanRMS`/`PeakRMS` values are already computed and stored per chunk
(issue 053's ring buffer / issue 054's acoustic gate) — the RMS-column
part of this ticket is scoped purely to surfacing them in the `list`
table, not adding new instrumentation.

Amended same day: the user additionally asked for a "this is speech"-level
visual indicator — a ~10-character Braille sparkline (e.g.
`⣠⣦⣀⣠⣀⣤⣀⣠⣦⣀`) showing level over the chunk's duration, rather than a
single flat number. Unlike the RMS column, this *does* require new
computation (per-time-bucket RMS + Braille-glyph mapping), since no
existing code renders a level-over-time strip — the existing
`audio.RenderAudioLevelMeter` is a single-value live bar, not a
sparkline. See Section 3 for the two implementation options considered.
