# 115 — Prevent modifier buffer flush drops from expired stopDrainTimeout

**Status**: In Progress — Implementation + Phase-3 review complete; blocked on human live-verification gate (§5.8/§6.3)
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

### Second Incident — Plain (Non-Buffered) Delivery Path Also Affected (Chunk 1120, 2026-09-14)

The same race also drops chunks on the plain (non-modifier-buffered) delivery
path — no modifier gating involved, just normal queue backlog at stop time.
Filed independently as issue 120, merged here since the root cause and fix
are identical.

- Chunk #1119 (8.0s utterance) finalized `14:47:39.77`, transcription ran
  `14:47:39.77` → `14:47:44.28` (4.5s, long sentence + LLM cleanup).
- Chunk #1120 (4.7s utterance, the last chunk of the session) finalized
  `14:47:44.017` — the same instant recording was stopped — but queued
  behind #1119 in the serial transcription pipeline. Its transcription
  didn't start until `14:47:45.80` and finished `14:47:48.80` (3.0s).
- `drain.stopAt` boundary was `~14:47:44.017`, deadline
  `14:47:44.017 + 5s = 14:47:49.017`. By the time the plain delivery path
  (`internal/eager/eager.go`, the non-buffered branch around line 748,
  second `drain.eligible()` check after `delivery.Claim` and the
  `TypingStarted` telemetry record) re-checked eligibility, only ~200ms of
  margin remained; it lost the race and hit `recordStaleDelivery`.
- `voxi chunks show 1120` confirmed: `accepted: true`, full 14-word
  transcript present, but `typing_started_at`/`typing_ended_at` both zero.
- Same silent-loss problem as the modifier-buffer path: `recordStaleDelivery`
  only writes a telemetry event, nothing user-visible.

This confirms the fix must cover **both** call sites gated by
`drain.eligible()` in `eager.go` (the buffered `FlushDeliveries` path and
the plain non-buffered path), not just the modifier-buffer one — they share
the same `sessionDrain`/`stopDrainTimeout` mechanism and the same flaw.

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

## 4. Comprehensive Edge Case Test Matrix

To guarantee rock-solid behavior across all timing and lifecycle boundaries, the implementation must add dedicated unit and integration tests covering these exact scenarios:

### Edge Case 1: Trailing Multi-Chunk Buffered Drain Exceeding Stop Timeout (Live Bug Repro)
- **Setup**: A session processes 2 valid speech chunks. During transcription of chunk 1, the user taps `Super` and then `Super+X` (Stop).
- **Condition**: Total sequential transcription and LLM processing takes >5.0s (e.g. 6.5s) after the stop request timestamp.
- **Assertion**:
  - `drain.cancel()` fires after 5s without aborting the transcription or injection of the buffered chunks.
  - All buffered chunks are injected into the target window in exact chronological sequence.
  - Zero `delivery_stale` telemetry events are emitted for valid chunks.

### Edge Case 2: Rapid `Stop` -> `Start` Session Interleaving
- **Setup**: Session A (with a slow draining chunk in `modifierBuffer`) is stopped, and Session B immediately starts and produces a chunk.
- **Condition**: Session B finishes capturing and transcribing before Session A's background worker finishes draining.
- **Assertion**:
  - Session A's flush does not overwrite or interleave mid-word with Session B's typing stream.
  - Delivery claims prevent duplicate typing.
  - Session A's `modifierBuffer` state does not leak into Session B (session isolation).

### Edge Case 3: LLM Cleaner Timeout & Degradation Fallbacks
- **Setup**: LLM post-processing server hangs or responds after >1500ms.
- **Assertion**:
  - Context timeout cancels the HTTP request after 1500ms.
  - Subprocess / HTTP client connection is closed immediately (no leaking sockets/goroutines).
  - The pipeline immediately falls back to the clean ASR transcript (0ms extra delay).
  - Diagnostic event `llm_cleanup_fallback` with reason `timeout` is logged.
  - Text is typed normally into the focused window without dropping.

