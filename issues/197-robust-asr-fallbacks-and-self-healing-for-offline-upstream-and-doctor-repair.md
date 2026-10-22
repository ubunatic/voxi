# 197 — Robust ASR fallbacks and self-healing for offline upstream and doctor repair

**Status**: Open
**Priority**: P1 (High)
**Severity**: High
**Category**: Bug
**Related**: lmcoder:issues/196, voxi:issues/181, voxi:issues/189, `spec/models.yaml`, `internal/doctor`

---

## 1. Problem & Motivation
When the upstream LLM/ASR host (x600) is offline or unreachable, `lmcoder-proxy` fails to establish an SSH tunnel and crashes on t14. Consequently, `http://127.0.0.1:8735/v1` (the ASR endpoint for `r2t2-confucius4`) becomes unreachable.
Meanwhile:
1. `voxi doctor` only checks audio hardware/PipeWire services and reports 7 OK, completely oblivious that ASR speech-to-text is dead and voice input cannot work.
2. `voxi doctor --fix` has no check or repair mechanism to detect ASR failure, switch to available local fallbacks (such as `r2t2-confucius4-voxi` on port 18131 or direct local asr on port 8749), or restore default models once the primary backend recovers.
3. Voxi's eager transcription failure logs connection refused and gives up without fallback.

## 2. Technical Specification / Findings
- Voxi supports multiple ASR models in `spec/models.yaml`:
  - `r2t2-confucius4`: base_url `http://127.0.0.1:8735/v1` (lmcoder proxy)
  - `r2t2-confucius4-voxi`: base_url `http://127.0.0.1:18131/v1` (`voxi-r2t2.service`, llama-server)
  - `cohere-transcribe-03-2026`: local `crispasr` binary
- When `r2t2-confucius4` fails because proxy/x600 is down:
  - If lmcoder local asr is running on 8749 or can be reached directly, or if `voxi-r2t2.service` (18131) can be run, or falling back to local `crispasr`, ASR can continue working.
- `voxi doctor` needs an ASR engine check (or `voxi doctor` should check ASR health and provide repair/workaround actions).
- When doctor or background watchdog activates a fallback, it should record that a temporary fallback is active, probe the canonical endpoint periodically or during checks, and revert when normal service is restored.

## 3. Implementation & Verification Plan
1. Add ASR service diagnostics to `voxi doctor` and `voxi settings test`.
2. Provide automated repair/workaround in `voxi doctor --fix` when ASR backend is unreachable:
   - Detect if local lmcoder asr (8749) is running and accessible or if `voxi-r2t2.service` can be started, or switch temporarily to `r2t2-confucius4-voxi` or `cohere-transcribe-03-2026`.
3. Provide self-healing / fallback in eager transcription if primary backend fails.
4. Verify end-to-end with live tests.
