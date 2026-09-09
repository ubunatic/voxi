# 098 — Unify chunks list and feedback sample list Behind a Shared Lister; Add --all/--full/--short to sample list

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Enhancement
**Category**: Dev Tooling / CLI UX
**Related**: [070 RMS/sparkline column](070-show-recorded-volume-rms-column-in-voxi-chunks-list.md),
[071 colorize sparkline](071-colorize-the-level-sparkline-in-voxi-chunks-list-color-flag-design-proposal.md),
[072 2x braille resolution](072-2x-braille-resolution-for-the-level-sparkline-in-voxi-chunks-list-packed-dual-column-glyphs.md),
[055 save chunk by index](055-save-chunk-by-index-feedback-command.md),
[097 external speech sample catalog](097-external-public-domain-speech-sample-catalog-download-on-demand-not-committed.md)
(forward-looking: a third sample corpus this unified lister should be designed with in mind, not a
hard requirement — 097 is unimplemented),
[internal/chunks/command.go](../internal/chunks/command.go) (`chunks list`/`show` implementation),
[internal/chunks/color.go](../internal/chunks/color.go) (`--color` sparkline colorizer),
[internal/chunks/chunks.go](../internal/chunks/chunks.go) (`Chunk` struct),
[internal/feedback/command.go](../internal/feedback/command.go) (`sample list` implementation,
~line 270-288),
[internal/devsample/sample.go](../internal/devsample/sample.go) (`Sample` struct, `SamplesDir`,
`PublicSamplesDir`)

---

## 1. Problem & Motivation

Verbatim user request:

> we need sth. like `voxi feedback sample list --all --full|--short` (default: short)
> and also make 'samples list' look more like 'chunks list' incl. timeline and more stats
> chunks and samples should use shared code and lister for this I would say

`voxi chunks list` and `voxi feedback sample list` both list recorded/stored audio items with
transcripts, but have grown independently and now look nothing alike.

## 2. Current State

### `voxi chunks list` (rich)

`internal/chunks/command.go`'s `listCmd` (RunE around line 31-91) prints a header-and-rows table:

```
INDEX  TIMESTAMP  AUDIO  RTF  RMS  LEVEL  STATUS  TRANSCRIPT
```

- `LEVEL` renders a fixed-width Braille sparkline (`Chunk.VolumeSparkline`, precomputed at finalize
  time in the eager pipeline via `internal/audio`), optionally ANSI-colorized by loudness via
  `--color auto|always|never` (`internal/chunks/color.go`, `colorizeSparkline`/`shouldUseColor`,
  `NO_COLOR` respected per no-color.org).
- Also supports `--reverse` and `--format text|json`.
- `Chunk` (`internal/chunks/chunks.go`, `Chunk` struct) carries far more fields than the table
  shows: `SessionID`, `ChunkID`, pipeline timestamps (`FinalizedAt`,
  `TranscriptionStartedAt/EndedAt`, `TypingStartedAt/EndedAt`), `AudioDurationSecs`, `PCMBytes`,
  `MeanRMS`, `PeakRMS`, `VoicedRatio`, `ProbableSilence`, `TranscribeDurationSec`,
  `TranscriptWordCount`, `RTF`, `RawTranscript`, `CleanedTranscript`, `Accepted`,
  `RejectionReason`, `TranscriptChars`, `TranscriptDigest`, `RepeatUnit`/`RepeatCount`.
- `chunks show [INDEX|last]` (`FormatChunkDetails`) prints the full per-chunk detail block;
  `chunks play` replays the audio.
- No `--full`/`--short` verbosity split exists on `chunks list` today — verbosity is instead
  handled by the separate `show` subcommand, plus `--format text|json`.

### `voxi feedback sample list` (thin)

`internal/feedback/command.go`, the `sample` subcommand's `list` entry (~line 270-288):

- Reads only the private manifest via `devsample.LoadManifest(home)`. The public/promoted corpus
  at `testdata/noise-samples/` (added via `voxi feedback sample promote`, issue 096) is invisible
  to this command entirely — there is no `--all` or any flag to point at another directory (the
  only related flag in this file is `promote --to DIR`).
- Output is one bare tab-separated line per sample: `name<TAB>timestamp<TAB>text-preview(60 chars)`
  (`Sample.Preview`). No header, no color, no stats, no sparkline/timeline.
- The underlying `Sample` struct (`internal/devsample/sample.go`) is much thinner than `Chunk`:
  `Name`, `WAVFile`, `Text`, `Keyterms`, `Timestamp`. No duration, no RMS/loudness, no sparkline —
  dev samples currently carry no acoustic metadata at all, unlike chunks (which get it from
  `internal/audio.AnalyzePCM`, wired in `internal/eager/eager.go`).