### Edge Case 4: Acoustic Transient Short-Circuit during Stop Drain
- **Setup**: Trailing audio after speech contains only silence, breathing, or key clicks (`!job.Plausible`).
- **Assertion**:
  - Chunk is identified as implausible in `<1ms` and discarded without invoking Whisper/CrispASR or LLM.
  - Session shutdown and post-drain flush proceed immediately without waiting for unnecessary subprocesses.

### Edge Case 5: Full Metadata & Telemetry Integrity
- **Setup**: Inspect `chunkBuf` manifest and sidecar JSONs after a buffered flush.
- **Assertion**:
  - `typing_started_at` and `typing_ended_at` are properly populated with real timestamps instead of zero values (`0001-01-01T00:00:00Z`).
  - `voxi chunks show <N>` reflects `ACCEPTED` status and accurate latency breakdown.

---

## 5. Advisor Implementation Plan (Phase 1, 2026-09-14)

Read-only audit of `internal/eager/eager.go` (1694 lines), `eager_test.go`,
`delivery.go`, `spec/eager.yaml`, `internal/typing/typing.go`.

### 5.1 Additional findings beyond the ticket text

1. **Transcription is killed by the same 5s lease, not just delivery.**
   `eager.go:598` sets `transcribeParent := drain.ctx`, so `drain`'s
   `time.AfterFunc(5s, cancel)` kills the `voxtype`/`crispasr` subprocess 5s
   after stop — the nominal `transcribeTimeout` (30s) can never be reached for
   a post-stop chunk. Both incidents finished transcription with only
   ~200ms/~2.6s of margin; a marginally slower run loses the audio *before*
   delivery is even considered. Fixing only `drain.eligible()` would leave this
   second, earlier silent-loss vector in place.
2. **`cleanWithLLM` also inherits `drain.ctx`** (`eager.go:631`), so the LLM
   cleanup is cancelled by the same lease. Its own 1500ms bound
   (`eager.go:1062`, `1069`) and fall-back-to-ASR-text behaviour are already
   correct; what is missing is the reason taxonomy / `llm_cleanup_fallback`
   telemetry asked for by Edge Case 3.
3. **The buffered flush path records no typing telemetry at all.**
   `eager.go:940-967` emits `delivery_duplicate` / `delivery_stale` /
   `injector_*` but never `telemetry.TypingStarted` / `TypingComplete`, unlike
   the plain path (`747`, `760`). So buffered chunks are invisible in the
   typing-latency view even when they succeed.
4. **`bufferedDelivery` carries only `{ID, Text}`** (`eager.go:1356`), which is
   why the flush path cannot update `chunkMeta` (ticket §3.4) and why it passes
   a bogus `index 0` to `recordStaleDelivery` (`943`, `959`).
5. **`recordStaleDelivery` is silent by construction** — telemetry only, no
   `deps.Dependencies`, so nothing reaches stdout/journal. Contrast
   `reportEagerFailure` (`eager.go:1000`).
6. **A dropped chunk is in fact recoverable**: `history.AppendHistory`
   (`eager.go:768`) runs for every accepted chunk regardless of the
   typing/buffering branch, and `voxi history retype ID` (`cmd/voxi/main.go:251`)
   types a history entry into the focused window. The user-visible drop message
   should say so.
7. **`typing.TypeTextObserved` waits up to 5s** for physical modifier release
   (`internal/typing/typing.go:88-89`) using the passed ctx. Any injection
   timeout must therefore be comfortably above 5s (recommend 15s), and it must
   not be a context that is already about to be cancelled.
8. **Project spec rule applies**: new tunables belong in `spec/eager.yaml` +
   `spec/eager.go` + `spec/schemas/eager.schema.json`, not as bare Go consts
   (see `docs/Spec.md`; `modifier_gate` is the existing precedent).

### 5.2 Core architecture: replace the wall-clock lease with a generation signal

The 5s lease is a *proxy* for "a newer generation exists". Make the real signal
explicit and keep the wall clock only as a far-away safety net.

**Step 1 — make `stopRequest` carry the generation-superseded signal.**
`stopRequest` (`eager.go:99-114`) is already created per session in
`eagerSessionManager.Start()` (`1255`) and threaded into `run` → into
`runEagerCaptureSessionAt`. Extend it with a cancellable context instead of
inventing new plumbing:

