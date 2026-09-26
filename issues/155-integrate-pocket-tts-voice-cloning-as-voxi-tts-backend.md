# 155 — integrate Pocket TTS voice cloning as voxi TTS backend

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Feature
**Category**: Feature
**Related**: issue 154 (canary, must pass first), issue 153, `spec/tts.yaml`, `internal/tts/`

---

## 1. Problem & Motivation
After the canary (154) works, `voxi say` and TTS reading should be able to speak in the user's cloned voice.

## 2. Technical Specification / Findings
- New TTS engine `pocket` next to `piper` in `spec/tts.yaml` (spec is the source of truth; no duplicated values in Go).
- Voice profile = reference WAV (from the issue 153 allowlist) + engine/model version, stored under
  `~/.local/share/voxi/voices/`. Created by `voxi voice prepare` (or a new `voxi voice clone`).
- Run the sherpa-onnx binary as a subprocess, same pattern as piper; no PyTorch.
- Consent: the voice profile is private and local; document that only the user's own voice is intended.

## 3. Implementation & Verification Plan
- Engine selection via spec/config; `voxi say --no-llm "text"` speaks with the cloned voice.
- Unit tests for profile creation and command construction; one live end-to-end check.
- Update `docs/TTSReading.md`.
