# 164 — Voxi settings menu: TTS backend switch

**Status**: Closed — TTS Backend row added to voxi settings menu and dump; tests pass, installed
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Feature
**Related**: docs/TTSReading.md, internal/config/config.go (`TTSBackend`), internal/tts/process.go (`NewEngine`), issue 163

---

## 1. Problem & Motivation

Switching between Piper and VoxCPM (and Festival, espeak-ng, auto) requires hand-editing
`~/.config/voxi/config.yaml` (`tts_backend`) and restarting `voxi-agent.service`.
The interactive `voxi settings` menu covers cleaner, models, keystroke delay, history and
modifier gating, but has no TTS entry.

/goal Let the user pick the TTS backend from `voxi settings` (persisted as `tts_backend`), verified by tests and a live switch, or stop and report when blocked on a user decision or denied permission.

## 2. Technical Specification / Findings

- Persist via the existing `UserSettings.TTSBackend`; validate with `config.ValidateTTSBackend`.
- Menu choices: `auto`, `piper`, `voxcpm`, `festival`, `espeak-ng`.
- Open questions (record, do not guess): also expose `tts_voxcpm_host` / preset? Should the menu offer restarting `voxi-agent.service`, since the daemon reads the setting only at startup? Show a note when `VOXI_TTS_BACKEND` in env overrides the saved value.
- `voxi settings --dump` and `--json` should include the new field.

## 3. Implementation & Verification Plan

Re-verify against live code and recent commits before starting. Add unit tests for the menu entry and persistence; run `make restart-service` and confirm `voxi tts` uses the chosen backend.
