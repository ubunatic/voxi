# 103 — Normal (Non-Final) Utterance Queued Just Before Stop Is Killed Via Canceled Session ctx, Silently Dropped

**Status**: Open
**Priority**: P1 (High)
**Severity**: Major
**Category**: Bug
**Related**: [083 reject runaway repeated dotool injection](083-prevent-runaway-repeated-dotool-desktop-injection.md), [057 recording start/stop race](057-recording-start-stop-race-delayed-hallucinated-typing-after-stop-cannot-restart-recording.md), [101 modifier-release race](101-modifier-release-race-leaks-buffered-typing-into-gnome-overview-search-box.md)

---

## 1. Problem

During a live dictation session on 2026-09-09, the trailing words of a
message — "...the last chunk did not make it through clean in this text
here 'super-x' ending did not type" — were silently dropped: never typed,
never surfaced as an error.

Root cause confirmed live via `voxi chunks show`:

- **Chunk #1734**, session `20260909T143751.538053041Z-000005`, chunk `/9`,
  timestamp 2026-09-09 16:38:34, audio 1.70s, `voiced_ratio` 0.718,
  `probable_silence` false (genuine speech, not noise) — **Rejection Reason:
  `transcribe_error: signal: killed`**, Transcribe Duration 0.04s (killed
  almost instantly after the transcription subprocess started), Transcript
  Word Count 0, Raw/Cleaned Transcript both empty. The WAV may still be in
  the local ring buffer — re-check with `voxi chunks show 1734` if needed,
  but this diagnostic text is sufficient as the fixture on its own even if
  it has since rotated out.

This was found during live verification of issue 101 (modifier-release race
fix) but is unrelated to 101's scope (buffering during a held modifier) —
it reproduces purely from stop timing, with no modifier involved.

## 2. Mechanism (`internal/eager/eager.go`)

In `runEagerCaptureSession`'s capture loop, once the segmenter
(`segmenter.ProcessFrame`) finalizes a candidate utterance, it's queued onto
`jobChan` as a normal `TranscribeJob` (`Final: false`) at lines 714–730. The
single sequential transcription worker goroutine pulls jobs off `jobChan`
and derives each job's transcription-subprocess context via (lines 499–507):

```go
transcribeParent := ctx
if job.Final {
    transcribeParent = context.Background()
}
transcribeCtx, cancelTranscribe := context.WithTimeout(transcribeParent, transcribeTimeout)
cmd := exec.CommandContext(transcribeCtx, transcribeBinPath, cmdArgs...)
```

Only the *one* trailing utterance produced by `segmenter.Flush()` at the
moment capture stops (lines 750–768) is marked `Final: true`, per the
`TranscribeJob.Final` doc comment at lines 340–349, and therefore
transcribes on `context.Background()` — immune to the session's own `ctx`
being canceled. Every other already-finalized-and-queued (but not yet
transcribing, or mid-transcription) job still derives from the session
`ctx`, which is canceled the instant `eagerSessionManager.Stop()` fires
(explicit user stop, Super+X, or the session simply ending).

If a normal (non-Final) utterance finishes segmentation and is queued onto
`jobChan` shortly before stop, and the worker hasn't started (or has only
just started) transcribing it when `ctx` cancels, its
`exec.CommandContext`-spawned subprocess is killed via the canceled
context — chunk #1734's `signal: killed` after 0.04s. The utterance is lost:
no retry, no typed output, no recovery, no user-visible indication beyond a
rejected chunk in diagnostics.

The `Final`/`segmenter.Flush()` mechanism (added in 083 §8, commit
`8795182`) protects only the segmenter's own buffered-but-not-yet-finalized
trailing speech at the moment of stop. It does not protect an
already-finalized utterance still sitting in (or just pulled off) `jobChan`
at that same moment — a distinct gap in the same area of code.

## 3. Practical Implication / Relation to Other Tickets

