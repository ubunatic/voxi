# 212 — bench test TestRunCrispasrGPUSuccessWithDetectedBackend hangs until 10m timeout

**Status**: Open
**Priority**: P1 (High)
**Severity**: Major
**Category**: Bug
**Related**: 133, 211

---

/goal The test runs offline and finishes in under a second, and `make test-q1` is green again; or stop and report when blocked on a user decision.

## 1. Problem & Motivation
`make test-q1` on 2026-11-02 (issue 211 sprint, /tmp/test-q1.log) failed: `internal/bench` panicked at the 10m test timeout. The test stuck was `TestRunCrispasrGPUSuccessWithDetectedBackend` (internal/bench, ~line 660), running for 4m30s. The test package was not touched by 211.

## 2. Findings
The goroutine dump shows an HTTP/2 response body being copied (`io.Copy` ← `http2.transportResponseBody.Read`): the test downloads something real from the network. Its fake weights file (1 KiB `cohere-transcribe-q5_0.gguf` under a temp XDG_CACHE_HOME) apparently doesn't satisfy the runner, so the runner fetches real model weights.

## 3. Plan
- Find what the runner downloads and why the fake cache isn't accepted; make the test hermetic (inject the downloader, or fake a valid cache entry) and fail fast instead of downloading in tests.
- Check other bench tests for the same pattern.

## Decision
Lean sprint 2026-11-02 (dev d212 haiku:med, reviewer opus:med); host decisions on the plan:
1. Cause: cohere weights under the 1 GiB `GGUFMinBytes` floor are treated as missing, so the runner downloads real weights. Fix with sparse fixtures at the floor (same pattern as eager), not a lowered production floor.
2. Inject the HTTP client via `deps` (`HTTPClient`); bench tests use a guard client that counts and fails requests.
3. A bench-package `TestMain` backstop transport that fails any real network request is accepted.
