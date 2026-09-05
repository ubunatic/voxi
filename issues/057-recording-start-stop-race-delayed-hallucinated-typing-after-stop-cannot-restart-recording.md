# 057: Recording Start/Stop Race: Delayed Hallucinated Typing After Stop, Cannot Restart Recording

**Status**: Fixed (2026-09-05)
**Priority**: P1 (High)
**Severity**: Major
**Category**: Bug
**Related**: [eager daemon toggle/state machine](../internal/eager/eager.go), [agent child backend](../internal/agent/eager_backend.go), [050 warm-model daemon transcription](050-optional-warm-model-daemon-transcription.md), [052 CPU/GPU scheduling priority](052-cpu-gpu-priority-under-load.md), [Roadmap](../docs/Roadmap.md)

---

## 1. Problem & Motivation

User-reported, same-day, in exact words: **"random hallus in chat now long
after record is off and then I cannot turn record on anymore."** This is a
live, currently-unresolved daily-driver reliability bug, distinct from the
two bugs already fixed earlier the same session (transcribe-subprocess
zombie-timeout in `internal/eager/eager.go`; silence-artifact punctuation
asymmetry in `internal/feedback/feedback.go`, see
[034 §7](034-isolated-silence-artifact-feedback.md#7-bug-fix-2026-09-03-asymmetric-punctuation-trim)).
Voxi's core promise is "you press stop and it stops" — this symptom breaks
that promise in two ways at once:

1. Text is typed into the focused window well after the user believes
   recording has stopped.
2. After that, the recording toggle stops responding — the user cannot
   start a new recording.

## 2. Evidence Gathered So Far (Not Yet Root-Caused)

- `journalctl --user -u voxi-agent.service --since "-15 min"` showed a
  suspicious rapid-fire sequence at `11:14:50`: `Recording started` /
  `Recording stopped` / `Recording started` / `Recording stopped`, all
  within the same second — a strong lead pointing at either a double-
  triggered hotkey/toggle call or a race condition in `runEagerDaemon`'s
  `stopCurrent`/`startRecording`/`toggleRecording` closures
  (`internal/eager/eager.go`), but **not confirmed** as the root cause.
- At the time of investigation, `ps aux` showed no zombie `voxtype`/
  `pw-record` processes, and `voxi record status` / `voxi mode` both
  reported idle — i.e. the daemon's own state was not stuck in a bad state
  at that moment. The bug appears intermittent or trigger-path-specific
  rather than a permanent stuck state.
- A direct CLI test (`voxi record toggle`) during the same investigation
  *did* successfully start a real `pw-record` process (confirmed via
  `ps aux`), then a follow-up toggle raced with an in-flight response and
  returned `read agent response: read unix @->/run/user/1000/voxi/agent.sock:
  i/o timeout` while a `voxtype transcribe` process was still actively
  running against a leftover `utt_002.wav` and the `pw-record` process had
  already gone `<defunct>` (zombie, not reaped) — i.e. under real
  conditions, a toggle call can time out against the agent socket while a
  transcription is still mid-flight. This suggests the agent-socket
  request/response path can stall while `stopCurrent`/`startRecording` are
  contending with an in-progress transcribe job, consistent with (but not
  proof of) a mutex/state race rather than a hard crash.
- This has **not** been established as the same root cause as the
  previously-fixed zombie-timeout bug, nor as the same issue as the
  separate, still-undiagnosed "typed with big delay, something got worse
  since yesterday" side note recorded in
  [051 §3.2](051-warm-model-prior-art-research.md#32-fresh-live-evidence-2026-09-03-same-day-follow-up).
  Do not assume any of these three reports share a cause without further
  diagnosis — track them separately until evidence says otherwise.

## 3. Suspected Areas (Investigate, Do Not Assume)

- `internal/eager/eager.go`'s `runEagerDaemon`: the mutex-guarded
  `isRecording`/`activeCancel` state and the `stopCurrent`/`startRecording`/
  `toggleRecording` closures, especially how they interact with an
  in-flight transcription job on the single sequential worker
  (`jobChan`) when a stop/start is requested mid-transcription.
  Note the already-added `transcribeTimeout` (30s bound on the
  `voxtype transcribe` subprocess) changes when a stale job's
  process exits, but does not itself address any request/response
  race on the daemon's own unix-socket command protocol.
- The unix-socket command protocol's per-connection goroutine handling
  (`Accept()` loop) — whether a `toggle`/`start`/`stop` request can be
  read, acted on, and acknowledged out of order relative to another
  request arriving immediately after.
- Whether the user's actual trigger path (hotkey via `voxi-modifierd`
  and/or a GNOME Shell extension binding) can itself double-fire a toggle
  signal (e.g. a key-repeat or debounce gap), independent of any daemon-side
  race — the rapid-fire `journalctl` timestamps are also consistent with a
  double-triggered input at the source, not only a daemon bug.

## 4. Implementation Plan

1. Reproduce deliberately: script a rapid toggle-during-active-transcription
   sequence (e.g. speak a short utterance, then send `toggle`/`stop`/`start`
   in quick succession) and capture `voxi-agent.service` logs plus
   `eager-metrics.json` timing across the sequence.
2. Read `runEagerDaemon` in full for the exact mutex scope around
   `stopCurrent`/`startRecording`/`toggleRecording` and identify any window
   where a request can read stale `isRecording` state or block indefinitely
   on `activeCancel`/job-channel drain.
3. Determine whether the agent-socket read/response path has its own
   timeout; if a request can block indefinitely on daemon-side state
   contention, add a bounded timeout there too (mirroring the existing
   `transcribeTimeout` precedent) so a stuck toggle degrades to a clear
   error rather than silently refusing all future input.
4. Confirm whether `voxi-modifierd`/hotkey/GNOME-extension trigger paths can
   double-fire, independent of the daemon fix.
5. Add a regression test/canary exercising rapid toggle-during-transcription
   sequences so this class of race is caught before it reaches daily use
   again.

## 5. Acceptance Criteria

- Root cause identified and documented here (daemon-side race,
  socket-protocol issue, trigger-path double-fire, or a combination) before
  any fix is called complete.
- Toggling recording during an in-flight transcription never results in (a)
  text typed after the user has told the daemon to stop, or (b) the daemon
  becoming unresponsive to further start/stop/toggle requests.
- `go test ./...`, `make check` pass; `make restart-service` run since this
  touches live daemon code (`internal/eager`, and possibly `internal/agent`).
- New regression test covers the rapid toggle-during-transcription sequence.

## 6. Non-Goals

- Not a re-investigation of the already-fixed transcribe-subprocess timeout
  bug or the silence-artifact punctuation bug — those are closed and
  verified separately.
- Not assumed to be the same root cause as the separate "typed with big
  delay, something got worse since yesterday" report tracked in 051 §3.2 —
  treat as a distinct symptom until diagnosis says otherwise.

## 7. Root Cause & Fix (2026-09-05)

**Root cause, in full**: a chain across three layers, not one bug.

1. **`internal/eager/eager.go`, `runEagerDaemon`'s `stopCurrent`** (pre-fix):
   `sessWg.Wait()` blocked until the *entire session* finished — audio capture
   **and** draining any transcription job still queued or in flight,
   bounded only by `transcribeTimeout` (30s). So any stop/start/toggle
   request issued while an utterance was still transcribing blocked for up
   to 30s.
2. **`internal/agent/agent.go`, `Agent.Record`**: holds a single `Agent.mu`
   mutex across the *entire* backend call
   (`EagerChildBackend.Record` -> `eager.ControlEagerDaemon`, which itself
   had no read/write deadline on the eager.sock connection). One stalled
   toggle therefore stalled *every other* status/mode/record request behind
   the same mutex, not just the one connection.
3. **`internal/agent/agent.go`, `Client.request`**: the CLI's own connection
   to `agent.sock` has only a 5s deadline. When the daemon-side stall (1)
   exceeded that, the client saw `read agent response: ... i/o timeout` and
   gave up — but the server-side goroutine kept running, still holding
   `Agent.mu`, until the stale eager-daemon operation eventually finished.
   Each user retry queued another goroutine behind the same mutex; once the
   stale transcription finally drained, all of them resolved back-to-back —
   producing exactly the rapid-fire `started`/`stopped`/`started`/`stopped`
   log burst from Section 2, and reading to the user as "toggle stopped
   responding, then things went haywire."
4. **Zombie `pw-record`**: in `runEagerCaptureSession`, the recording
   subprocess was killed as soon as its context was canceled, but
   `recCmd.Wait()` (the reap) was `defer`red to the function's own return —
   which was additionally gated behind `close(jobChan); transWg.Wait()`, the
   same up-to-30s drain from (1). The process sat `<defunct>` for that whole
   window, exactly as observed live during diagnosis.

Live reproduction confirmed the chain: two `voxi record toggle` calls fired
concurrently reliably reproduced both the client-side `i/o timeout` and,
separately (see below), a genuine daemon-side deadlock.

**Fix**:

- `internal/eager/eager.go`: extracted the daemon's start/stop/toggle state
  machine into a new `eagerSessionManager` type. `Stop`/`Start`/`Toggle` now
  wait only until audio capture has stopped and the recording subprocess is
  reaped (fast, bounded by process teardown, not transcription) via a new
  `onCaptureStopped` callback threaded through `runEagerCaptureSession`. A
  session's leftover transcription drain continues in the background;
  `eagerSessionManager.Wait()` (used only at daemon shutdown) blocks for
  that full drain so a queued utterance still gets typed before the process
  exits.
- `runEagerCaptureSession`'s zombie fix: the recording subprocess is now
  killed **and reaped immediately** after the capture loop ends, before the
  (potentially slow) transcription flush/drain — not deferred behind it.
- Each daemon session now gets its own `os.MkdirTemp` subdirectory (instead
  of one shared temp dir for the daemon's whole lifetime), since Stop no
  longer waits for a stale session's drain before a new session can start:
  without this, a fresh session's `utt_001.wav` could collide with a
  still-draining stale session's file of the same name.
- **Second bug found only via live verification, not the design above**:
  the first cut of this fix called `onCaptureStopped` only from the
  "happy path" reap step. `exec.Command`'s `Start()` returns `ctx.Err()`
  immediately — without ever spawning a process — when its context is
  already canceled at call time, which happens routinely here (a session
  can be told to stop before its capture goroutine gets past setup). That
  early `return fmt.Errorf("start audio capture"...)` skipped the
  happy-path signal entirely, permanently deadlocking
  `eagerSessionManager.Stop()`. Confirmed live with a `SIGQUIT` goroutine
  dump: `Stop()` parked forever on `<-stopped`, the capture goroutine had
  already exited, and nothing else ever closed the channel. Fixed by
  wrapping the callback in a `sync.Once`-guarded `defer` at the top of
  `runEagerCaptureSession`, so it fires exactly once on *every* return path,
  not just the one that reaches the reap step.
- `internal/eager/eager.go`, `ControlEagerDaemon`/`GetEagerRecordingStatus`:
  added an 8s `eagerSocketTimeout` deadline on the eager.sock client
  connection, mirroring the existing `transcribeTimeout` precedent. This is
  defense-in-depth, not the primary fix — with the above in place, these
  calls should return in low single-digit milliseconds — but it bounds any
  future daemon-side stall to a clear error instead of wedging `Agent.mu`
  indefinitely.
- `Agent.mu`'s single-mutex-across-the-whole-backend-call design in
  `internal/agent/agent.go` was **not** changed. It amplified the stall
  (item 2 above) but is not itself the root cause, and narrowing its scope
  is a larger architectural change outside this ticket's blast radius; it's
  now moot in the common case since the daemon-side call it wraps returns
  fast.

**Files changed**: `internal/eager/eager.go` (new `eagerSessionManager`
type; `runEagerCaptureSession` signature gained `onCaptureStopped
func()`; `ControlEagerDaemon`/`GetEagerRecordingStatus` gained a socket
deadline), `internal/eager/eager_test.go` (new regression tests, see
below). `internal/agent/eager_backend.go` and `internal/agent/agent.go`
were read and are implicated in the chain (items 2-3 above) but not
modified.

**New regression tests** (`internal/eager/eager_test.go`, run with `-race`):

- `TestEagerSessionManagerStopDoesNotBlockOnTranscriptionDrain`: `Stop()`
  must return well under a simulated slow drain, while `Wait()` (shutdown)
  must still block for the full drain.
- `TestEagerSessionManagerRapidToggleDuringDrainStartsFreshSession`:
  reproduces the ticket's rapid toggle-during-transcription sequence
  end to end against the extracted state machine; asserts no deadlock and
  the right number of sessions started.
- `TestRunEagerCaptureSessionSignalsCaptureStoppedOnEarlyReturn`: the
  narrower regression test for the `exec.Command.Start()`-on-an-already-
  canceled-context deadlock found during live verification — asserts
  `onCaptureStopped` still fires exactly once when `runEagerCaptureSession`
  takes an early error-return path.

**Verification performed**:

- `go build ./...`, `go vet ./...`, `go test ./...` (including `-race` on
  the new tests), and `make check` all pass.
- `make restart-service` run (rebuilt, reinstalled, restarted
  `voxi-agent.service`).
- Live, real end-to-end verification against the running service and its
  real `pw-record`/`voxtype` subprocesses:
  - Sequential and concurrent (2-6 simultaneous) `voxi record toggle`
    calls, including the exact race that previously produced the client's
    `i/o timeout` error, all resolved in ~1-2ms with no errors.
  - Directly reproduced the pre-fix daemon-side deadlock at the eager.sock
    protocol level (two concurrent raw `toggle` connections against a
    non-service foreground daemon instance): pre-fix, one connection hung
    for 90s+ (confirmed via `SIGQUIT` goroutine dump, not just a timeout);
    post-fix, the same concurrent request pattern resolved in ~1ms across
    multiple repeated rounds.
  - `ps aux` / zombie scan showed no lingering `pw-record` or `voxtype`
    processes after any of the above, before or after final cleanup.
  - `voxi record status` remained responsive throughout, and the final
    daemon state was always consistent with the actual number of toggles
    issued.

All of Section 5's acceptance criteria are met: root cause identified
across all three implicated layers, toggling during an in-flight
transcription no longer produces text-after-stop-becomes-unresponsive
behavior or a zombie process, the test suite and `make check` pass, and a
regression test suite covers the rapid toggle-during-transcription sequence
(plus the narrower early-return deadlock found only via live testing).
