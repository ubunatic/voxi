# 114 — Fix out-of-checkout modifierd build fallback

**Status**: Closed — resolved and Podman-verified
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
- Exercise the out-of-checkout branch in an isolated integration test that produces an executable modifier daemon.

## 4. Resolution and Verification

`internal/install/install.go` now runs `go install ubunatic.com/voxi/cmd/voxi-modifierd@latest` with `GOBIN` set to the installer cache directory and includes the Go diagnostic on failure. The local checkout and packaged-binary paths remain unchanged.

`make test-install-podman` passed with the CLI installed by `go install` from local source, then invoked from the container's home directory. Its `voxi install --modifierd` step fetched and built the remote helper, staged the system unit, and installed both through the isolated sudo stub. `make check` passed.