```go
type stopRequest struct {
    mu sync.Mutex
    at time.Time
    // generation is cancelled when a *newer* session starts, i.e. the real
    // "this generation is abandoned" signal that stopDrainTimeout only
    // approximated with a 5s wall clock (issue 115).
    generation context.Context
    supersede  context.CancelFunc
}

func newStopRequest() *stopRequest {
    ctx, cancel := context.WithCancel(context.Background())
    return &stopRequest{generation: ctx, supersede: cancel}
}

// generationCtx is nil-safe: a zero-value or nil request is never superseded
// (standalone CLI path, and older tests that pass nil).
func (r *stopRequest) generationCtx() context.Context {
    if r == nil || r.generation == nil {
        return context.Background()
    }
    return r.generation
}

func (r *stopRequest) markSuperseded() {
    if r != nil && r.supersede != nil {
        r.supersede()
    }
}
```

Using a context (not a channel) means no extra watcher goroutine is needed —
the drain context is simply derived from it.

**Step 2 — supersede the previous generation in `Start()`, never in `Stop()`.**
This is the whole behavioural fix: *stopping* must not shorten the drain;
only *starting a new session* must.

- Add `prevRequest *stopRequest` to `eagerSessionManager` (`eager.go:1180`).
- `Stop()` (`1215`) keeps clearing `m.activeRequest` but must **not** clear or
  supersede `m.prevRequest`; set `m.prevRequest = request` there (and in the
  session-exit cleanup at `1275-1285` leave it alone).
- `Start()` (`1247`), after its internal `m.Stop()` returns and under `m.mu`:
  `m.prevRequest.markSuperseded()`, then `request := newStopRequest()` and
  `m.prevRequest = request`.
- Note `Start()` already calls `Stop()` first, so the supersede always happens
  strictly before the new session's capture begins — no window where the old
  generation is still considered current while the new one types.

**Step 3 — rebuild `sessionDrain` around the generation context.**

```go
// drainDeliveryDeadline is an absolute safety net, not the delivery policy.
// The policy is "deliver unless a newer generation exists" (see stopRequest
// .generation). This deadline only stops a hung/abandoned generation from
// typing minutes later when the user never starts another session; it is
// sized well above transcribeTimeout (30s per queued job) plus injection.
const drainDeliveryDeadline = 60 * time.Second  // -> spec/eager.yaml
const injectionTimeout      = 15 * time.Second  // -> spec/eager.yaml

func newSessionDrainFor(parent context.Context, timeout time.Duration) *sessionDrain {
    ctx, cancel := context.WithCancel(parent)
    return &sessionDrain{parent: parent, ctx: ctx, cancel: cancel, timeout: timeout}
}
```

- `eager.go:525` becomes
  `drain := newSessionDrainFor(request.generationCtx(), drainDeliveryDeadline)`.
- `stop()` / `stopAt()` keep their boundary bookkeeping; the
  `time.AfterFunc(d.timeout, d.cancel)` now fires at 60s, not 5s.
- Keep `eligibleAt()` exactly as-is — the capture-side frame boundary check
  (`eager.go:876`) is correct and is not part of this bug.
- Replace `eligible() bool` with a reason-returning form so drops can be
  explained to the user and to telemetry:

```go
// deliverable reports whether this generation may still type, and why not.
func (d *sessionDrain) deliverable() (bool, string) {
    d.mu.Lock()
    defer d.mu.Unlock()
    if d.parent != nil && d.parent.Err() != nil {
        return false, "superseded"   // a newer session is typing
    }
    if d.stopped && !time.Now().Before(d.deadline) {
        return false, "drain_deadline"
    }
    return true, ""
}
```
  Keep a thin `eligible() bool` wrapper if it keeps existing tests compiling.

**Step 4 — detach the injection context from the drain.**
Both `typing.TypeTextObserved(drain.ctx, ...)` call sites (`eager.go:752`,
`962`) must use an independent per-delivery context:

```go
injectCtx, cancelInject := context.WithTimeout(context.Background(), injectionTimeout)
typeErr := typing.TypeTextObserved(injectCtx, d, text+" ", observer)
cancelInject()
```

