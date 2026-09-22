# 139 — Ship a systemd user unit for the R2T2 llama-server backend

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Major
**Category**: Feature
**Related**: 134 (engine + the capped command), 136 (monitor reports it offline), `@docs/ASREngines.md`, `@docs/InstallationArchitecture.md`

---

## 1. Problem

`openai-transcribe` models need an HTTP server that voxi deliberately does not
start (134 §6). With `r2t2-confucius4` selected as the live ASR model, the
server currently only runs because someone launched it by hand: **a reboot
silently breaks dictation** until it is started again. Observed live on
2026-09-22: 7 consecutive chunks lost to `connection refused` before the cause
was visible.

136 makes the failure visible in `voxi monitor`, but visible is not fixed.

## 2. /goal

A user can have the R2T2 backend start and restart automatically, without voxi
itself supervising a child process. Concretely: a systemd **user** unit carrying
the capped command, installed through voxi's existing installation path
(`@docs/InstallationArchitecture.md`), off by default and opt-in. Once enabled,
dictation works after a reboot with no manual step, and `voxi monitor` shows the
backend online.

## 3. Notes / Uncertainties

- **The memory cap is a safety invariant, not a tuning knob.** The unit must
  carry `-c 4096` (134 §5): an uncapped `llama-server` on this AMD Cezanne iGPU
  inflated to 26.5 GiB VRAM / 99% RAM and hung the machine. Consider a
  `MemoryMax=` on the unit as a second belt.
- Model paths currently live in `~/.cache/voxi/models/`; decide whether the unit
  hardcodes them or reads the spec.
- Only worth installing when an `openai-transcribe` model with a local
  `base_url` is actually selected — do not start a 3 GiB server for users on
  whisper or Cohere.
- Out of scope: voxi supervising the process itself. That was rejected in 134
  §6 because it puts the safety cap in a second place where it can rot.
