# 126 — Use lmcoder OpenAI-compatible transcription API as ASR backend target

**Status**: In Progress — openai-transcribe engine implemented against voxi-clients whisper-server canary; lmcoder endpoint still pending
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Architecture
**Related**: [[106-configure-llm-transcription-cleanup-service-defaults-for-lmcoder-integration]], lmcoder:issues/091, lmcoder:issues/071, docs/LmcoderOperations.md (lmcoder)

---

## 1. Problem & Motivation

`voxi` currently relies on its own local/embedded ASR pipeline for speech-to-text, and
separately talks to `lmcoder`'s OpenAI-compatible chat/completions endpoint purely for
*post-transcription* LLM cleanup ([[106]]).

Sister project `lmcoder` has filed `lmcoder:issues/091` to expose an OpenAI-compatible
`/v1/audio/transcriptions` endpoint (matching the standard multipart-upload,
`model`/`language`/`response_format` contract). Once that lands, `voxi` should be able
to add it as an alternative ASR backend target: a self-hosted, OpenAI-API-compatible
transcription service running on the user's own machines, selectable alongside (or
instead of) the current ASR path, keeping the whole voice pipeline — transcription and
cleanup — under local control rather than an external vendor.

This ticket tracks the `voxi`-side integration only. It is blocked on `lmcoder:issues/091`
landing first.

## 2. Scope

- Add an ASR backend option in `voxi` that speaks the OpenAI `/v1/audio/transcriptions`
  contract against a configurable `base_url` (mirroring the existing `OPENAI_BASE_URL`
  pattern already used for LLM cleanup in [[106]]).
- Decide how this backend is selected: config/env toggle, CLI flag, or auto-probe of a
  local lmcoder instance at startup.
- Define fallback behavior if the endpoint is unreachable/slow — reuse the graceful
  degradation approach from [[106]] (short timeout, fall back to the existing local ASR
  path) rather than inventing a new failure mode.
- Out of scope: implementing the endpoint itself (tracked in `lmcoder:issues/091`);
  changes to the LLM cleanup step ([[106]], [[112]], [[121]], [[122]], [[123]]) beyond
  wiring the new transcription source into the existing pipeline.

## 3. Acceptance Criteria

- `voxi` can transcribe live audio via a local `lmcoder` `/v1/audio/transcriptions`
  endpoint end-to-end, verified with a real recording (not just a mocked client).
- Config/env surface documented, consistent with the `OPENAI_BASE_URL`/`VOXI_CLEANUP_MODEL`
  conventions from [[106]].
- Fallback to existing ASR path verified when the lmcoder endpoint is offline.

## 4. Verification Guidance

- Blocked on `lmcoder:issues/091` reaching a usable state; canary against it manually
  before wiring full integration.
- Live-verify via `voxi monitor`/an actual dictation session, not unit tests alone.

## 5. Progress (2026-09-16)

Implemented the generic client side (this ticket's actual scope) without waiting on
`lmcoder:issues/091`, since a second server already speaks the identical
`/v1/audio/transcriptions` contract: `voxi-clients/agy-voice/whisper-server` (see that
repo's issue 001), a research server bridging agy's internal voice RPC behind the
OpenAI API shape.

- New `openai-transcribe` engine value in `spec/models.yaml`/`spec/models.go`
  (`internal/eager/openai_transcribe.go`): multipart POST to
  `{base_url}/audio/transcriptions` with `response_format=text`, HTTP-only (no local
  binary — `requireEngineBinary` returns early for this engine).
- Config: `OPENAI_ASR_BASE_URL` / `UserSettings.OpenAIASRBaseURL`, default
  `http://127.0.0.1:8090/v1` (matches `whisper-server`'s default `--addr`), following the
  `OPENAI_BASE_URL` naming from [[106]].
- New spec field `does_llm_cleanup` (per-model, `spec/models.go`/schema): the
  `openai-transcribe-local` model entry sets it `true` since `whisper-server`'s
  Gemini-backed output already reflects server-side cleanup — `internal/eager` now skips
  its own `cleanWithLLM` pass for any model with this flag set, avoiding a redundant
  double cleanup. Re-check this default once `lmcoder:issues/091` lands: if lmcoder's
  endpoint returns raw ASR instead, a lmcoder-targeted model entry should set it `false`.
- Live-verified: `go test ./internal/eager/... ./internal/config/... ./spec/...` green,
  plus a direct canary call against the running local `whisper-server`
  (`transcribeOpenAIWAV` against `test/fixtures/short-abc.wav`) returned a real transcript
  end-to-end. `voxi-agent.service` restarted via `make restart-service` to pick up the
  `internal/eager` change.
- New spec field `api_model` (`spec/models.go`/schema, required for `engine:
  openai-transcribe`): the request's `model` field must be an id the remote server
  understands, not voxi's own spec map key — the model entry was accordingly renamed
  `openai-transcribe-local` -> `openai-transcribe-gemini` (`api_model: gemini`), naming
  what it actually targets (agy's Gemini voice RPC behind `whisper-server`, not literal
  whisper.cpp). `internal/chunks` `voxi chunks list` badges: 🔷 for this
  Gemini-via-openai-transcribe model specifically, 🌐 for any other openai-transcribe
  model, distinct from 👂 (whisper) and ⚡ (cohere-transcribe).
- Remaining for full close-out: swap/add a model entry once `lmcoder:issues/091` exposes
  its endpoint, confirm whether lmcoder's output needs `does_llm_cleanup: false`, and the
  explicit unreachable-endpoint fallback-to-existing-ASR-path behavior from the
  acceptance criteria (today an unreachable `openai-transcribe` endpoint surfaces as a
  transcription error for that chunk, same as any other engine failure, rather than
  falling back to a different engine mid-session).

### Unrelated pre-existing bug surfaced during live verification

Selecting `openai-transcribe-local` via `voxi settings` produced zero requests against
`whisper-server` in production, tracing back to `internal/agent/eager_backend.go`:
`voxi-agent.service` runs `voxi agent --daemon`, whose `EagerChildBackend.Start` spawned
the actual transcription child as `voxi eager --daemon` with no `--model` flag at all —
`UserSettings.ASRModel` was saved to `config.yaml`/`env` and shown in the settings TUI,
but never read by anything that starts the daemon, so it silently always ran
`spec.DefaultModel` (`cohere-transcribe-03-2026`) regardless of the user's selection.
Fixed in the same session: `eager_backend.go` now loads `UserSettings` and passes
`--model <ASRModel>` through to the child. Live-verified via `ps aux` showing the child
process launched with `--model openai-transcribe-local` after `make restart-service`.