Rationale to put in the comment: eligibility is decided *before* the claim and
again immediately before injection; once keystrokes start, aborting mid-word is
strictly worse than finishing, so the injector must not inherit a context that
can be cancelled underneath it. `context.Background()` (not the daemon root
`ctx`) so a daemon shutdown mid-injection cannot leave half a word.

Net effect on the two reported incidents: chunks #563/#564 and #1120 are
transcribed to completion (no 5s subprocess kill), pass `deliverable()` because
no new session ever started, and are injected under their own 15s budget.

### 5.3 Never drop silently

Give `recordStaleDelivery` the pieces it needs to be user-visible, mirroring
`reportEagerFailure`'s stdout/journal contract (`eager.go:1000`,
`docs/EagerDeliverySafety.md` §"User-visible failures"):

```go
func recordStaleDelivery(d deps.Dependencies, recorder *telemetry.Recorder, sessionID, chunkID string, index int, reason string) {
    _ = recorder.Record(telemetry.Event{
        Event: telemetry.DeliveryStale, Timestamp: time.Now(), SessionID: sessionID,
        ChunkID: chunkID, ChunkIndex: index, DeliveryID: chunkID, CancelReason: reason,
    })
    if d.Stdout == nil {
        return
    }
    fmt.Fprintf(d.Stdout, "voxi eager: transcript not typed (session=%s chunk=%s reason=%s); recover with: voxi history retype 1\n",
        sessionID, chunkID, reason)
}
```

- Reuses the existing `CancelReason` telemetry field (`telemetry.go:63`) — no
  schema change needed.
- Deliberately does **not** print the transcript text (privacy contract in
  `docs/EagerDeliverySafety.md`); the recovery hint carries the value instead,
  and is honest because `history.AppendHistory` already ran (finding 6).
- Update all four call sites: `eager.go:558` (ineligible capture — reason
  `"capture_boundary"`), `733`, `749` (plain path), `943`, `959` (flush path).

### 5.4 Persist typing metadata on the buffered flush path

- Widen the buffered entry:
  `type bufferedDelivery struct { ID, Text string; Index int; Meta chunks.Chunk }`
  (`eager.go:1356`).
- `AppendDelivery(id, text string, index int, meta chunks.Chunk)` (`1387`) —
  exactly **one** production caller (`eager.go:727`) and **zero** test callers,
  so this is cheap. `Flush()` (`1427`) and the `pending`-only fallback entry
  (`1444`) are unaffected apart from the wider struct literal.
- In the flush loop (`940-967`), after a successful injection:

```go
typeStart := time.Now()
_ = recorder.Record(telemetry.Event{Event: telemetry.TypingStarted, ..., ChunkIndex: item.Index, Attempt: 1})
typeErr := typing.TypeTextObserved(injectCtx, d, item.Text, observer)
typeEnd := time.Now()
_ = recorder.Record(telemetry.Event{Event: telemetry.TypingComplete, ..., Success: &ok})
item.Meta.TypingStartedAt = typeStart
item.Meta.TypingEndedAt = typeEnd
_, _ = chunkBuf.Update(item.Meta)
```
  `chunkBuf` is in scope at the flush site (declared `eager.go:537`), and
  `chunks.Buffer.Update` (`internal/chunks/chunks.go:392`) is the same call the
  plain path already uses at `763`. This closes ticket §3.4 and Edge Case 5,
  and incidentally fixes the bogus `index 0` in the stale records.

### 5.5 LLM fallback taxonomy (ticket §3.3 / Edge Case 3)

`cleanWithLLM` (`eager.go:1013-1100`) already bounds at 1500ms on both the
request context and `http.Client.Timeout` and already falls back to the ASR
text on every error path — but every `return text, record` is anonymous. Add a
`reason` to `chunks.LLMCleanupRecord` (or a new field on it) and set it at each
return: `timeout` (`errors.Is(err, context.DeadlineExceeded)`),
`connection_error`, `http_status`, `invalid_schema` (decode failure / zero
choices), `empty_response`. Emit one telemetry event `llm_cleanup_fallback`
(new const in `internal/telemetry/telemetry.go:18-32`) carrying the reason in
`CancelReason`. This is the smallest scope item and can be dropped to a
follow-up ticket if the developer's budget runs tight — it is not part of the
drop bug.

