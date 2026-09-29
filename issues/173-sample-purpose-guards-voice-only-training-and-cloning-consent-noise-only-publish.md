# 173 — Sample purpose guards: voice-only training and cloning, consent, noise-only publish

**Status**: Closed — resolved
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

## Delivery (2026-09-29)

- Store: `Put` and `Add` refuse `voice` samples without consent; `GrantConsent`, `RequireConsent`.
- `voxi sample add|record --purpose voice` and `move ID voice` ask "This is my own voice and I consent
  to cloning it? [y/N]" or take `--own-voice`; consent time goes into the sidecar. Moving a sample
  without transcript into voice is refused.
- `voxi sample publish ID [--no-speech] [--public-store DIR]`: noise only, asks for the no-speech
  confirmation, FLAC-encodes via ffmpeg into `testdata/samples/noise/` (0644). `sample list --public`.
- `voice prepare|train|clone` read only `voice/` (already true since 171) and now refuse voice samples
  without consent; they print a note when the legacy `voice-training.txt` still exists.
- `voice clone` writes `tts_voice_sample`; `voxi install --voxcpm` names that sample in its step line.
- Tests: `internal/sample/guards_test.go`, `internal/tts/clone/guards_test.go`. Review (Terra): green
  after fixing an `Add` consent bypass.
