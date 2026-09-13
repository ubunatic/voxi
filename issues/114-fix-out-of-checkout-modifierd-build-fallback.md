# 114 — Fix out-of-checkout modifierd build fallback

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Bug
**Related**: commit `a1e522a`, [104 installer](104-add-voxi-install-command-for-complete-user-install-with-optional-privileged-setup.md)

---

## 1. Problem & Motivation

When `voxi-modifierd` is neither packaged nor available from a local source checkout, the installer promises to build it from the remote module. That fallback always fails because `go build` does not accept a package path with `@latest`. A user installing the optional modifier daemon outside a checkout cannot use this path.

## 2. Technical Findings

`internal/install/install.go` calls:

```text
go build -o <target> ubunatic.com/voxi/cmd/voxi-modifierd@latest
```

A reproduction from `/tmp` exited 1 with: `can only use path@version syntax with 'go get' and 'go install' in module-aware mode`. No binary was created. The installer suppresses the command's diagnostic and eventually returns a generic missing-binary error.

## 3. Implementation & Verification Plan

- Use a supported versioned install flow (for example, `go install` with a temporary `GOBIN`) or fetch a versioned module before building.
- Preserve the existing local checkout and packaged-binary paths.
- Surface the underlying Go error when remote installation fails.
- Add a test that exercises the out-of-checkout branch without network dependence, and run an isolated integration canary that produces an executable modifier daemon.

**Status**: Draft

---

Reserved placeholder ticket.