### 5.6 Spec wiring

`spec/eager.yaml` — add a sibling section to `modifier_gate`:

```yaml
drain:
  # Absolute safety net only; the real gate is "a newer session started".
  delivery_deadline_ms: 60000
  # Per-delivery injection budget. Must exceed the 5s physical-modifier
  # release wait inside typing.TypeTextObserved.
  injection_timeout_ms: 15000
```

Mirror in `spec/eager.go` (`DrainSpec` struct + `DeliveryDeadline()` /
`InjectionTimeout()` accessors + positive-value validation in
`parseEagerSpec`, with `injection_timeout_ms > 5000` validated explicitly) and
in `spec/schemas/eager.schema.json` (add `drain` to `properties` and
`required`). Delete or repurpose `stopDrainTimeout`; if kept for the capture
boundary, rename it so nothing reads it as a delivery policy.

### 5.7 Test plan

All five edge cases are reachable with the harness that already exists in
`internal/eager/eager_test.go` — **no new fakes are needed**:

- *Slow transcription, deterministically*: the release-file fake `voxtype`
  shell script pattern from `TestStopDrainsInflightTranscriptionBeforeInjection`
  (`eager_test.go:422-426`) and the per-job counter variant in
  `TestStopDrainsInflightQueuedAndFlushedJobsInOrder` (`493`). The test holds
  the transcription hostage past any deadline by simply not touching the
  release file.
- *New session started mid-drain*: `newEagerSessionManager(...)` +
  `mgr.Start()` / `mgr.Stop()` / `mgr.Wait()`, as in
  `TestStopDrainsInflightQueuedAndFlushedJobsInOrder` (`540-560`) and
  `TestModifierBufferDoesNotLeakAcrossSessions` (`1342`).
- *Injection capture*: `deps.Dependencies.RunStdin` appending to a slice
  (`eager_test.go:535`) gives exact typed text and ordering.
- *Shortened timings*: use `newSessionDrainFor(parent, 25*time.Millisecond)`
  the way `newSessionDrainWithTimeout` is used today (`179`, `216`).

New/updated tests:

| # | Test | Asserts |
|---|---|---|
| EC1 | `TestBufferedFlushSurvivesDrainDeadlineWhenNoNewSessionStarts` | Fake ASR held past a short `drainDeliveryDeadline`-equivalent; both buffered chunks typed, in order, via `RunStdin`; **zero** `delivery_stale` events in the recorder; drain ctx cancellation does not abort the injection. |
| EC1b | `TestPlainDeliverySurvivesSlowQueueBacklogAfterStop` | Issue 120 repro on the non-buffered path (`eager.go:732/748`): two queued chunks, second released long after stop, both typed. |
| EC2 | `TestSupersededGenerationDoesNotTypeIntoNewSession` | Session A held in transcription; `mgr.Start()` for B; A's flush emits `delivery_stale` with `reason="superseded"`, types nothing, and the user-visible stdout line is present; B's own text types normally; `delivery.Claim` prevents any duplicate. |
| EC2b | `TestSessionDrainSupersedeBeatsWallClock` (unit) | Direct `newSessionDrainFor(genCtx, time.Hour)`: `deliverable()` is `(true,"")` after `stop()`, becomes `(false,"superseded")` the instant `markSuperseded()` is called, and `drain.ctx` is cancelled. Replaces the intent of `TestSessionDrainGenerationOverlapKeepsOnlyAuthorizedLease` (`215`), which should be kept as the absolute-net case. |
| EC3 | `TestCleanWithLLMFallsBackWithReason` | Table-driven against an `httptest.Server` that hangs / resets / returns bad JSON / returns `""`; asserts the ASR text is returned unchanged, the reason string, the `llm_cleanup_fallback` event, and that the hang case returns in ~1.5s, not later. Extends the existing `TestCleanWithLLM` (`1488`). |
| EC4 | `TestImplausibleTrailingChunkSkipsTranscriptionDuringDrain` | A trailing `!job.Plausible` chunk: the fake `voxtype` is never invoked (counter file stays absent) and the session returns promptly. Guards `eager.go:563-593`. |
| EC5 | `TestBufferedFlushPersistsTypingTimestamps` | After a buffered flush, `chunkBuf.Get(...)` shows non-zero `TypingStartedAt`/`TypingEndedAt` and `Accepted: true`; plus `typing_started`/`typing_completed` telemetry now present for the flushed chunk. |

