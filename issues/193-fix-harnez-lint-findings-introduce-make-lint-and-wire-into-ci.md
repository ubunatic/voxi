# 193 — Fix harnez lint findings, introduce make lint, and wire into CI

**Status**: Closed — Resolved by 2a4bde6
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Quality
**Related**: 037

---

## 1. Problem & Motivation
The repository currently reports 74 harnez lint findings across shell scripts (`scripts/install.sh`, `scripts/test-e2e-headless.sh`, `scripts/test-install-podman.sh`, `scripts/canary-gnome-shortcut.sh`, and `website/install.sh`). Furthermore, the project lacks a standardized `make lint` target executing `gofmt` check, `go vet`, and `harnez lint --check` across tracked files, and CI does not enforce harnez linting.

## 2. Technical Specification / Findings
- Harnez lint checks Bash scripts against `@docs/Bash.md`: no semicolons before `then`/`do`, line break before `then`/`do`/`done`, and `if test ...` instead of `[` or `[[`.
- `website/install.sh` is generated from `scripts/install.sh` via `make website-install-sh`.
- Makefile target `lint` must check formatting (`gofmt -l`), run `go vet ./...`, and run `harnez lint --check` on tracked `.go`, `.sh`, `.md`, and `Makefile` files.
- GitHub CI (`.github/workflows/ci.yaml`) must invoke `make lint`.

## 3. Implementation & Verification Plan
- Refactor Bash conditionals and semicolons in `scripts/*.sh` and `website/install.sh` to comply with Harnez Bash guidelines.
- Add `lint: ⚙️` in `Makefile`.
- Wire `make lint` into `.github/workflows/ci.yaml`.
- Verify with `make lint` (0 findings).
