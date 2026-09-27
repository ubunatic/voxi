# 155 — integrate cloned-voice TTS backend (Chatterbox via tts-serve)

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

## Update 2026-09-27

Chatterbox (MIT) via tts-serve on CPU, 10 s calm reference: all 7 demo texts rendered with no dropped words (checked with voxtype). User rated it good. About 10 s compute per 1 s audio on CPU; needs ~8 GB free RAM. Prefer this over Pocket TTS for the backend.

## Sprint Plan (2026-09-28, supersedes the Pocket TTS spec above)

Backend: Chatterbox served by tts-serve (local FastAPI; see issue 157 for its API: `GET /capabilities`,
POST text + reference audio -> WAV). voxi is an HTTP client only (Go `net/http`, no new deps); voxi does
not install or launch the Python server in this ticket. Piper stays the default engine.

### M1 — spec + HTTP client (no live server needed)
- `spec/tts.yaml`: new engine `tts-serve` (URL, timeout, reference WAV path, engine-specific settings);
  engine selection key; Go reads values from spec, no duplicates.
- `internal/tts`: client that posts text + reference WAV and returns WAV bytes into the existing playback
  path. Clear error when the server is unreachable (name the URL and the setting).
- Unit tests with `httptest.Server`: request shape, success, non-200, timeout, unreachable.
- Acceptance: `go test ./...` green; piper behaviour unchanged.

### M2 — voice profile + CLI wiring
- Reference WAV selection from the issue 153 allowlist, stored under `~/.local/share/voxi/voices/`
  (via `voxi voice prepare` or a small `voxi voice clone` subcommand).
- `voxi say --no-llm "text"` uses the cloned voice when engine = tts-serve.
- Long-latency awareness: ~10 s compute per 1 s audio on CPU; sentence-chunked requests so playback
  can start before the whole text is synthesized, and a sane default timeout.
- Update `docs/TTSReading.md` (setup, consent: own voice only, RAM ~8 GB, latency).
- Acceptance: unit tests; one live end-to-end check if a tts-serve server is running, else document skip.

### M1 delivered (09fbca9): spec + HTTP client
`tts_serve` spec block, opt-in `tts-serve` backend (auto never picks it), `internal/tts/ttsserve.go`
client with httptest coverage. Tests green.

### M2 delivered: voice profile + CLI wiring
Pre-work done: (1) fetched `github.com/scorbo2/tts-serve` source — Chatterbox's `SynthesisRequest`
(`impl/server_chatterbox.py`) is flat, `extra="forbid"`, no `engine` field, `audio_base64` required
(`min_length=1`); `internal/tts/ttsserve.go` now sends `text`/`audio_base64` plus `tts_serve.settings`
merged as flat top-level fields, with no `engine` key or `settings` envelope. (2) `reference_wav` is
now also settable at runtime via `tts_serve_reference_wav` in `~/.config/voxi/config.yaml` or
`VOXI_TTS_SERVE_REFERENCE_WAV`, written by the new `voxi voice clone` subcommand; the spec default
stays empty. (3) `tts_serve.timeout_ms` documented against `SplitText`'s existing sentence chunking,
with a test (`TestTTSServeEngineSendsOneRequestPerSentenceChunk`) confirming one HTTP request per chunk.
`voxi voice clone [--sample <id>] [--name <name>]` installs an allowlisted sample WAV (issue 153) to
`~/.local/share/voxi/voices/<name>.wav` and records it. `voxi say --no-llm` speaks with the cloned voice
once `tts_backend: tts-serve` is selected. `docs/TTSReading.md` updated. `go test ./...` green; no
tts-serve server was reachable at `http://127.0.0.1:8000` in this environment, so the live end-to-end
check was skipped.

### M2 Pre-Work / Required Refinements (from M1 review)
1. **Request shape unverified:** issue 157 says engine-specific parameters "vary by engine"; the client
   nests them under a `settings` object and sends an `engine` field. Check the tts-serve source
   (tts-engine-common request model, Chatterbox impl) and match it (likely flat top-level keys, no
   `engine` field). Record the source link in a code comment.
2. **reference_wav is not user-settable:** the spec is embedded, so an empty `reference_wav` can only
   change by rebuilding. The M2 voice profile path (`~/.local/share/voxi/voices/`) or user config must
   supply it at runtime; the spec keeps only the default.
3. **Timeout vs latency:** 60 s covers only ~6 s of audio on CPU. Sentence chunking (M2) must keep each
   request well under the timeout; add a test that long text becomes several requests.
