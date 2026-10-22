# 198 — Improve error reporting in voxi install and monitor for privileged/backend failures

**Status**: Closed
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Enhancement
**Related**: `internal/install/install.go`, `internal/monitor/render.go`, `internal/monitor/collector.go`

---

## 1. Problem & Motivation
1. `voxi install --modifierd` failed with:
   `[modifier daemon (privileged)] failed: sudo install modifier binary: exit status 1`
   `Error: install phase modifier daemon (privileged): sudo install modifier binary: exit status 1`
   `e.Run` hid or discarded stderr from the invoked commands (`sudo install ...`), leaving the user in the dark about WHY it failed (e.g. `sudo: a password is required`, fingerprint reader timeout, permission denied).
2. In `voxi monitor`, when ASR was offline, it only printed a brief warning like `⚠️ ASR backend OFFLINE: http://127.0.0.1:8735/v1 unreachable (dial tcp 127.0.0.1:8735: connect: connection refused)`. It did not explain what component was supposed to provide this endpoint (e.g. `lmcoder-proxy.service` or `voxi-r2t2.service`), why it might have failed, or how to fix it (e.g. `run: voxi doctor --fix` or `systemctl --user status lmcoder-proxy`).

## 2. Technical Specification / Findings
- In `internal/install/install.go`:
  Updated `DefaultEffects().Run`, `RunInteractive`, and `RunStdin` to capture `stderr` (and attach `os.Stdin` where applicable so interactive auth can prompt). On non-zero exit, the error message includes the captured stderr (e.g. `exit status 1: sudo: a password is required`).
- In `internal/monitor`:
  Enriched `ASRBackendState` with an `Advice` field and updated `checkASRBackendState` to inspect the manager, model, and port:
  - For `lmcoder` / port 8735: advises checking `lmcoder status` / `systemctl --user status lmcoder-proxy` and running `voxi doctor --fix`.
  - For `voxi-r2t2` / port 18131: advises checking `systemctl --user restart voxi-r2t2` and running `voxi doctor --fix`.
  - Updated `formatASRBackendLine` and `renderDaemons` to display the advice in the monitor TUI and speedlines.

## 3. Implementation & Verification
1. Modified `internal/install/install.go` to capture stderr and append it to returned errors. Added `TestDefaultEffectsRunCapturesStderr` in `internal/install/install_test.go`.
2. Modified `internal/monitor/collector.go` and `internal/monitor/render.go` to surface actionable remediation advice in ASR warnings and TUI lines. Added `TestCheckASRBackendStateOfflineAdvice` in `internal/monitor/asr_probe_test.go`.
3. Tested and verified all package tests pass with `make test-short`.
