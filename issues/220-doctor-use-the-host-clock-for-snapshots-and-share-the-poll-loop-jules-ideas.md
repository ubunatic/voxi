# 220 — doctor: use the Host clock for snapshots and share the poll loop (Jules ideas)

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Refactor
**Related**: 211, `reports/jules/doctor-ideas.md`

---

/goal `internal/doctor` reads time only through `Host.Now()` and its two wait loops share one helper, with tests unchanged in meaning; or stop and report when blocked on a user decision.

## Findings (Jules ideas run, checked at 40f819f)
- `internal/doctor/fix.go:379` `now := time.Now()` in `snapshot` bypasses `Host.Now()` (`host.go:28`). Swapping it alone fails `TestSnapshotPrunesOldFiles`: `fixtureHost.Now()` returns the zero time, so prune deletes the new snapshot. Give the fixture a fixed clock in the same change.
- `waitForGraph` (`fix.go:203`) and `awaitStart` (`fix.go:434`) each run their own poll loop; one shared helper with a wall-clock deadline (as `awaitStart` already does) would do.
- Optional: split the `coredumpctl` text parsing out of `checkCoredumps` (`doctor.go:394`) for a direct unit test.

Dropped Jules ideas: structured exec errors in test fakes (tests already use `*exec.Error`, `doctor_test.go:371`); streaming `pw-dump` decode (one dump measured 9 ms in 214).
