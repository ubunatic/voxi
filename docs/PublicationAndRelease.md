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
5. **Git Tagging**:
   - Tag releases using semver (e.g. `v0.1.0`).
