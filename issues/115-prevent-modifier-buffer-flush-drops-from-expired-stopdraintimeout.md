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

## 3. Required Fix

1. **Remove `drain.eligible()` check in the post-drain `FlushDeliveries()` path**:
   - `eagerBuf` is session-local. Once `transWg.Wait()` completes, flushing the session's own buffered text is safe and must not be aborted due to a wall-clock drain timeout that was intended only to kill abandoned worker loops.
2. **Early exit for non-speech chunks**:
   - Confirm that unvoiced / low-energy transients (`!job.Plausible`) continue to short-circuit immediately without blocking transcription or notifications, ensuring the system only waits on chunks that actually contain speech.
3. **Update chunk metadata on buffered flush**:
   - Currently, `chunkMeta.TypingStartedAt` and `chunkMeta.TypingEndedAt` are only updated on the direct typing path (line 761), leaving buffered chunks with zero-value timestamps in `manifest.json` / sidecars even when delivered. Ensure `chunkBuf.Update` records the typing timestamp upon flush.
4. **Add automated regression tests**:
   - Test that a multi-chunk session entering `modifierBuffer` that takes longer than `stopDrainTimeout` to transcribe still flushes and delivers all buffered text upon completion.
