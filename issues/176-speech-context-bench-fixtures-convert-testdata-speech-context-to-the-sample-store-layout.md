# 176 — Speech-context bench fixtures: convert testdata/speech-context to the sample store layout

**Status**: Open — found during 175
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Bug
**Related**: 171 (removed the TSV reader), 032 (bench), docs/SampleStore.md §2, scripts/speech_context_bench

---

## 1. Problem & Motivation

Since 171, `scripts/speech_context_bench -corpus DIR` opens DIR as a sample store (purpose folders
with JSON sidecars). `testdata/speech-context/` still holds a legacy `corpus.tsv` plus locally
recorded, git-ignored WAVs, so `-corpus testdata/speech-context` finds no samples. Benchmarks on
the private store (the default) still work.

## 2. Technical Specification / Findings

- Convert `testdata/speech-context/corpus.tsv` into `testdata/speech-context/dictation/<id>.json`
  sidecars (tracked); WAVs stay git-ignored and are recorded locally next to them.
- Decide how the bench reports a sidecar whose WAV was not recorded yet (skip with a note, not fail).
- The one local file there, `artifact-keyboard-smash.wav`, duplicates a private noise sample.

## 3. Implementation & Verification Plan

`go run ./scripts/speech_context_bench -corpus testdata/speech-context -list-only` lists the
fixtures; README in that directory updated.
