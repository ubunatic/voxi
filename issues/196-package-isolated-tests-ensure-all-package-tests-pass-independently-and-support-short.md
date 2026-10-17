# 196 — Package-isolated tests: ensure all package tests pass independently and support -short

**Status**: Closed — Resolved by 5e84c30
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Quality
**Related**: 037

---

## 1. Problem & Motivation
Packages should be individually testable via `go test ./internal/<pkg>/ -count=1 -race` without hidden inter-package or host environment dependencies (e.g. relying on specific `$HOME`, `$XDG_*`, or unmocked tools). In addition, `go test -short` must provide a fast unit testing tier that skips heavy external integrations, PTYs, or long-running audio waits.

## 2. Technical Specification / Findings
- Iterate through all packages in `./internal/...` and `./cmd/...` to ensure `go test -count=1 -race` succeeds in isolation.
- Inject dependencies or stub environment variables where tests assume ambient system state.
- Support `testing.Short()` in heavy integration/stress tests to enable fast unit test runs via `make test-short`.

## 3. Implementation & Verification Plan
- Run `go test -race -count=1` across each package directory individually.
- Fix any failing package tests.
- Verify `make test-short` and `make test-pkg`.
- Verify `make container-test`.
