# 096 — Spectral-centroid keyboard-clack vs. speech classification (research)

**Status**: Research In Progress — spectral centroid and ZCR are falsified; sustained motor/mouse noise broadens the problem, so the current direction is harmonicity/pitch-salience and transient-shape analysis (see §3e, §6)
**Priority**: P2 (Medium)
**Category**: ASR Quality / Acoustic Gating
**Related**: [internal/audio/audio.go](../internal/audio/audio.go) (`CheckCandidateAcoustics`), [093](093-collapse-an-immediately-repeated-trailing-sentence-clause-in-eager-transcripts.md)/[094](094-collapserepeatedtrailingclause-wrongly-deletes-a-legitimate-short-answer-that-matches-the-question-s-last-word.md) (adjacent hallucination-filtering work), `scripts/clack_features` (analysis tool, reads both corpora below), `~/.config/voxi/samples` (private dev-sample corpus, real speech, not in git), [testdata/noise-samples](../testdata/noise-samples/) (public, git-lfs-tracked, FLAC-encoded — all clack/mouse/bg-noise samples below were promoted here via `voxi feedback sample promote`, since none contain real speech)

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

## 3c. Findings (fourth keyboard: quieter Cherry-switch board) — confirms full interleaving

Recorded 3 more clack chunks (`keyboard-clack-cherry-1511/1512/1513`, a quieter Cherry-switch
board). Re-ran `scripts/clack_features` over the combined 24-sample corpus (relevant excerpt,
sorted by centroid):

```
NAME                          ZCR    CENTROID  FRAMES
short-eh                   0.2168    1869.2Hz      14
keyboard-clack-cherry-1513 0.2405    2857.1Hz     162
keyboard-clack-flat-1509   0.2509    2446.7Hz     276
keyboard-clack-cherry-1512 0.2525    2854.0Hz     408
keyboard-clack-flat-1510   0.2549    2446.7Hz     226
short-three                0.2637    2080.3Hz      19
keyboard-clack-cherry-1511 0.2837    2862.2Hz     358
short-abc                  0.3189    2488.5Hz      50
keyboard-clack-1478        0.3421    3038.2Hz      84
...
short-yes                  0.4426    2664.8Hz      20
```

Both features now fully interleave rather than cluster: Cherry clacks (well-sampled, 162-408
frames) sit at ~2854-2862Hz — *above* `short-yes` (2664.8Hz) — while the flat-keyboard clacks sit
at 2446.7Hz — *below* `short-abc` (2488.5Hz). Sorted by centroid, clack and speech samples now
alternate rather than form two separable groups; no single threshold anywhere in the observed
range classifies all 24 samples correctly. ZCR shows the same overlap (Cherry clacks 0.24-0.28
sit directly on top of `short-eh`/`short-three`/`short-abc`).

This is a stronger, unambiguous confirmation of §3b's falsification, not just a repeat of it: with
2 keyboards the gap only narrowed; with 4 keyboards there is no ordering of the samples by either
feature that separates the two classes at all. §6's redirection toward transient-shape features
stands, and is now the primary hypothesis rather than one option among several.

## 3d. Findings (non-keyboard noise: mouse clicks/movement/lift-off on a wooden desk)

Recorded 2 more noise chunks (`mouse-noise-1514/1515`) — mouse clicks, movement, and lifting a
Lenovo vertical mouse and setting it back down on a wooden desk, not keyboard-related at all.
Re-ran `scripts/clack_features` over the combined 26-sample corpus (relevant excerpt):

```
NAME                          ZCR    CENTROID  FRAMES
short-eh                   0.2168    1869.2Hz      14
mouse-noise-1515           0.2391    2231.6Hz     302
mouse-noise-1514           0.2397    2373.9Hz     362
keyboard-clack-cherry-1513 0.2405    2857.1Hz     162
keyboard-clack-flat-1509   0.2509    2446.7Hz     276
...
short-three                0.2637    2080.3Hz      19
...
short-abc                  0.3189    2488.5Hz      50
```

Both mouse-noise samples (well-sampled, 302/362 frames) land squarely between `short-eh` and the
flat-keyboard clacks — indistinguishable from speech by centroid or ZCR, same as every keyboard
tested so far. This reframes the problem: it isn't "detect keyboard clacks specifically," it's
"distinguish any percussive/transient non-speech noise from short speech," and spectral-content
features (centroid, ZCR) don't do that regardless of the noise source. Strengthens the case for
§6's pivot to transient-shape features, since those target the mechanism common to all of these
noise sources (a sharp mechanical impact) rather than any one device's spectral signature.

