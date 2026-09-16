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
