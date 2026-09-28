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

## Sprint Plan (2026-09-28)

### M1 — `--tts-serve` flag, setup steps and systemd unit (no live download)
- `voxi install --tts-serve`: checkout/venv under `~/.cache/voxi/tts-serve`, pinned installs, a
  `voxi-tts-serve.service` user unit with `CHATTERBOX_HOST/PORT` derived from `tts_serve.url` and
  `CHATTERBOX_DEVICE=cpu`; confirmation prompt before the multi-GB install; low-RAM warning.
  Pins (tts-serve commit, chatterbox commit) live in spec, not Go.
- Decide the open questions in the ticket (default: pin tts-serve; start on boot via the unit) and
  record the decision here.
- Unit tests with injected command runner / file system: command sequence, unit-file content,
  port derivation, prompt declined -> nothing installed. No network in tests.
- Acceptance: `go test ./...` green; `make install`.

### M2 — live install and end-to-end check (needs the user's go for the download)
- Run `voxi install --tts-serve` for real, `curl <url>/capabilities`, `voxi voice clone`,
  `voxi say --no-llm` in the cloned voice. Document setup in `docs/TTSReading.md`.

### M1 delivered (1ed5180): `--tts-serve` flag, pins, unit
Decisions: pin both repos in `spec/tts.yaml` `tts_serve_install` (tts-serve `6ca92b2`, master HEAD
2026-09-28, no upstream tags; chatterbox `5de7a54`); start at login via `voxi-tts-serve.service`
(`WantedBy=graphical-session.target`). Re-runs reuse checkout and venv. Tests green, no downloads run.

### M2 Pre-Work / Required Refinements (from M1 review)
1. **CPU-only PyTorch:** plain `pip install` on Linux pulls CUDA torch plus nvidia wheels (several GB
   extra, useless on this AMD CPU setup). Install torch/torchaudio from the CPU index
   (`--index-url https://download.pytorch.org/whl/cpu`) first, matching chatterbox's pinned versions.
   Test the pip command sequence.
2. **`MemoryMax=8G` may OOM-kill the server** while loading the model, and `Restart=on-failure` then
   loops. Measure peak RSS during the live run and set the limit above it (or drop it); record the number.
3. Before the live run, check `~/.cache/voxi/tts-serve` for local changes (`git status`); the pinned
   checkout must not overwrite them.
