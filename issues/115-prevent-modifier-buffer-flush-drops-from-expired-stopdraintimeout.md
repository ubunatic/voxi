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

## 3. Required Fix & Timeout Architecture (Zero Zombie Guarantee)

To guarantee no hung processes, no zombie worker loops, and reliable delivery of valid chunks:

1. **Bounded Per-Stage Timeouts**:
   - **ASR Transcription**: Bounded per job via `transcribeTimeout` (e.g. 30s) so an engine crash or infinite loop is killed and reaped immediately.
   - **LLM Cleanup**: Bounded via HTTP request context (`context.WithTimeout(ctx, 1500*time.Millisecond)`).
2. **User Notification & Fallback on LLM Timeout**:
   - When the LLM cleaner HTTP call times out or fails, the pipeline must:
     - Fall back immediately to the clean ASR transcript so speech is **never lost**.
     - Emit a diagnostic warning / notification to the user (e.g., in daemon log & telemetry `llm_cleanup_timeout`, and optional audible/visual status alert) informing them that LLM post-processing timed out and the raw/ASR transcript was used instead.
3. **Safe Post-Drain Flush**:
   - Remove `drain.eligible()` check in the post-drain `FlushDeliveries()` path.
   - Once `transWg.Wait()` completes, the sequential worker has finished all in-flight jobs. Flushing the session's own buffered text is safe and must not be aborted by a wall-clock session drain deadline.
4. **Early Exit for Non-Speech Chunks**:
   - Confirm that unvoiced / low-energy transients (`!job.Plausible`) continue to short-circuit immediately without blocking transcription or notifications, ensuring the system only waits on chunks that actually contain speech.
5. **Update Chunk Metadata on Buffered Flush**:
   - Ensure `chunkMeta.TypingStartedAt` and `chunkMeta.TypingEndedAt` are updated and persisted to `manifest.json` / sidecars when delivered via `FlushDeliveries()`.
6. **Automated Regression Tests**:
   - Test that a multi-chunk session entering `modifierBuffer` that takes longer than `stopDrainTimeout` to transcribe still flushes and delivers all buffered text upon completion.
   - Test that an LLM cleaner timeout falls back cleanly to the base transcript, emits a user-visible warning, and delivers the text without getting dropped.
