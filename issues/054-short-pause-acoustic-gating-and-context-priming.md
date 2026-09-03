# 054: Short-Pause Hallucination Gating: Acoustic Validation and Context Priming Analysis

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Performance
**Related**: [046 speech-context default-on](046-speech-context-default-on.md), [049 leading hallucination subs byuk](049-leading-hallucination-subs-byuk.md), [053 ring buffer chunks](053-ring-buffer-recent-audio-chunks-and-transcripts.md), [internal/audio/audio.go](../internal/audio/audio.go), [internal/eager/eager.go](../internal/eager/eager.go)

---

## 1. Problem & Motivation

During natural speech, a speaker frequently makes brief 0.5s–1.2s conversational pauses between clauses or sentences. In the recorded chunk ring buffer (e.g. Chunk #34, duration 1.12s), we observed:
1. Two genuine sentences were dictated before and after (chunks #32 and #33, then #35).
2. During the 1.12s pause, a minor non-speech acoustic event occurred (a transient breath, lip movement, or desk noise of ~300ms duration).
3. The VAD segmenter triggered on this brief spike (`ThresholdRMS: 150`, `MinSpeechMs: 200`), slicing out a 1.12s chunk containing mostly ambient silence and a transient click.
4. Whisper transcribed this 1.12s non-speech snippet into the hallucinated phrase:
   ```text
   "Switch Boss."
   ```
   Other observed short-pause hallucinations include: `"-Trap."`, `"-D, etc."`, `"YAML.com."`, `"and all."`.

Because this text did not match whole-string stop words, it passed validation and was typed into the active window.

Crucially, there are two distinct technical hypotheses for why this occurs:
1. **The Context / Prompt Priming Hypothesis**:
   Is Whisper being primed into generating hallucinations by previous context? Specifically:
   - Does `--initial-prompt` (enabled by default in Issue 046) bias Whisper's autoregressive decoder so heavily that given near-silent or ambiguous audio, it predicts vocabulary tokens instead of emitting silence?
   - Does this hallucination also happen when there is **no** initial prompt / no context?
   - In upstream `whisper.cpp`, if context tokens (`carry_over_context` or previous utterance text) are fed to subsequent audio segments, Whisper is notorious for "hallucination looping" or trying to continue an imaginary thought during pauses.
2. **The Acoustic Energy / Voicing Gating Hypothesis**:
   Real human speech contains sustained voiced vowels and formant structures lasting multiple consecutive frames (>250–350ms of continuous harmonic energy).
   Non-speech transients (inhalations, lip smacks, keyboard clicks, throat clearing) have brief impulse spikes or high spectral tilt, but very low total acoustic energy.
   Currently, Voxi's VAD triggers if any frame touches `ThresholdRMS >= 150` for `MinSpeechMs: 200`, which is easily satisfied by a sharp breath or chair squeak.

---

## 2. Research & Investigation Goals

### 2.1 Context Priming vs. Isolated Silence Investigation
- Using saved chunk audio fixtures (e.g. `chunk_0034.wav` which produced `"Switch Boss."`, `kt-sentences-plus-silence`, and `bug-d-etc`):
  1. Test `voxtype transcribe` **with** the default `--initial-prompt` vs. **without** any initial prompt.
  2. Does removing the speech context prompt prevent `"Switch Boss."` and `"-Trap."`?
  3. If speech context increases hallucination propensity on near-silence, how can we keep vocabulary biasing for genuine speech without inducing decoder hallucinations on silence?
  4. Verify whether `voxtype` carries over any internal decoder context across invocations (currently `internal/eager` spawns a fresh subprocess per utterance, so each process only sees `--initial-prompt`, not previous utterance transcripts).

### 2.2 Pre-ASR Acoustic Gating (VAD Hardening)
- **Minimum Voiced Duration**: Increase `MinSpeechMs` or require that frames above `ThresholdRMS` be sustained for a continuous duration (e.g. at least 300ms of voiced frames, not merely 100–200ms of transient noise).
- **Energy / Volume Ratio**: Calculate the average RMS across the entire candidate segment:
  - If a segment has peak RMS > 1000 but the overall segment average RMS is < 200, it is an impulsive transient surrounded by silence, not an utterance.
  - Drop or suppress segments whose active voiced ratio is below a threshold (e.g., < 35% of frames contain speech energy).

### 2.3 Post-ASR Plausibility Gating (Speech-Rate / Duration Sanity Check)
- A human cannot pronounce "Switch Boss" (2 words, 3 syllables) or "So we either extend the classification system" (14 syllables) in 0.8s or with < 300ms of voiced energy.
- Add a heuristic filter:
  - If audio duration is $< 1.2\text{s}$ and the transcript contains multiple words or > 2 syllables without corresponding acoustic energy, flag as a probable hallucination and reject/suppress.

---

## 3. Implementation Plan

1. **Benchmark Canary on Saved Chunks**:
   - Run a test script against `/run/user/<uid>/voxi/chunks/chunk_0034.wav` and other saved short chunks with:
     - No initial prompt vs. full speech-context initial prompt.
     - Whisper decoding options: `--no-speech-threshold` (e.g. 0.6), `--entropy-threshold`, `--logprob-threshold`.
2. **Audio Segmenter Enhancement (`internal/audio`)**:
   - Add minimum sustained voiced frames validation in `ProcessFrame` to prevent transient breaths/clicks from emitting a segment.
   - Add an energy density check (`TotalSpeechRMS / Duration`).
3. **Eager Pipeline Integration (`internal/eager`)**:
   - Discard sub-threshold or unvoiced transient audio slices before spawning `voxtype`.
   - Log rejected transient slices in the ring buffer with `rej:low_energy_transient` so they can be inspected in `voxi chunks list`.

---

## 4. Verification

- Re-play `chunk_0034.wav` and test whether it is rejected pre-ASR or correctly silenced.
- Verify that genuine short utterances (e.g. "Yes.", "No.", "Stop.", "Go ahead.") still transcribe reliably.

---

## 5. Intermediate Review Feedback (2026-09-03)

The pending implementation compiles and the full Go test suite passes, but Issue
054 is not ready to close. The current worktree also mixes this work with the
Issue 053/055 chunk ring-buffer and save-chunk implementation; keep the eventual
Issue 054 change reviewable and independently verifiable.

### Findings

1. The acoustic defaults are too permissive for the reported failure mode.
   `MinVoicedFrames: 3` and `MinVoicedRunFrames: 2` represent only 60ms total
   and 40ms consecutive energy at 20ms per frame. A roughly 300ms transient can
   therefore pass the gate. Calibrate these values against saved noise and
   genuine short-utterance fixtures instead of treating the current defaults as
   sufficient.
2. The energy-density implementation is unfinished. `audio.AnalyzePCM` computes
   mean RMS and voiced ratio, but it has no caller, and neither metric influences
   segment acceptance. Implement and calibrate the planned mean-energy and/or
   voiced-ratio gate, or remove the unused API if canary evidence rejects that
   approach.
3. Acoustically rejected audio is currently lost. `AudioSegmenter.ProcessFrame`
   returns `nil` for a rejected candidate, so `internal/eager` cannot store its
   audio or emit the required `rej:low_energy_transient` ring-buffer metadata.
   The segmenter/eager boundary needs to return a rejected candidate plus a
   structured reason, or provide an equivalent diagnostic path without invoking
   ASR.
4. Tests do not exercise the new rejection behavior. Add focused cases for an
   isolated spike, separated spikes, the minimum accepted sustained run,
   `Flush`, the max-window path, and genuine short speech. Also test the acoustic
   statistics if they remain part of the design.
5. The context-priming investigation is still outstanding. Record controlled
   with-prompt versus without-prompt results for the same audio. The named
   `chunk_0034.wav`, `kt-sentences-plus-silence`, and `bug-d-etc` fixtures were
   not present in `testdata/speech-context` during review; only
   `artifact-keyboard-smash.wav` was available.
6. Post-ASR duration/speech-rate plausibility gating has not been implemented or
   explicitly ruled out based on evidence.
7. `git diff --check` reports trailing whitespace in the newly added
   `artifact-keyboard-smash` corpus row; clean this up before committing.

### Pickup Checklist

- Run the prompt/no-prompt and decoder-threshold canaries on identical saved
  chunks and document commands, model, output, and conclusions here.
- Choose acoustic thresholds from fixture evidence, including false-negative
  checks for "Yes", "No", "Stop", and "Go ahead".
- Carry structured acoustic rejection diagnostics into the chunk ring buffer
  without spawning `voxtype`.
- Add regression tests for all acceptance and rejection boundaries.
- Run `gofmt`, `git diff --check`, and `go test ./...`.
- Because this changes the live eager daemon path, finish by running
  `make restart-service` rather than only `make install`.