- This is a genuine, live-reproduced instance of the failure class 083
  exists to close: pathological/lost output at the injection boundary —
  specifically a **dropped-on-stop** utterance, rather than 083's primary
  focus (runaway/repeated output limits and an at-most-once delivery
  ledger). 083 §8 already fixed the sibling case (the segmenter's own
  trailing Flush()'d utterance being wrongly dropped by the original
  ctx-cancellation fix) and explicitly flags in its closing paragraph that
  "the at-most-once/generation-ID design still needs to distinguish 'the
  last thing the user said, right up to stop' from 'a stale or pathological
  result arriving after stop' — collapsing both into one `ctx.Err()` check
  reintroduces this regression." This ticket is exactly that: a case 083's
  existing Final/Flush fix does not cover because the dropped job here was
  never part of the segmenter's flush — it was already finalized and queued
  before stop, on the ordinary non-Final path.
- 083's own "remaining work" (§7) separately names a still-unbuilt
  "cross-session delivery ledger giving each accepted transcript an
  at-most-once identity." Whether this gap is best understood as part of
  that ledger work, or as a narrower fix to the Final/ctx-derivation logic
  on its own, is an open disposition question (see §5 below) — not decided
  here.
- 057 is the origin of the general session-ctx-cancellation-on-stop design
  this gap sits next to (the `eagerSessionManager.Stop()`/fast-cancel
  contract that replaced the old blocking drain). 057's whole point was
  making `Stop()` *not* block on stale transcription drains — any fix here
  must not reintroduce that regression (see options below).
- Unrelated in scope to 101 (modifier-release race): 101 is about
  target-window safety at a modifier-release edge; this reproduces with no
  modifier involved, purely from ordinary stop timing.

## 4. Non-Goals

- Not a re-diagnosis of 083's core pathological-output-rejection logic —
  chunk #1734 was correctly rejected (empty transcript, `signal: killed`),
  not wrongly accepted. The gap here is *loss of legitimate speech*, the
  inverse problem from 083's original `Ubuntuktuktuktuk...` incident.
- Not a re-litigation of 057's stop-should-not-block design — any fix must
  preserve fast, non-blocking `Stop()` for the common case.
- Not an implementation ticket — see explicit scope note below.

## 5. Scope — Open Questions Only, No Fix Prescribed

Do not implement anything here. These are unranked options for whoever
picks this up, recorded as open questions:

- Should *every* queued/in-flight job at stop time get the same
  `context.Background()` treatment as the Final job, not just the
  segmenter's own flushed trailing utterance? Tradeoff: could delay a "fast"
  stop indefinitely if a slow transcription is in flight — this directly
  conflicts with 057's whole point (making `Stop()` not block on stale
  transcription drains). A blanket fix here risks reintroducing that
  regression and needs explicit reconciliation with 057's design, not just
  a mechanical widening of the `Final` condition.
- Could the killed job's audio be preserved (it is already written to the
  ring buffer via `chunkBuf.AddExistingWAV`, confirmed in this session's
  reading of the code at line 492) and retried/resumed later, or surfaced
  to the user as a recoverable "we lost this one, here's the audio" instead
  of silently vanishing?
- Should there be a short grace window between `ctx` cancellation and
  actually killing an in-flight transcription subprocess, distinct from a
  full background-context exemption — i.e. a bounded grace period rather
  than either "always killed immediately" or "never killed"?
- Is this the same class of gap the "cross-session delivery ledger"
  mentioned in 083's §7 remaining work is meant to eventually close, making
  this ticket's real disposition "fold into 083 once that ledger work
  starts" rather than a fully independent fix track? This is recorded here
  explicitly as an open disposition question, not a decision — whoever
  picks up either ticket should re-read both before choosing.

## 6. Reproduction / Fixture Reference

Chunk #1734 (session `20260909T143751.538053041Z-000005`, chunk `/9`,
2026-09-09 16:38:34) is the reproducible fixture: 1.70s audio,
`voiced_ratio` 0.718, `probable_silence` false, `transcribe_error: signal:
killed`, 0.04s transcribe duration, empty transcript. Re-check with `voxi
chunks show 1734` if the chunk is still in the local ring buffer at the
time this is picked up; not required to still exist since the diagnostics
above are self-contained.
