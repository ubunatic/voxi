# 192 — GitHub Actions CI and local container test workflow

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Infrastructure / CI
**Related**: 183, 037, .github/workflows/ci.yaml, Containerfile, Makefile

---

## 1. Problem & Motivation
The project previously relied solely on local host verification and installation container tests (`test/install/`).
A standardized GitHub Actions CI pipeline and reproducible local containerized test runner (`make container-test`)
were needed to ensure code formatting, vetting, building, and race-detection pass reliably across clean environments.

## 2. Technical Specification / Findings
- **GitHub Actions Workflow** (`.github/workflows/ci.yaml`):
  - Checkout with Git LFS enabled (`lfs: true`) to pull audio fixtures (`.wav`, `.flac`).
  - `actions/setup-go@v5` reading Go version from `go.mod` with dependency caching enabled.
  - Concurrency group cancelling superseded runs on the same branch/ref.
  - Verification steps: `gofmt` check, `go vet ./...`, `go build ./...`, `go test -race -v ./...`, `go test ./spec/...`.
- **Reproducible Container Test** (`Containerfile`, `make container-test`):
  - Root `Containerfile` based on `golang:1.26-bookworm` with system dependencies (`libevdev2`, `libxkbcommon-dev`, `git`, `gcc`).
  - Pre-downloads Go modules during container build for fast, offline-capable execution.
  - Makefile target `container-test` auto-detects `podman` or `docker` via `ENGINE ?= $(shell command -v podman || command -v docker)`.
  - Mounts repository read-only (`/src:ro`) and only a dedicated output directory writable (`.test-output:/output:rw`).
- **Tests Skipping Cleanly in Containers (`VOXI_CONTAINER_TEST=1`)**:
  1. `internal/eager/live_asr_test.go`: `TestLiveASR_R2T2Corpus` (needs live network connection to llama-server).
  2. `internal/eager/cleanup_eval_test.go`: `TestRealCleanupEvaluation` (needs live network connection to LLM cleanup endpoint).
  3. `internal/eager/e2e_pipeline_test.go`: `TestRealE2EPipeline` (needs voxtype/crispasr binaries and private audio corpus WAV fixtures).
  4. `internal/tts/mpris_test.go`: `TestMPRISControl` (needs active graphical/session D-Bus daemon).
  5. `internal/tts/process_test.go`: `TestSupervisorParentDeathKillsAndReapsGrandchild` (needs setsid / process-tree control).
  6. `scripts/canary_nested`: (needs live graphical Wayland/GNOME session).

## 3. Implementation & Verification Plan
- Verified `make container-test` runs with podman and passes completely green (`go vet`, `go build`, `go test -race`, `gofmt`).
- Verified `.github/workflows/ci.yaml` parses validly.
- Verified test suite and spec tests pass cleanly on host.
