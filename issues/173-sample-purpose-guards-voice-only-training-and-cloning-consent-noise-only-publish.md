# 173 — Sample purpose guards: voice-only training and cloning, consent, noise-only publish

**Status**: Open — filed from 169
**Priority**: P1 (High)
**Severity**: Major
**Category**: Safety
**Related**: 169, docs/SampleStore.md §3 invariants, depends on 170, 172

---

## 1. Problem & Motivation

Development samples must never be used for voice cloning or training by accident, and only the
user's own voice may be cloned. Today one allowlist file enforces this. Decision in docs/SampleStore.md §3.

## 2. Technical Specification

- `voice prepare|train|clone` read only `voice/`; no flag can widen it. `voice-training.txt` is ignored
  after migration (warning if present).
- Adding to or moving into `voice/` requires the confirmation "This is my own voice and I consent to
  cloning it" (`y`, or `--own-voice`); consent timestamp stored in the sidecar; refuse without it.
- `sample publish` refuses non-`noise` samples and requires an explicit confirmation that no
  intelligible speech is contained.
- `config.yaml` records `tts_voice_sample` (id) next to `tts_voice_reference_wav`; `voxi install`
  VoxCPM step names the source sample.

## 3. Implementation & Verification Plan

Tests: training on a dictation sample is impossible, missing consent refused, publish of voice/dictation
refused. `make restart-service` not needed (on-demand commands); `make install`.
