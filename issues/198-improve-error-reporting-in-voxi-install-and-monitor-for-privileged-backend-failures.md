# 198 — Improve error reporting in voxi install and monitor for privileged/backend failures

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Enhancement
**Related**: `internal/install/install.go`, `internal/monitor/render.go`, `internal/monitor/collector.go`

---

## 1. Problem & Motivation
1. `voxi install --modifierd` failed with:
   `[modifier daemon (privileged)] failed: sudo install modifier binary: exit status 1`
   `Error: install phase modifier daemon (privileged): sudo install modifier binary: exit status 1`
   `e.Run` hides or discards stderr from the invoked commands (`sudo install ...`), leaving the user in the dark about WHY it failed (e.g. fingerprint reader timeout, permission denied, missing file, etc.).
2. In `voxi monitor`, when ASR is offline, it only prints a brief warning like `⚠️ ASR backend OFFLINE: http://127.0.0.1:8735/v1 unreachable (dial tcp 127.0.0.1:8735: connect: connection refused)`. It does not explain what component is supposed to provide this endpoint (e.g. `lmcoder-proxy.service` or `voxi-r2t2.service`), why it might have failed, or how to fix it (e.g. `run: voxi doctor --fix` or `systemctl --user status lmcoder-proxy`).

## 2. Technical Specification / Findings
- In `internal/install/install.go`:
  `DefaultEffects().Run` uses `exec.CommandContext(ctx, name, args...).Run()`, which attaches neither Stdout nor Stderr.
  When commands like `sudo` fail or prompt, the user doesn't see stderr output unless interactive or captured.
  Even better, capturing or displaying the command's stderr in error messages will show the exact failure reason (e.g. PAM authentication failure, file not found, permission denied).
  Also, privileged install commands should ideally stream stderr to `out` or capture stderr to include in `error`.
- In `internal/monitor`:
  Enrich `ASRBackendState` with actionable advice / diagnostic details:
  - If model uses lmcoder: mention `lmcoder status` / `lmcoder-proxy.service`
  - If model uses `voxi-r2t2`: mention `systemctl --user start voxi-r2t2`
  - Point to `voxi doctor --fix`

## 3. Implementation & Verification Plan
1. Update `Effects.Run` / command execution in `internal/install` to capture or stream stderr on failure.
2. Enrich monitor warnings and ASR backend status with guidance on causes and resolution.
3. Verify `voxi install --modifierd` error output and `voxi monitor` display.
