# 096 — Spectral-centroid keyboard-clack vs. speech classification (research)

**Status**: Research In Progress — spectral centroid alone falsified on a 3rd keyboard (flat/chiclet
keys); needs a second feature or different approach, see §3b
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

## 3a. Findings (second keyboard: Logitech MX Keys)

Recorded 5 more clack chunks (`keyboard-clack-logi-1503/1504/1505/1506/1507`) on a Logitech MX
Keys (first keyboard was mechanical). Re-ran `scripts/clack_features` over the combined 18-sample
corpus:

```
NAME                          ZCR    CENTROID  FRAMES
short-uh                   0.0833    1165.7Hz      16
short-nah                  0.1616    1707.4Hz      21
short-one-two              0.1704    1596.7Hz      39
short-no-no-yes            0.2021    1775.8Hz      48
short-eh                   0.2168    1869.2Hz      14
short-three                0.2637    2080.3Hz      19
short-abc                  0.3189    2488.5Hz      50
keyboard-clack-1478        0.3421    3038.2Hz      84
keyboard-clack-1477        0.3452    3043.9Hz     145
keyboard-clack-logi-1506   0.3477    3291.0Hz       7
keyboard-clack-1479        0.3485    3032.4Hz      63
keyboard-clack-logi-1507   0.3525    2976.8Hz       3
keyboard-clack-logi-1503   0.3538    3198.4Hz     246
keyboard-clack-1481        0.3693    3222.3Hz      28
keyboard-clack-logi-1505   0.3738    3296.5Hz     151
keyboard-clack-1482        0.3980    3195.4Hz      21
keyboard-clack-logi-1504   0.4150    3495.7Hz      32
short-yes                  0.4426    2664.8Hz      20
```

The threshold holds: still zero overlap across both keyboards, but the gap narrows to
~312Hz (2664.8Hz `short-yes` to 2976.8Hz `keyboard-clack-logi-1507`, down from ~370Hz with one
keyboard). Note `keyboard-clack-logi-1507` used only 3 non-silent frames — a very short/quiet
clack — so that centroid estimate is noisier than the others; more Logi samples would firm it up.
A ~2850Hz threshold still classifies all 18 samples correctly, but the shrinking margin as more
keyboards are added is exactly the risk flagged in §5 — worth tracking whether it keeps narrowing
or stabilizes.

## 3b. Findings (third keyboard: flat/chiclet-key variant) — threshold falsified

Recorded 3 more clack chunks (`keyboard-clack-flat-1508/1509/1510`) on a flat/chiclet-key
keyboard. Re-ran `scripts/clack_features` over the combined 21-sample corpus:

```
NAME                          ZCR    CENTROID  FRAMES
keyboard-clack-flat-1508   0.0739    1894.3Hz       2
short-uh                   0.0833    1165.7Hz      16
short-nah                  0.1616    1707.4Hz      21
short-one-two              0.1704    1596.7Hz      39
short-no-no-yes            0.2021    1775.8Hz      48
short-eh                   0.2168    1869.2Hz      14
keyboard-clack-flat-1509   0.2509    2446.7Hz     276
keyboard-clack-flat-1510   0.2549    2446.7Hz     226
short-three                0.2637    2080.3Hz      19
short-abc                  0.3189    2488.5Hz      50
keyboard-clack-1478        0.3421    3038.2Hz      84
keyboard-clack-1477        0.3452    3043.9Hz     145
keyboard-clack-logi-1506   0.3477    3291.0Hz       7
keyboard-clack-1479        0.3485    3032.4Hz      63
keyboard-clack-logi-1507   0.3525    2976.8Hz       3
keyboard-clack-logi-1503   0.3538    3198.4Hz     246
keyboard-clack-1481        0.3693    3222.3Hz      28
keyboard-clack-logi-1505   0.3738    3296.5Hz     151
keyboard-clack-1482        0.3980    3195.4Hz      21
keyboard-clack-logi-1504   0.4150    3495.7Hz      32
short-yes                  0.4426    2664.8Hz      20
```

**Spectral centroid alone no longer separates the classes.** `keyboard-clack-flat-1509` and
`-1510` (2446.7Hz each, 276 and 226 frames — well-sampled, not noise) sit *below*
`short-abc` (2488.5Hz) and `short-yes` (2664.8Hz). The flat/chiclet-key keyboard's clacks are
quieter and spectrally lower than the mechanical/Logi ones, landing squarely in the same range as
real short speech. (`keyboard-clack-flat-1508` at 2 frames is too sparse to draw any conclusion
from — essentially silent, likely already caught by the existing `low_energy_transient`/
`unvoiced_transient` gate before a centroid check would ever run, per its own rejection reason.)

This falsifies §3/§3a's working hypothesis that spectral centroid alone is sufficient. §5/§6
updated accordingly — a single-feature threshold on centroid is not a viable general solution;
either a second feature is needed alongside it, or a different approach entirely.

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

- **n=21, three keyboards, one room, one speaker.** Confirmed by §3b: the threshold that held for
  2 keyboards (mechanical, Logitech MX Keys) broke on a 3rd (flat/chiclet keys). Different mic
  distance/room acoustics and a wider variety of speech (louder, sung tones, other languages)
  are still entirely untested and could shrink margins further even within the keyboards already
  covered.
- Not yet validated against real dictation sessions/telemetry, only the curated sample set.
- Spectral centroid was computed energy-weighted across all non-silent frames of the whole
  chunk; a per-frame (not per-chunk) decision might behave differently for chunks that mix a
  clack onset with trailing real speech in the same segment.
- ZCR doesn't rescue this either: the flat-keyboard clacks' ZCR (0.2509/0.2549) also falls inside
  the speech cluster's range (`short-three` 0.2637, `short-abc` 0.3189), so neither feature alone,
  nor an obvious combination of the two, currently separates all 21 samples.

## 6. Next steps

- **Spectral-centroid-alone is falsified as of §3b — do not wire it into `CheckCandidateAcoustics`
  as currently scoped.** A quiet flat-keyboard clack and a real short speech utterance can share
  the same centroid range, so a threshold here would trade false-accepted clacks for
  false-rejected speech (the exact failure class issue 094 already burned us on).
- Investigate features that target the *transient shape* rather than the steady-state spectrum,
  since that's the more fundamental acoustic difference between a percussive clack and voiced
  speech regardless of keyboard: attack sharpness/rise-time, spectral flux (frame-to-frame
  spectral change, high at a clack's onset), or onset-to-decay energy ratio. These need a
  per-frame or per-onset analysis, not the current whole-chunk energy-weighted average.
- Record 1-2 more keyboards (especially another flat/chiclet or laptop-style one, to see if
  keyboard-clack-flat's low centroid is that whole *class* of keyboard or an outlier) before
  drawing conclusions about which feature(s) might work.
- Given single-feature thresholds keep breaking as the sample set grows, revisit whether a
  trained classifier is still overkill (previously ruled out for a 13-sample, one-edge-case
  problem — see chat discussion 2026-09-09) now that it's a 21-sample, three-keyboard problem with
  two falsified single-feature hypotheses. Still likely premature at n=21, but the bar for
  "hand-crafted features are good enough" is looking higher than initially assumed.
