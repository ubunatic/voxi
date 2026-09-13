# 106 — Configure LLM Transcription Cleanup Service Defaults for `lmcoder` Integration

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Architecture
**Related**: `systemd/voxi-agent.service`, `systemd/voxi-eager.service`, `lmcoder:issues/071`, `lmcoder:docs/LmcoderOperations.md`

---

## 1. Problem & Motivation

When `voxi` runs as a `systemd --user` background service (e.g. `voxi-agent.service` or `voxi-eager.service`), it executes in an isolated environment that does not inherit shell export variables from `.bashrc` or `.zshrc`.

To enable seamless, zero-config post-processing of speech transcripts using the local `lmcoder` service running on `http://127.0.0.1:8734`:
1. `voxi` user services must carry explicit environment defaults or read a standardized configuration file (`~/.config/voxi/env` or `~/.config/voxi/config.yaml`).
2. The service should default to `OPENAI_BASE_URL=http://127.0.0.1:8734/v1` and `VOXI_CLEANUP_MODEL=qwen3-4b-instruct-2507-q4` (or `smollm3-3b-instruct-q4`).
3. If `lmcoder` is offline or unloading, `voxi` must gracefully fall back to raw ASR output without crashing or blocking audio transcription pipelines.

---

## 2. Technical Specification

### 2.1 Systemd Service Environment
In `systemd/voxi-agent.service` and `systemd/voxi-eager.service`:
- Add `EnvironmentFile=-%h/.config/voxi/env` so users can customize endpoints per machine.
- Include sensible fallback defaults:
  ```ini
  EnvironmentFile=-%h/.config/voxi/env
  Environment=OPENAI_BASE_URL=http://127.0.0.1:8734/v1
  Environment=OPENAI_API_KEY=none
  Environment=VOXI_CLEANUP_MODEL=qwen3-4b-instruct-2507-q4
  ```

### 2.2 Graceful Degradation & Timeout Handling
- Cleanup requests must use a short HTTP timeout (e.g. `1.5s`).
- If the endpoint returns a connection error, 503, or times out, `voxi` logs a warning and yields the raw ASR transcript directly.

---

## 3. Implementation Plan

1. Update unit files in `systemd/` with `EnvironmentFile` and default `OPENAI_BASE_URL`.
2. Ensure `voxi` config loader reads `~/.config/voxi/env` or CLI flags when launched by systemd.
3. Add a fallback unit test verifying that transcript pipeline functions even when the LLM endpoint is unreachable.
4. Verify with `systemctl --user daemon-reload` and live transcription check.

