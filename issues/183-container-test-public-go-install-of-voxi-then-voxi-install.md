# 183 — Container test: public go install of voxi, then voxi install

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Infrastructure
**Related**: 116 (installer portability, real systemd), 114, 104 (voxi install), 181 (voxi may later install lmcoder the same way), `test/install/`, `scripts/test-install-podman.sh`

---

/goal A Podman test installs the published voxi with `go install ubunatic.com/voxi/cmd/voxi@latest`
in a clean container, without the local checkout, then runs `voxi install` and checks the result.
Stop and report when blocked on a user decision, a denied permission, or a failure in the published
release that needs a new release to fix.

## 1. Problem & Motivation
`make test-install-podman` (`scripts/test-install-podman.sh`, `test/install/`) covers two paths:
`go install` from the bind-mounted local checkout, and the curl installer for the published release.
Nobody tests what a new user types: `go install ubunatic.com/voxi/cmd/voxi@latest`. That path
depends on the ubunatic.com vanity tag, the Codeberg repo, the latest release tag and its `go.mod`.
On 2026-10-06 the same path was broken for lmcoder (old tag with old module name) and only a manual
`go install` noticed.

## 2. Findings (2026-10-06)
- Module `ubunatic.com/voxi`, vanity tag live (go-import → codeberg.org/ubunatic/voxi), latest tag v0.1.13.
- The existing image `test/install/Containerfile` (golang:1.26-bookworm, test user, systemd and
  sudo stubbed by `test/install/test-go-install.sh`) can probably be reused; only the install source
  changes (no `/src` mount, `@latest`).
- The existing test's `voxi install` checks (binaries, user units, `systemctl` calls) apply as they are.

## 3. Plan
- Add the public path as its own test step or script, reported separately from the local-source
  result so one cannot hide the other (see 116). Optional: allow a version override (`@vX.Y.Z`)
  for checking a release right after tagging.
- Needs network; keep it out of offline/unit test targets.
- Verify: `make test-install-podman` (or a new target) passes against the current release.
