# 159 — voxi install --tts-serve: set up and start the Chatterbox tts-serve server

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Feature
**Category**: Feature
**Related**: issue 155 (tts-serve client backend), issue 157 (tts-serve research), `spec/tts.yaml`, `docs/TTSReading.md`

---

/goal `voxi install --tts-serve` leaves a running, restart-safe Chatterbox tts-serve server that the
`tts-serve` backend reaches at `tts_serve.url`, verified by a live `voxi say --no-llm` in the cloned
voice; stop and report when blocked on a user decision (e.g. multi-GB download, low RAM) or a denied
permission.

## 1. Problem & Motivation
Issue 155 made voxi a client of tts-serve but left installing and starting the server to the user, and
`docs/TTSReading.md` does not say how. On 2026-09-28 the server could not be started: the checkout at
`~/.cache/voxi/tts-serve` and the Chatterbox weights in `~/.cache/huggingface` still existed, but the
Python venv from the 2026-09-27 test was gone.

## 2. Technical Specification / Findings
- Upstream steps (`impl/server_chatterbox.md`): venv; `pip install` pinned
  `git+https://github.com/resemble-ai/chatterbox.git@5de7a54...` (PyPI 0.1.7 lacks
  `MULTILINGUAL_T3_MODELS`); `pip install ./tts-engine-common fastapi uvicorn loguru soundfile`;
  run `python impl/server_chatterbox.py`.
- Env: `CHATTERBOX_HOST`, `CHATTERBOX_PORT` (default 7500, but voxi spec URL uses 8000 — derive the
  port from `tts_serve.url`, don't duplicate it), `CHATTERBOX_DEVICE=cpu`.
- Pulls PyTorch (several GB). Needs ~8 GB free RAM; ask before the download, warn on low RAM.
- Likely shape: a new flag on the existing `voxi install` (like `--modifierd`), a systemd user unit
  (e.g. `voxi-tts-serve.service`) so the server survives reboots, and an uninstall path.
- Open: pin the tts-serve commit too? Start on boot, or on demand from the agent?

## 3. Implementation & Verification Plan
- Before starting, re-check current `voxi install` code and issue 155 state.
- Unit tests for command/unit-file construction; live: fresh install, `curl <url>/capabilities`,
  `voxi say --no-llm` in the cloned voice.
- Document setup in `docs/TTSReading.md`.