## 3e. Findings (sustained non-percussive noise: outdoor gardening-vehicle motor + kitchen noise)

Recorded 2 more chunks (`bgnoise-motor-kitchen-1517/1518`) of a loud outdoor gardening-vehicle
motor plus kitchen noise, both audible together. Unlike every sample so far, this is *sustained*
ambient noise, not a percussive transient (no keyboard/mouse/click involved at all). Relevant
excerpt from the 28-sample corpus:

```
NAME                          ZCR    CENTROID  FRAMES
short-uh                   0.0833    1165.7Hz      16
short-nah                  0.1616    1707.4Hz      21
short-one-two              0.1704    1596.7Hz      39
bgnoise-motor-kitchen-1517 0.1993    2001.8Hz     581
short-no-no-yes            0.2021    1775.8Hz      48
bgnoise-motor-kitchen-1518 0.2100    2098.4Hz     311
short-eh                   0.2168    1869.2Hz      14
mouse-noise-1515           0.2391    2231.6Hz     302
mouse-noise-1514           0.2397    2373.9Hz     362
short-three                0.2637    2080.3Hz      19
short-abc                  0.3189    2488.5Hz      50
short-yes                  0.4426    2664.8Hz      20
```

Both bg-noise samples (well-sampled: 581/311 frames) land directly inside the speech cluster,
between `short-no-no-yes` and `short-eh`. **This changes the working theory from §3d**: since
this noise has no percussive attack at all (a continuous motor hum + kitchen ambience, not a
click), its overlap with speech means the problem isn't specifically about transient *shape*
either — a sustained non-speech sound can land in exactly the same centroid/ZCR range as a
sustained speech sound. Attack-sharpness/spectral-flux (§6, aimed at percussive onsets) would not
be expected to help distinguish *this* kind of noise from speech, even if it turns out to help
with clacks/clicks specifically.

The property most of these non-speech sounds still lack, that voiced speech has, is **harmonicity
(a periodic fundamental frequency / pitch)**: keyboard clacks, mouse clicks, and motor/kitchen
noise are all acoustically closer to broadband or quasi-periodic-but-inharmonic noise, whereas
voiced speech (vowels in particular) has a clear pitch period. A harmonic-to-noise ratio or
autocorrelation-based pitch-salience feature is now a stronger candidate than transient shape for
covering the full range of noise types tested so far — added to §6.

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
- Investigate transient-shape features (attack sharpness/rise-time, spectral flux, onset-to-decay
  energy ratio) for *percussive* noise sources specifically (keyboard clacks, mouse
  clicks/lift-off) — these need a per-frame/per-onset analysis, not the current whole-chunk
  energy-weighted average. Per §3e, this hypothesis should NOT be expected to also cover sustained
  non-percussive noise (motor/kitchen ambience) — that needs a different feature, see next.
- **New primary candidate per §3e: harmonicity / pitch salience** (harmonic-to-noise ratio, or an
  autocorrelation-based pitch-detection confidence score). Unlike centroid/ZCR/transient-shape,
  this targets a property that should hold across *all* noise types tested so far (percussive and
  sustained alike): voiced speech has a periodic fundamental, keyboard/mouse/motor/kitchen noise
  does not. This is the most promising untested direction as of 2026-09-09.
- Record 1-2 more keyboards (especially another flat/chiclet or laptop-style one, to see if
  keyboard-clack-flat's low centroid is that whole *class* of keyboard or an outlier), and more
  non-keyboard/non-percussive noise sources (fans, traffic, other ambient hums) to stress-test the
  harmonicity hypothesis the same way centroid/ZCR were stress-tested.
- Given single-feature thresholds keep breaking as the sample set grows, revisit whether a
  trained classifier is still overkill (previously ruled out for a 13-sample, one-edge-case
  problem — see chat discussion 2026-09-09) now that it's a 28-sample, multi-source noise problem
  with three falsified single-feature hypotheses (centroid, ZCR, and — pending confirmation —
  transient shape's inapplicability to sustained noise). Still worth trying harmonicity first,
  but the bar for "hand-crafted features are good enough" keeps rising.
