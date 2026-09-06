# 070: Show Recorded Volume (RMS) Column in `voxi chunks list`

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
- Out of scope: changing `voxi chunks show`'s existing "Mean / Peak RMS"
  line, changing the acoustic gate's thresholds or behavior, adding a
  visual level meter (`audio.RenderAudioLevelMeter` already exists and is
  used live during recording in `internal/eager/eager.go:531-532`, but a
  bar-graph column is a separate, optional follow-up, not required here).

## 4. Acceptance Criteria

- `voxi chunks list` gains a volume/level column (mean RMS) for every
  listed chunk, including rejected ones, positioned in the table between
  RTF and STATUS.
- A test in `internal/chunks/command_test.go` asserts the new column
  renders the expected RMS value for both an accepted chunk and a
  rejected (`rej:low_energy_transient`) chunk, and that the header row
  contains the new column label.
- `voxi chunks list --format json` is unchanged (still emits the full
  `Chunk` struct, `MeanRMS`/`PeakRMS` included).

## 5. Non-Goals

- Converting RMS to dBFS or a normalized percentage display.
- Adding a visual ASCII level-meter bar to the list table (could reuse
  `audio.RenderAudioLevelMeter` in a future ticket if requested).
- Changing acoustic gating thresholds or `low_energy_transient` detection
  logic (issue 054's domain, already Implemented).

## 6. Background

Raised 2026-09-06 at the user's request after running `voxi chunks list`
and seeing several `rej:low_energy_transient` rows with no way to gauge
how quiet the recording actually was, or to compare rejected chunks
against accepted ones for volume. Investigation confirmed the underlying
`MeanRMS`/`PeakRMS` values are already computed and stored per chunk
(issue 053's ring buffer / issue 054's acoustic gate) — this ticket is
scoped purely to surfacing them in the `list` table, not adding new
instrumentation.
