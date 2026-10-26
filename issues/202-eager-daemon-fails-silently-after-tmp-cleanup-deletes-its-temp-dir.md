# 202 — Eager daemon fails silently after /tmp cleanup deletes its temp dir

**Status**: Open
**Priority**: P1 (High)
**Severity**: Major
**Category**: Bug
**Related**:

---

/goal The running `voxi-agent.service` keeps dictating after someone deletes its temp dir in
`/tmp`, verified by deleting the dir on the live service; or stop and report when blocked on a
user decision or denied permission. On a loop or no progress, clear the goal and write a failure report.

## 1. Problem & Motivation

On 2026-10-26, `/tmp` had run full and was cleaned up by hand. The cleanup deleted the eager
daemon's temp dir (`/tmp/voxi-eager-daemon-<rand>`). After that, every chunk failed with
`voxi eager: audio failed (...): open /tmp/voxi-eager-daemon-.../utt_001.wav: no such file or directory`,
and nothing got typed. `voxi monitor` still showed `● REC`, the mic level and the RTF as healthy,
so the user had no hint of what was wrong. Restarting the service fixed it.

## 2. Technical Specification / Findings

- The dir is created once at startup: `internal/eager/service.go:86`
  (`os.MkdirTemp("", "voxi-eager-daemon-*")`). Nothing recreates it.
- Possible fixes, in order of preference (the implementer picks one):
  1. Recreate the dir when it is missing (before each recording or chunk write), or
  2. detect this error and exit non-zero so systemd restarts the service (check `Restart=`).
  - Optionally move the dir to `$XDG_RUNTIME_DIR`, which a `/tmp` cleanup doesn't touch.
- Report repeated chunk/audio failures in the monitor's health line, so a broken pipeline never
  looks healthy.

## 3. Implementation & Verification Plan

- Unit test: delete the temp dir between chunks and check that the next chunk still succeeds
  (`t.TempDir()`).
- Live: `make restart-service`, then `rm -rf /tmp/voxi-eager-daemon-*`, dictate, and check that
  text gets typed and the journal shows no `no such file or directory`.
- Monitor shows a degraded state while chunks fail.