Also update: `TestSessionDrainLeaseExpiresAndCancels` (`178`) — still valid as
the absolute-net test, but assert the `deliverable()` reason is
`"drain_deadline"`; `TestStopBoundaryPreservesReadCompletedBeforeCancellationObservation`
(`236`) uses `&stopRequest{}` — must still compile/pass with the zero value
meaning "never superseded".

Run `go test -race -count=1 ./internal/eager/...` then `make check`. Because a
new watcher-free context derivation replaces a timer, also confirm no goroutine
or `time.AfterFunc` leak under `-race` with `-count=5`.

### 5.8 Live verification gate (required)

Per `docs/AgenticLoop.md` Phase 3, unit tests are not sufficient for this
class of change — it alters the live daemon's delivery policy. After
`make restart-service`:
1. Dictate a long sentence, press Super+X while the last chunk is still
   transcribing, confirm the text lands (the EC1 live repro).
2. Dictate, stop, and immediately start a new session — confirm the old
   generation does **not** type into the new window, and that the drop line
   appears in `journalctl --user -u voxi-agent.service`.
3. `voxi chunks show <N>` on a buffered-flush chunk: `typing_started_at` /
   `typing_ended_at` must be real timestamps, not `0001-01-01T00:00:00Z`.
Record the outcome in this ticket before closing, and update
`docs/EagerDeliverySafety.md` §"Stop and queue semantics", which currently
documents the now-removed "bounded five-second drain lease".

### 5.9 Work split & staffing

**One bounded unit of work for a single `eager`-category developer.** The
changes are all within `internal/eager` plus a small, mechanical `spec/` +
`internal/telemetry` surface, and they are mutually dependent (the delivery
gate, the injection context, and the buffered-entry widening cannot be landed
independently without leaving a half-fixed drop path). Splitting would create
exactly the sequential-write contention `docs/AgenticLoop.md` Invariant 1 warns
about. §5.5 (LLM taxonomy) is the one cleanly separable slice and may be
deferred to its own ticket if needed.

**Recommended model: the top frontier model, not Sonnet.** Justification (retained
for the record; the work was implemented in Phase 2 — see section 6):
(a) the change rewires *context lifetime ownership* across three concurrent
actors (capture goroutine, transcription worker, session manager) where the
failure mode is a silent data loss that passes every existing test — precisely
the shape `docs/AgenticLoop.md` flags as "unit-test-only confidence" risk;
(b) it must preserve the issue 057 non-blocking-Stop invariant and the issue
101 buffer-isolation invariant while changing the mechanism both rely on;
(c) the original 5s lease was itself a deliberate safety measure, so the
developer must reason about *what it was protecting against* rather than
delete it — a judgement call, not a mechanical edit.

---

## 6. Implementation Record (Phase 2, 2026-09-14)

Implemented per the advisor plan, with section 5.5 included rather than deferred.

### 6.1 What landed

| Plan | Status | Notes |
|---|---|---|
| 5.2 generation-based supersede signal | done | `stopRequest.generation` / `markSuperseded`, `newSessionDrainFor`, `deliverable() (bool, reason)`, injection on its own `context.Background()` budget |
| 5.3 never drop silently | done | `recordStaleDelivery` takes `deps.Dependencies` + reason; all five call sites updated |
| 5.4 buffered-flush metadata | done | `bufferedDelivery{ID,Text,Index,Meta}`; flush path now records `typing_started`/`typing_completed` and persists `TypingStartedAt`/`EndedAt` via `chunkBuf.Update` |
| 5.5 LLM fallback taxonomy | done | `LLMCleanupRecord.FallbackReason` + `telemetry.LLMCleanupFallback` event |
| 5.6 spec wiring | done | `drain.delivery_deadline_ms: 60000`, `drain.injection_timeout_ms: 15000` in yaml, Go struct, schema, with validation that the injection budget exceeds the 5s modifier-release wait |
| 5.7 tests | done | EC1, EC1b, EC2, EC2b, EC3, EC4, EC5 plus the two existing-test updates |

