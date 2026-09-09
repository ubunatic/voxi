# 096 — Spectral-centroid keyboard-clack vs. speech classification (research)

**Status**: Research In Progress — first-keyboard result promising, more keyboards pending
**Priority**: P2 (Medium)
**Category**: ASR Quality / Acoustic Gating
**Related**: [internal/audio/audio.go](../internal/audio/audio.go) (`CheckCandidateAcoustics`), [093](093-collapse-an-immediately-repeated-trailing-sentence-clause-in-eager-transcripts.md)/[094](094-collapserepeatedtrailingclause-wrongly-deletes-a-legitimate-short-answer-that-matches-the-question-s-last-word.md) (adjacent hallucination-filtering work), `scripts/clack_features` (new analysis tool), `~/.config/voxi/samples` (private dev-sample corpus, not in git)

---

## 1. Problem

Loud mechanical-keyboard clacks near the mic sometimes pass the existing acoustic gate
(`CheckCandidateAcoustics`'s RMS/voiced-frame checks) and reach the transcription engine (whisper
or cohere-transcribe), where they get hallucinated into phantom text (commonly "Thank you.") —
caught today only post-hoc by stop-word rejection, after wasting a transcription call. Plain
volume/RMS thresholding cannot separate clacks from real short speech: the user's keyboard is loud
enough that clack RMS (448-1346) and voiced ratio (0.19-0.28) overlap the same range as genuine
one-word speech utterances (RMS 261-573, voiced ratio 0.17-0.26) — confirmed empirically, see §3.

## 2. Method

Recorded two dev-sample sets via `voxi feedback sample save-chunk` (private, local-only, in
`~/.config/voxi/samples`, never committed):

- **5 keyboard-clack noise chunks** (`keyboard-clack-1477/1478/1479/1481/1482`): real recorded
  keyboard clacks that the eager pipeline accepted as plausible speech and Whisper hallucinated as
  "Thank you." Ground truth marked `[keyboard noise]` (see §4 for why this isn't a clean `""`).
- **8 short real-speech counter-examples** (`short-yes`, `short-no-no-yes`, `short-abc`,
  `short-one-two`, `short-three`, `short-uh`, `short-nah`, `short-eh`): deliberately short/quiet
  utterances in the same acoustic neighborhood as the clacks, recorded specifically to stress-test
  any classifier threshold against false-rejection of legitimate short answers (same failure class
  as issue 094).

Wrote `scripts/clack_features` (new, `go run ./scripts/clack_features`), a standalone offline
analysis tool (no dependency on the private `devsample` package) that:

- Reads the corpus.tsv-compatible manifest and each sample's WAV.
- Frames audio into 25ms windows (400 samples @ 16kHz, 50% overlap), skipping near-silent frames
  (RMS < 150, mirroring the existing `low_energy_transient` gate).
- Computes, per frame: zero-crossing rate (ZCR) and spectral centroid (Hamming-windowed 512-point
  radix-2 FFT, magnitude-weighted mean frequency, DC bin excluded).
- Aggregates to one mean ZCR and one energy-weighted mean spectral centroid per sample.

## 3. Findings (first keyboard)

```
NAME                       ZCR    CENTROID  FRAMES
short-uh                0.0833    1165.7Hz      16
short-nah                0.1616    1707.4Hz      21
short-one-two            0.1704    1596.7Hz      39
short-no-no-yes          0.2021    1775.8Hz      48
short-eh                 0.2168    1869.2Hz      14
short-three               0.2637    2080.3Hz      19
short-abc                 0.3189    2488.5Hz      50
keyboard-clack-1478       0.3421    3038.2Hz      84
keyboard-clack-1477       0.3452    3043.9Hz     145
keyboard-clack-1479       0.3485    3032.4Hz      63
keyboard-clack-1481       0.3693    3222.3Hz      28
keyboard-clack-1482       0.3980    3195.4Hz      21
short-yes                 0.4426    2664.8Hz      20
```

- **ZCR alone does not separate the classes**: `short-yes` (0.4426) exceeds every clack sample's
  ZCR. Not usable as a standalone feature.
- **Spectral centroid cleanly separates the classes** on this keyboard: every speech sample is
  ≤2664.8Hz, every clack sample is ≥3032.4Hz — a ~370Hz gap, zero overlap across all 13 samples. A
  threshold around 2850Hz would classify all 13 correctly.

## 4. Known limitation in the sample corpus

`voxi feedback sample save-chunk`'s interactive prompt has no way to save a literal empty ground
truth: a blank Enter falls back to the (wrong) raw ASR guess when one exists, and there's no
`--text` flag to force `""`. The 5 clack samples were saved with the marker text `[keyboard noise]`
instead. Note `scripts/speech_context_bench`'s WER tokenizer strips `[`/`]` as plain separators, so
if this corpus is ever pointed at that bench, `[keyboard noise]` would score as the literal words
"keyboard noise", not be recognized as a non-speech/negative fixture. Not an issue for this
ticket's use (feature-value inspection only), but blocks reusing this corpus for bench-style
accuracy scoring without either a real non-speech convention or a `--text` override flag.

## 5. Caveats / what's not yet validated

- **n=13, one keyboard, one room.** The ~370Hz gap is a promising first result, not a proven
  threshold — mechanical vs. membrane vs. laptop-chiclet keyboards, different mic
  distance/room acoustics, and a wider variety of speech (louder, sung tones, other languages)
  could shrink or close the gap.
- Not yet validated against real dictation sessions/telemetry, only the curated 13-sample set.
- Spectral centroid was computed energy-weighted across all non-silent frames of the whole
  chunk; a per-frame (not per-chunk) decision might behave differently for chunks that mix a
  clack onset with trailing real speech in the same segment.

## 6. Next steps

- Record dev-sample sets on 2-3 more physical keyboards (in progress — user recording more) to
  test whether the ~2850Hz threshold (or spectral centroid as a feature at all) holds up across
  keyboards, or whether it's specific to this one's spectral signature.
- If it holds: wire spectral centroid into `CheckCandidateAcoustics` as an additional rejection
  reason (e.g. `high_spectral_centroid`), gated so it only fires on chunks already borderline on
  the existing RMS/voiced-ratio checks — not as a blanket replacement for them.
- If it doesn't hold across keyboards: consider spectral flatness or per-frame (not per-chunk)
  classification as fallback features before considering a trained classifier (ruled out for now
  as overkill for a 13-sample, one-edge-case problem — see chat discussion 2026-09-09).
