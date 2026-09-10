# 092 — eagerSessionManager.Toggle has a check-then-act race under concurrent SIGUSR1/socket invocation

**Status**: Closed
**Priority**: P2 (Medium)
**Severity**: Bug (race condition, hard-to-reproduce double-toggle)
**Category**: Go Code Quality / Concurrency
**Related**: [internal/eager/eager.go](../internal/eager/eager.go) (`eagerSessionManager.Toggle`, `runEagerDaemon`)

---

## 1. Problem

`runEagerDaemon` (eager.go:928-1040) dispatches `sessions.Toggle()` from two
independent goroutines that can run at the same time:

- The `SIGUSR1`/`SIGUSR2` signal-handling goroutine (eager.go:1002-1010),
  triggered by external callers (e.g. the GNOME extension keybinding,
  `voxi-modifierd`, or a user-bound hotkey sending the signal).
- A per-connection goroutine spawned for every accepted Unix-socket
  connection (eager.go:1026-1040), triggered by `voxi eager toggle` CLI
  invocations or other local clients writing `"toggle"` to the daemon socket.

`Toggle()` itself (eager.go:903-913) is a classic check-then-act race:

```go
func (m *eagerSessionManager) Toggle() string {
        m.mu.Lock()
        rec := m.isRecording
        m.mu.Unlock()
        if rec {
                m.Stop()
                return "Recording stopped"
        }
        m.Start()
        return "Recording started"
}
```

The mutex is released between reading `m.isRecording` and calling
`Stop()`/`Start()`. If two `Toggle()` calls race (e.g. a duplicate SIGUSR1
delivery and a near-simultaneous socket `toggle`, or two rapid hotkey
presses reaching the daemon through two different transports), both can read
`isRecording == false`, and both then call `Start()`. Since `Start()`
internally calls `Stop()` first (eager.go:869), the second `Start()`'s
internal `Stop()` will tear down the session the first `Start()` just stood
up — the net effect for the user is: dictation appears to start and then
immediately stop again, with no toggle actually landing in the "recording"
state despite two toggle requests being made. The reported message
("Recording started" from both calls) also becomes misleading, since one of
the two sessions is torn down microseconds after being reported as started.

## 2. Impact

- User-visible: a hotkey/signal delivered in quick succession — plausible
  during real usage if a keybinding fires more than once, or if both an
  evdev-modifier-driven signal and a manual CLI toggle land close together —
  can silently no-op the dictation start instead of toggling as expected,
  with no error surfaced anywhere.
- `Recording()` (eager.go:916-920) and `Toggle()`'s returned status string
  can each report state that's already stale by the time the caller reads
  it, compounding debugging difficulty for this class of bug.

## 4. Resolution (2026-09-11)

Closed by serializing the check-and-act portion of `Toggle()` with a dedicated
manager control mutex. The existing state mutex remains narrowly scoped, so
session stop waits and capture callbacks do not deadlock. Regression tests cover
concurrent toggles from both idle and recording states under the race detector.

## 3. Suggested Fix

Make the read-decide-act sequence atomic by holding `m.mu` across the
decision, or by moving the branch inside `Start`/`Stop`'s own locked
sections (e.g. a single `toggleLocked()` helper called under one lock
acquisition covering both the `isRecording` check and dispatching to the
Start/Stop logic). Add a test that fires concurrent `Toggle()` calls (e.g.
via `go test -race` with a small goroutine fan-in) and asserts the manager
ends in a single consistent state rather than racing.