Deviations from the plan:

- `stopDrainTimeout` was deleted outright rather than renamed; the capture
  boundary is `eligibleAt`, which never used the timeout.
- `eagerSessionManager.Wait()` releases the last generation context after
  `sessWg.Wait()`, so the final session's context does not outlive the manager.
  Nothing can still be pending at that point.
- EC3 asserts an extra `canceled` reason distinct from `timeout`, since a
  caller-cancelled cleanup (daemon shutdown) is a different diagnosis from a
  hung cleanup server.
- Spec wiring landed as the *first* commit, since the core change reads those
  values.

### 6.2 Automated verification

- `go test -race -count=1 ./internal/eager/...` — pass.
- `go test -race -count=5 ./internal/eager/...` — pass (43s), no goroutine or
  timer leak from replacing the `time.AfterFunc` lease with a context derivation.
- `make check` (vet + full suite + spec validation) — pass.
- Discrimination check: with `drain.delivery_deadline_ms` temporarily set back
  to `5000`, both `TestBufferedFlushSurvivesDrainDeadlineWhenNoNewSessionStarts`
  and `TestPlainDeliverySurvivesSlowQueueBacklogAfterStop` fail — the fake ASR
  subprocess is killed mid-transcription, exactly finding 5.1.1. They pass at
  60000. The repros therefore reproduce the live bug rather than merely
  exercising the code.

### 6.3 Live verification gate (section 5.8) — PARTIAL, human confirmation required

Verified:

- `make restart-service` rebuilt, installed, and restarted
  `voxi-agent.service`; the unit is `active` on the new binary with a clean
  journal.
- A live rapid `voxi record start` / `stop` / `start` / `stop` cycle against
  the real daemon ran clean — this is the path that now calls
  `markSuperseded()` — with no errors in
  `journalctl --user -u voxi-agent.service` and no new zombie processes.

**Not verified — requires a human at the microphone:**

1. Dictate a long sentence and press Super+X while the last chunk is still
   transcribing; confirm the text still lands.
2. Dictate, stop, then immediately start a new session; confirm the old
   generation does not type into the new window and that a
   `voxi eager: transcript not typed (... reason=superseded)` line appears in
   `journalctl --user -u voxi-agent.service`.
3. `voxi chunks show <N>` on a buffered-flush chunk: `typing_started_at` and
   `typing_ended_at` must be real timestamps, not `0001-01-01T00:00:00Z`.

Each has unit coverage (EC1/EC1b, EC2, EC5 respectively), but per
`docs/AgenticLoop.md` that is explicitly not sufficient for a change to the
live daemon's delivery policy. Record the outcome here before closing.

### 6.4 Phase-3 review fixes (commit `7c66434`)

Independent review passed the core fix and found one real defect plus doc drift:

1. **Broken recovery hint (fixed).** The drop message said
   `recover with: voxi history retype 1`, but `retype` resolves an entry by its
   8-character ID via `history.FindHistoryEntry`'s exact match, so that command
   always failed with `no history entry with id "1"` — at precisely the moment
   a user had lost a transcript. Verified against the real binary. The hint is
   now `voxi history list, then voxi history retype <ID>`, and the round trip
   (`history list` → ID → lookup) was confirmed end to end. The EC2 assertion
   that let this through (substring `"voxi history retype"` only) now asserts
   the full hint and rejects an index-shaped argument.
2. `docs/LLMTranscriptCleanup.md` and `docs/Roadmap.md` re-synced.
3. `sessionDrain.eligible()` removed (no production callers after
   `deliverable()`); `gofmt` applied to the field block the `prevRequest`
   comment split. Note `make check` does not run `gofmt -l`, which is why this
   was not caught automatically.

Section 6.3's live dictation gate is still open and still needs a human.
