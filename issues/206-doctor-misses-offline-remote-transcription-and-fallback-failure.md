# 206 — Doctor misses offline remote transcription and fallback failure

**Status**: Closed — Verified offline fallback, unavailable endpoints, doctor repair and live dictation
**Priority**: P1 (High)
**Severity**: Major
**Category**: Bug
**Related**: [197](197-robust-asr-fallbacks-and-self-healing-for-offline-upstream-and-doctor-repair.md), [189](189-add-voxi-doctor-command-with-repair-wizard-for-audio-stack-faults.md)

---

## Problem

With the remote lmcoder server offline, the user reports that the microphone works but transcription is not repaired by `voxi doctor --fix`. After repair, doctor reports `OK mic` and `OK asr-engine: crispasr online (local engine)`, with 7 OK and no faults; the only warning concerns historical default-sink journal messages. Install output showed selected models `r2t2-confucius4` and, on a later run, `cohere-transcribe-03-2026`; the R2T2 service was disabled in both cases because the selected model did not use a loopback endpoint.

The actual selected model and runtime failure path at the time of doctor are not yet established. Investigate why the check reports a healthy local engine while the user's transcription setup remains broken; do not assume the offline server is the only cause. Issue 197 previously addressed offline upstream and fallback behavior.

## Goal and verification

/goal Make doctor accurately diagnose the active end-to-end transcription path and handle an offline remote server with a usable fallback or a clear actionable failure, and verify that dictation works after any reported repair. Stop and report if blocked by a user decision or denied permission; on a loop, hard architectural or OS blocker, or no progress, clear the goal and write a failure report explaining why.

- Recheck live configuration, current code and recent commits before starting; reproduce the reported mismatch and distinguish microphone, local engine, remote endpoint and fallback health.
- Cover remote-offline and fallback-unavailable cases with regression tests; verify the repair or honest diagnostic against the live setup where feasible. Do not report success solely because a local binary is present when the actual transcription path is unusable.

## Progress

- 2026-10-30: Committed 67ca553 (fix(asr): diagnose active transcription and safe failover).
- `make test-q1` passed in full after the first test run failed or hung; the fault was corrected and the rerun passed.
- Live doctor probe transcribed through the proxy with the remote server offline (x600). The probe does not show which backend served the request.
- Live QA: with x600 offline and proxy stopped, doctor reported WARN and direct local R2T2 transcribed live dictation. With proxy and local ASR stopped, doctor reported FAULT; `doctor --fix --yes` restarted the proxy and its recheck transcribed the probe clip. After both services were restored, the user dictated successfully; chunk 339 was accepted and typed. The proxy's actual backend choice is not independently identified.
- Local cohere/whisper transcription remains unverified and reports WARN.
- Lint: `gokay check --all` failed with existing findings. A new Setenv lint finding was fixed but not rechecked. `gokay assess` completed.
- Sprint process: the haiku developer's initial work needed Opus escalation; sol performed the independent review.
