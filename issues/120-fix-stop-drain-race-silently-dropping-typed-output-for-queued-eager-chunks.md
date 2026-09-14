# 120 — Fix stop-drain race silently dropping typed output for queued eager chunks

**Status**: Closed — Duplicate of issue 115, evidence merged
**Priority**: P1 (High)
**Severity**: Major
**Category**: Bug

---

## 1. Problem & Motivation

User report: "chunk 1120 was not typed." Confirmed via `voxi chunks show 1120`:
the chunk was fully transcribed and accepted (`accepted: true`, 14-word
transcript, `raw_transcript` present) but `typing_started_at`/`typing_ended_at`
are both zero — the dictated text was silently dropped and never typed into
the focused window.

## 2. Root Cause

`internal/eager/eager.go`: `stopDrainTimeout` (5s, line 86) bounds how long
in-flight chunks are allowed to still type after `eagerSessionManager.Stop()`
is called. It is a single fixed wall-clock deadline
(`boundaryAt + stopDrainTimeout`) shared by every chunk still queued or
transcribing at stop time, checked via `sessionDrain.eligible()`.

Transcription (including LLM cleanup) runs serialized. In the reproduced
case:

- Chunk 1119 finalized 14:47:39.77, an 8s utterance that took until
  14:47:44.28 to transcribe (4.5s, long sentence + cleanup).
- Chunk 1120 finalized 14:47:44.017 (same instant the user stopped
  recording — "Recording stopped" logged then), but had to wait behind 1119
  in the transcription queue; its own transcription didn't start until
  14:47:45.80 and took another 3.0s (transcribe + LLM cleanup), finishing
  14:47:48.80.
- Drain deadline was `14:47:44.017 + 5s = 14:47:49.017`. By the time the
  pipeline reached the second `drain.eligible()` check (after claiming the
  delivery and recording a `TypingStarted` telemetry event —
  `internal/eager/eager.go` around line 748), only ~200ms of margin
  remained; it lost the race and hit `recordStaleDelivery` instead of
  calling `typing.TypeTextObserved`.

`recordStaleDelivery` (line 989) only records a telemetry event — no
stdout/journal message, no user-visible signal of any kind. The user has no
way to know spoken content was dropped except noticing missing text.

The 5s deadline's actual purpose (per its own comment) is to stop an
*abandoned* generation from typing into whatever now has focus after a
*newer* session has started — not to bound normal same-generation drain
when no new session ever starts. A backlog of 1-2 long queued utterances at
stop time can exhaust the whole window before the last chunk even starts
transcribing, well before any new session begins.

## 3. Desired Fix

Two independent problems, both should be addressed:

1. **Don't drop legitimately-queued audio captured before stop.** The
   drain deadline should not evict chunks whose only "crime" was queueing
   behind other pre-stop audio in a serial transcription pipeline. Options
   to evaluate:
   - Only cut off a chunk's delivery if a *newer* session has actually
     started (i.e. tie eligibility to `Start()`'s cancel of the old
     `sessCtx`/drain, which already exists as a separate mechanism) rather
     than pure wall-clock elapsed time.
   - If a bounded timeout is still wanted as a fallback (e.g. genuinely
     stalled transcription), make it scale with queue depth or measure
     from each chunk's own finalize time / transcription start, not a
     single shared deadline from stop.
2. **Never silently drop accepted, transcribed content.** Regardless of
   the eligibility fix, `recordStaleDelivery` should surface something
   user-visible (matching `reportEagerFailure`'s existing pattern for
   daemon/stdout visibility) when an accepted transcript is discarded
   instead of typed, so this class of loss is never silent again.

## 4. Verification

- Add a test reproducing the queued-backlog race deterministically (e.g.
  inject an artificial transcription delay for two queued chunks that
  together exceed the current fixed deadline, assert both still get
  typed when no new session starts).
- Add/extend a test asserting a genuinely abandoned chunk (newer session
  started before it finishes) is still correctly suppressed — must not
  regress the original anti-leak guarantee.
- Live smoke test: reproduce a similar back-to-back long-utterance pattern
  via `voxi eager --type` and confirm `voxi chunks show <N>` shows
  non-zero `typing_started_at`/`typing_ended_at` for every accepted chunk.
