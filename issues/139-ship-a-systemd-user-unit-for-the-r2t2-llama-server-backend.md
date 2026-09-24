# 139 — Ship a systemd user unit for the R2T2 llama-server backend

**Status**: Closed — M2 implemented and live-verified
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

## 4. Direction (2026-09-24)

User: **"`voxi install` should take care of all units."** `voxi install`
(`internal/install/install.go`) is the one place that writes, enables and
starts every voxi-related user unit, including the R2T2 server. Units come
from `systemd/*.service` through `voxi.ServiceAsset`, as the existing ones do.

Live findings at filing time:
- `voxi monitor` shows ASR OFFLINE: `asr_model: r2t2-confucius4` expects
  `127.0.0.1:18131` (spec/models.yaml `base_url`), and nothing listens there.
- There is no `llama-server` on `PATH`. The only build is lmcoder's vendored
  `~/.local/share/lmcoder/llama.cpp/b10590/llama-server`, and 134 refers to
  `~/.cache/voxi/llama.cpp`. The unit must resolve the binary deliberately
  (install-time lookup or a config setting) and fail clearly if it is missing.
  Never download or build llama.cpp as a side effect.
- Models are in `~/.cache/voxi/models/` (R2T2 gguf + mmproj present).

## 5. Milestones

- **M1 (R2T2 unit + install):** add `systemd/voxi-r2t2.service` with the
  capped command (`-c 4096`, `--port 18131`) plus `MemoryMax=` and
  `Restart=on-failure`. `voxi install` always writes it. It runs
  `enable --now` only when the selected `asr_model` is an `openai-transcribe`
  model whose `base_url` is loopback. Otherwise it disables and stops the unit
  and prints why. Tests use the existing fake `Effects`.
- **M2 (all units, one source of truth):** `voxi install` covers every unit
  voxi relies on (agent, eager, dotoold, r2t2; modifierd stays behind
  `--modifierd`). Make targets delegate to `voxi install` rather than
  duplicating unit logic. Update `@docs/InstallationArchitecture.md` and
  `@docs/ASREngines.md`. After `make install && voxi install`, confirm on this
  machine that the port answers and `voxi monitor` shows online.

### M1 delivered (5e0cb08): R2T2 unit + install
Live: `make install` with `llama_server_path` set to lmcoder's b10590 build.
Unit enabled and active, `:18131/health` returns ok, VRAM 8.4 GB total (includes
the 32k-context cleanup LLM), RAM 13 GB available.

### M2 Pre-Work / Required Refinements
- The activation rule is too broad: any future loopback `openai-transcribe`
  model would start the R2T2 server. Tie activation to the unit's own backend
  (the selected model's `base_url` host:port equals the unit's `127.0.0.1:18131`,
  with the port taken from one place, not duplicated) and add a test for a
  different loopback port.
- When the backend is active, check that the model and mmproj files exist and
  fail with a clear message, rather than letting the unit crash-loop.
- Unresolved path fallback `/usr/bin/llama-server`: fine, but print a line
  saying so.
