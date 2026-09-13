# 115 — Prevent modifier buffer flush drops from expired stopDrainTimeout

**Status**: Open
**Priority**: P1 (High)
**Severity**: High
**Category**: Core / Eager Engine
**Related**: [Issue 101](101-eager-modifier-release-race-guard.md), [Issue 057](057-eager-session-overlap-and-reaping.md), [Issue 083](083-eager-typing-reliability-and-chunk-retention.md), [internal/eager/eager.go](../internal/eager/eager.go)

---

## 1. Problem Description & Incident Forensics

When dictating a multi-chunk sentence and stopping recording (e.g. by pressing Super or Super+X while a chunk is transcribing or waiting in the queue), the final transcribed text can be completely lost instead of being typed into the focused window.

### Incident Evidence (Session `20260913T183735.957964611Z-000017`)
- **Utterance**: The user dictated:
  1. Chunk #563 (8.0s): *"I mean, we shouldn't broaden this yet, but we can at least test the systemd service installation there and the modifier deinstallation."*
  2. Chunk #564 (1.7s): *"deinstallation."*
- **Action**: User pressed Super / Super+X to stop recording at `20:37:46.397`.
- **Modifier gate**: Pressing Super activated the `modifierBuffer` race guard (Issue 101), queuing the transcribed deliveries for delayed flush upon stop.
- **Transcription**:
  - Chunk #563 completed transcription at `20:37:48.532` (took 3.34s) and was added to `eagerBuf`.
  - Chunk #564 completed transcription at `20:37:51.185` (took 1.14s) and was added to `eagerBuf`.
- **Drop**:
  - Capture stopped at `20:37:46.399`, initializing `drain.stopAt(20:37:46.397)` with `stopDrainTimeout = 5 * time.Second` (hard deadline: `20:37:51.397`).
  - The worker finished transcription and entered `eagerBuf.FlushDeliveries()` at `20:37:51.402` (5 milliseconds past the deadline).
  - `drain.eligible()` evaluated to `false`.
  - Both chunks were discarded with telemetry event `delivery_stale`, and never typed into the focused window.

---

## 2. Root Cause Analysis

In `internal/eager/eager.go`, after the transcription worker goroutine drains (`transWg.Wait()`), the session flushes any buffered output held during modifier gating:

```go
if pending, wasBuffering := eagerBuf.FlushDeliveries(); wasBuffering {
    for _, item := range pending {
        if !drain.eligible() {
            recordStaleDelivery(recorder, sessionID, item.ID, 0)
            continue
        }
        claimed, claimErr := delivery.Claim(item.ID)
        ...
        if !drain.eligible() {
            recordStaleDelivery(recorder, sessionID, item.ID, 0)
            continue
        }
        typeErr := typing.TypeTextObserved(...)
    }
}
```

### Why this is a bug:
1. **Audio was already verified valid**: Chunks held in `eagerBuf` were already verified eligible when captured (`job.Eligible`), evaluated as acoustically plausible speech (`job.Plausible`), transcribed successfully, and buffered specifically *for this session*.
2. **We only wait for genuine speech**: Non-speech slices (clicks, silence, transients with `!job.Plausible`) are filtered out immediately in milliseconds without running Whisper/CrispASR or holding up the pipeline. Only chunks with verified speech acoustics are queued and transcribed.
3. **Session is already sequentially drained**: Reaching line 940 happens strictly after `transWg.Wait()`. No new or stray background chunks from this session can arrive later.
4. **`stopDrainTimeout` (5s) is too tight for queue backlog + LLM cleanup**: When multiple valid chunks queue up or when ASR / LLM post-processing takes >5s total across the trailing chunks, the hard 5-second wall clock deadline from the initial stop request expires, causing `drain.eligible()` to silently drop valid, completed speech at the final delivery stage.

---

## 3. Required Fix & Architecture Refinements (Advisor Review)

Following advisor review (Codex / Astra), the fix requires addressing context ownership and delivery coordination beyond simply deleting the `drain.eligible()` lines:

1. **Decouple Context Ownership for Delivery**:
   - `drain.stopAt()` schedules cancellation of `drain.ctx`. Currently, `typing.TypeTextObserved(drain.ctx, ...)` inherits this context.
   - If `drain.ctx` is canceled after 5s, `TypeTextObserved` will fail with `context.Canceled` even if the `if !drain.eligible()` checks are removed!
   - **Fix**: The typing/injection call on the post-drain flush path must not use the canceled capture drain context. It should use an independent delivery context (e.g. `context.Background()` or the root daemon context `ctx`) bounded by an injection-specific timeout (e.g. `5s`).

2. **Session Interleaving & Delivery Ordering**:
   - While `eagerBuf` is session-local, rapid `Stop` -> `Start` sequences can result in the old session draining and typing into the desktop while a new session has already begun.
   - Ensure the delivery coordinator maintains strict chronological sequencing and respects active window focus boundaries.

3. **Bounded Per-Stage Timeouts & Explicit Error Taxonomy**:
   - **ASR Transcription**: Bounded per job via `transcribeTimeout` (30s) so an engine crash or infinite loop is killed and reaped immediately.
   - **LLM Cleanup**: Bounded via HTTP request context (`1500ms`). When it fails, distinguish between `timeout`, `connection_error`, `invalid_schema`, and `empty_response`, falling back to the base clean ASR text and logging telemetry (`llm_cleanup_fallback`).
   - **Injection**: Bounded per delivery batch.

4. **Update Chunk Metadata on Buffered Flush**:
   - Ensure `chunkMeta.TypingStartedAt` and `chunkMeta.TypingEndedAt` are updated and persisted to `manifest.json` / sidecars when delivered via `FlushDeliveries()`.

5. **Acoustic Speech Gate Verification**:
   - Confirm unvoiced/low-energy transients (`!job.Plausible`) continue to short-circuit without blocking transcription or notifications.

---

## 4. Lifecycle Regression Test Matrix

1. **Multi-Chunk Buffered Drain Beyond Stop Timeout**:
   - Verify that 2+ chunks taking >5.0s to transcribe in `modifierBuffer` complete and are injected in order, exactly once, without being canceled by `drain.ctx`.
2. **Context Cancellation Isolation**:
   - Verify that `drain.cancel()` does not abort in-flight or post-drain `TypeTextObserved` calls.
3. **Rapid Stop -> Start Sequencing**:
   - Test rapid Stop/Start with overlapping transcription; assert delivery ledger claims prevent duplicate or interleaved text.
4. **LLM Cleanup Timeout Fallback & Diagnostic Emission**:
   - Test LLM latency >1500ms; verify fallback to raw/ASR transcript, diagnostic telemetry emitted, and successful typing.
5. **Metadata & Telemetry Consistency**:
   - Verify `voxi chunks show` reports correct `TypingStartedAt` / `TypingEndedAt` for flushed chunks.