## 3. Proposal

Per the user's explicit direction ("chunks and samples should use shared code and lister for this
I would say"): factor the columnar-table/sparkline rendering currently living as
`chunks`-list-specific logic in `internal/chunks/command.go` and `internal/chunks/color.go` into
shared code both `voxi chunks list` and `voxi feedback sample list` can call, rather than growing
`sample list`'s table rendering as an independent reimplementation. Add a `--all` flag to
`sample list` to include the public/promoted corpus alongside the private one, and a
`--full`/`--short` verbosity pair (default `--short`) analogous to what `chunks show` vs.
`chunks list` currently split across two subcommands.

This is a design/proposal-shaped ticket — the following are open questions to resolve during
implementation, not a prescribed design:

- **Where should the shared lister/table-rendering code live?** A new internal package (e.g.
  `internal/listutil` or similar), extending `internal/chunks` and having `feedback` depend on it
  (already the case today — `internal/feedback` imports `internal/chunks` for `save-chunk`), or
  somewhere under `internal/deps`?
- ~~Does `Sample` need new stored acoustic-stats fields, or are stats computed on the fly at list
  time?~~ **Resolved by the user: on-the-fly.** No new stored fields on `Sample`; compute
  duration/RMS/sparkline etc. by reading the WAV/FLAC audio file per sample at list time, the same
  way `scripts/clack_features` already decodes FLAC via a shelled-out `ffmpeg` this session. That
  makes `ffmpeg` an accepted runtime dependency for this CLI path too (already true for
  `sample promote`, which shells out to `ffmpeg` to FLAC-encode).
- **New idea from the user, not in the original request**: a `--process` flag (or folded into
  `--full`) that actually runs each listed sample's audio through an ASR engine — explicitly
  floated as "run it against cohere" (`cohereTranscribeEngine`/`crispasr`, see
  `internal/eager/cohere.go`) — and shows the live transcription result alongside the stored
  ground-truth `Text`, rather than only ever showing the stored text. This turns `sample list
  --process` into a lightweight on-demand accuracy spot-check across the corpus (stored expected
  vs. fresh actual), distinct from the existing `scripts/speech_context_bench` batch-benchmarking
  tool. Open sub-questions: does this reuse `speech_context_bench`'s transcription invocation path,
  or `internal/eager`'s directly; does it run against every configured model/engine or just one
  (flag-selectable?); is a WER/diff shown per row, or just the two transcripts side by side; what
  happens to `--short` output width once a second transcript column is added.
- **Exact shape of `--all`/`--full`/`--short`.** What does `--short` vs. `--full` actually show
  (a compact table vs. a `chunks`-list-style rich table with sparkline/stats columns, or `--full`
  meaning full per-sample detail akin to `chunks show`)? Does `--all` mean "merge private +
  public/promoted corpora", or something else? Should this replace or complement `chunks list`'s
  existing `--format text|json` for consistency between the two commands?
- **Should `sample list` gain a `--to DIR` flag** (mirroring `promote --to`) to point at an
  arbitrary directory, instead of / in addition to a fixed `--all` that always merges the two known
  locations?
- **Forward-looking, not required now**: issue 097 (filed this session, unimplemented) proposes a
  third, external/downloaded sample corpus. If 097 ships, the unified lister should be able to
  accommodate a third source without a redesign — worth keeping in mind when choosing the shared
  abstraction's shape, but not a blocker for this ticket.

## 4. Acceptance Criteria (indicative, not exhaustive given open questions above)

- `voxi feedback sample list` shows a header-and-rows table comparable in spirit to `chunks list`
  (timestamp, some acoustic/stat indicator, transcript), not a bare tab-separated dump.
- `voxi feedback sample list --all` includes samples from both the private and public/promoted
  corpora, clearly distinguishing which source each row came from.
- `voxi feedback sample list --full` / `--short` (short is default) controls verbosity as
  described above.
- Acoustic/timeline stats shown in the table are computed on the fly from the sample's audio file
  at list time (WAV or FLAC), not stored on `Sample`.
- `chunks list` and `sample list` share the table-rendering implementation rather than each
  maintaining independent formatting code — the specific extraction is left to the implementer per
  the open questions in §3.
- `--process` (or a `--full`-gated variant) runs each listed sample through an ASR engine
  (starting point: cohere-transcribe) and surfaces the fresh transcript next to the stored
  ground-truth text — exact scope (which engine(s), diff/WER display) left to the open questions
  in §3.
