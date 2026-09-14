# Publication & Release Management

Guidelines, architecture decisions, and release lifecycle for publishing and maintaining `voxi` on Codeberg.

---

## 1. Remote & Hosting Philosophy

- **Canonical Repository**: Codeberg (`ssh://git@codeberg.org/ubunatic/voxi.git`).
- **Solo/Hobby Model**: Work is done directly on `main` following [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/).
- **No Web Issue Triage**: Public bug reports, proposals, and design RFCs must be submitted as Markdown files in `issues/` via Pull/Merge Requests rather than web issue trackers.

---

## 2. CI & Build Verification Policy

- **Local Verification First**:
  Quality is maintained locally before pushing via:
  ```bash
  make check       # Runs go vet, go test ./..., and spec validation
  make build-all   # Builds voxi and voxi-modifierd
  ```
- **Codeberg CI Runners**:
  Codeberg does not provide free public shared runners for Forgejo Actions (unlike GitHub). Forgejo Actions require self-hosted runners or external CI (e.g. Woodpecker). Do not add `.forgejo/workflows` or `.github/workflows` unless a dedicated self-hosted runner is actively configured and online.

---

## 3. Public Release Checklist & Artifacts

When cutting or publishing a release:

1. **Licensing**: Ensure `LICENSE` (`AGPL-3.0-or-later`) is present in the repository root.
2. **Quality Gates**:
   - `make check` passes 100% (unit tests, vet, spec validation).
   - `make validate-spec` passes schema verification against `spec/schemas/`.
3. **Documentation**:
   - `README.md` is kept up to date with dependencies (`dotool`, `wl-clipboard`, `whisper.cpp`/Vulkan), quickstart steps, and GNOME extension instructions.
   - `CONTRIBUTING.md` documents coding standards and the in-repo Markdown issue submission workflow.
4. **Website Sync**:
   - Verify with `uman website doctor voxi`.
   - Publish latest updates with `uman website sync voxi`.
   - A CLI `harnez release` publishes binaries and tags; it does not sync
     `website/`. Keep website copy current locally, then sync it as a separate
     publishing step when requested. A `releases/latest` link follows new tags
     without a version-string edit.
5. **Git Tagging**:
   - Tag releases using semver (e.g. `v0.1.0`).

---

## 4. Release Pipeline: `harnez release`

`make release` runs `harnez release` — see the full, canonical pipeline
doc at `harnez/docs/practices/GoRelease.md` (that file, not this one, is
the source of truth for the mechanics; this section only records what's
specific to voxi's setup and issue 085's onboarding).

- **Version source of truth**: `version.yaml` at the repo root.
- **Go version var**: `version.go` at the repo root declares
  `package voxi` (**not** `package main` — voxi's `main` package lives in
  `cmd/voxi/`, so the version var has to live in a small importable
  library package, the same pattern harnez itself uses). `cmd/voxi/main.go`
  imports it as `ubunatic.com/voxi` and wires `voxi.Version` into the
  Cobra root command's `Version` field.
- **Build**: `.goreleaser.yaml` builds both `voxi` and `voxi-modifierd`
  for linux/amd64+arm64, with
  `-X ubunatic.com/voxi.Version={{.Version}}` ldflags (not
  `-X main.Version=...`, for the same reason as above).
- **Signing key**: a dedicated passwordless minisign key at
  `~/.minisign/voxi.key` (not the shared fallback key).
- **Commands**:
  ```bash
  make release                    # bumps patch, builds, signs, tags, pushes, publishes
  harnez release --dry-run        # preview without touching anything
  harnez release --continue       # resume after a publish failure (e.g. fj not yet authenticated) without re-bumping/re-tagging
  ```
- **`go.work` caveat**: if voxi is checked out alongside other modules
  under a shared `go.work` (local co-development), `harnez release`'s
  build step forces `GOWORK=off` by default so it never silently builds
  against an untagged local sibling checkout instead of the pinned
  dependency versions — see harnez issue 284.
