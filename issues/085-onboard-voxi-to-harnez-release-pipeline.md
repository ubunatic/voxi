# 085: Onboard voxi to harnez release pipeline

**Status**: Closed — onboarded and verified live (v0.1.1 released)
**Priority**: P2 (Medium)
**Severity**: Enhancement
**Category**: Tooling
**Related**: [harnez issue 091 language-agnostic release spec](../../harnez/issues/091-language-agnostic-release-spec-and-thin-make-release.md), [harnez docs/practices/GoRelease.md](../../harnez/docs/practices/GoRelease.md), [PublicationAndRelease.md](../docs/PublicationAndRelease.md), [uman issue 024 deprecate uman release](../../uman/issues/024-deprecate-uman-release-in-favor-of-harnez-release.md)

---

## 1. Problem & Motivation

`make release` was a stub: it printed instructions to run `uman release`
manually and had no `version.yaml`, `version.go`-equivalent, or
`.goreleaser.yaml` — voxi could not actually be released. Meanwhile the
workspace had already standardized on `harnez release` (version.yaml spec,
language-agnostic version sync, forge auto-enable, minisign signing) per
harnez issue 091; `spriteview` was already using it, but voxi and `webman`
were still stuck on the old `uman release` stub/echo pattern.

## 2. Resolution

- Added `version.yaml` (single source of truth) and a root-level `voxi`
  package (`version.go`, `var Version = "0.1.0"`) — voxi's `main` package
  lives in `cmd/voxi/`, not the repo root, so the version var had to be a
  small importable library package (`ubunatic.com/voxi`), mirroring how
  harnez itself does it (`package harnez`, imported as `harnez.Version`
  into `cmd/harnez`). A naive `package main` at the repo root does not
  build — nothing can import a stray root-level `main` package.
- Added `.goreleaser.yaml` building both `voxi` and `voxi-modifierd`
  (linux/amd64+arm64), with `-X ubunatic.com/voxi.Version={{.Version}}`
  ldflags matching the library-package pattern above (`-X main.Version=...`
  would be wrong here).
- Wired `voxi.Version` into the Cobra root command's `Version` field.
- Generated a dedicated passwordless minisign key
  (`~/.minisign/voxi.key`, `minisign -G -W -f ...`) rather than reusing the
  shared fallback key.
- Switched `make release` to `release: check ⚙️ / harnez release`, matching
  `docs/practices/GoRelease.md` §2.5's thin-Makefile convention.

## 3. Verification

- `harnez release --dry-run` — clean plan (only a benign, documented
  non-fatal 401 on the forge `has_releases` preflight check from an
  unrelated expired token; see harnez `docs/practices/GoRelease.md` §3).
- `make check` (vet, tests, spec validation) — all green.
- Live release: `harnez release` actually cut and published **v0.1.1** to
  Codeberg (build, minisign sign, tag, push, `fj` publish). One real
  hiccup along the way: `fj` was freshly installed but not yet
  authenticated (`fj auth login`), which failed only the final publish
  step non-destructively — `harnez release --continue` after login
  finished cleanly with no re-bump/re-tag. Confirms the `--continue`
  recovery path documented in GoRelease.md §4 works as described.

## 4. Follow-ups

- `webman/Makefile` still has the identical stale `uman release` stub —
  same fix applies, intentionally left out of this pass (see the session
  this ticket was filed from).
