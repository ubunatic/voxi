# 116 — Test installer portability across distributions and real systemd

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Infrastructure
**Related**: [Installation Architecture](../docs/InstallationArchitecture.md), [104 converged installer](104-add-voxi-install-command-for-complete-user-install-with-optional-privileged-setup.md), [114 out-of-checkout modifier helper](114-fix-out-of-checkout-modifierd-build-fallback.md)

---

## 1. Problem & Motivation

`make test-install-podman` now verifies the source Go, Make, and published curl paths on one Debian Bookworm-based image. Its `systemctl` and `sudo` commands are stubs. This proves command construction and installed files, but not service lifecycle behavior under a real manager or dependency portability across the Linux distributions Voxi targets.

## 2. Scope

- Add a bounded distribution matrix with at least one non-Debian base image relevant to the documented Linux/Wayland audience. Keep the same install-path assertions on each supported image, adapting package names and tools explicitly.
- Add an isolated real-systemd canary for user service loading, enablement, and activation. Test the optional system unit only where container privileges allow it safely; retain a separate stubbed path for permission and command assertions.
- Preserve the host safety boundary: never map host `/dev/uinput` or `/dev/input`, and do not claim that a stubbed `systemctl` proves live service health.
- Keep current-source and published-release results separate so a release canary cannot mask a source regression.

## 3. Acceptance Criteria

- [ ] The test output identifies each distribution, source or release provenance, and whether systemd was stubbed or real.
- [ ] A non-Debian image completes the user installation path with executable dependencies and valid unit files, or reports a specific supported-platform limitation.
- [ ] A real-manager canary verifies the user agent unit reaches the intended enabled/active state without host input devices.
- [ ] The architecture doc records the final supported matrix and remaining limits.
