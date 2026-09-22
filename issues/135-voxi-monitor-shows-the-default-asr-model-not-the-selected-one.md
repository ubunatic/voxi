# 135 — voxi monitor shows the default ASR model, not the selected one

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Major
**Category**: Bug
**Related**: 134 (R2T2 engine, where this surfaced)

---

## 1. Problem

`voxi monitor` reports `cohere-transcribe-03-2026` even when the user has
selected another ASR model in `voxi settings` and the daemon is demonstrably
running that other model.

Reproduced 2026-09-22 (verified):
- `~/.config/voxi/config.yaml` has `asr_model: r2t2-confucius4`, and
  `~/.config/voxi/env` has `VOXI_ASR_MODEL=r2t2-confucius4`.
- The live daemon runs `voxi eager --daemon --model r2t2-confucius4`
  (from `ps`; `internal/agent/eager_backend.go:55-56` passes it from user
  settings).
- `voxi monitor` still displays `cohere-transcribe-03-2026`.

**Cause**: `detectActiveModel` (`internal/monitor/collector.go:277-303`) never
reads voxi's own user settings. It reads *voxtype's* legacy
`~/.config/voxtype/config.toml`, and only when `voxtype.service` is active;
otherwise it returns `spec.DefaultModel`. So the displayed model is the spec
default for every non-whisper engine, whatever the user chose.

This misleads exactly when it matters most: while evaluating or switching
engines, the monitor claims the old model is in use.

## 2. /goal

`voxi monitor` shows the ASR model the agent is actually running. Resolution
order matches how the daemon itself decides (`internal/agent/eager_backend.go`):
voxi user settings (`config.ASRModel`) first, then the existing legacy
voxtype fallback, then the spec default. A test covers each branch, including
the regression: settings say a non-default model, voxtype.service is inactive,
and the monitor must not report the spec default.

## 3. Notes

- Re-verify against current code before starting; this ticket may lag it.
- Keep the voxtype legacy path working — it is the correct answer when
  voxtype.service really is the active backend.
- Consider showing the engine too (`whisper` / `cohere-transcribe` /
  `openai-transcribe`), since the model name alone no longer implies the
  runtime. Optional, not required by the /goal.
