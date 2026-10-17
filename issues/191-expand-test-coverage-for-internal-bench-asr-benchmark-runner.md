# 191 — Expand test coverage for internal/bench ASR benchmark runner

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Code Quality / Test Coverage
**Related**: 037, 133, 176

---

## 1. Problem & Motivation
A test coverage audit (`go test -cover ./...`) identified that `internal/bench` has only 27.4% statement coverage.
`internal/bench` orchestrates offline transcription benchmarking across models (Whisper, Parakeet, Cohere, etc.),
calculating RTF, word error rates, and output formats. Major code paths including multi-model comparisons, error handling,
and output formatters currently lack automated test assertions.

## 2. Technical Specification / Findings
- `internal/bench/bench.go` defines benchmark runners, result aggregation, and metric calculations.
- Existing tests in `internal/bench/bench_test.go` only test basic fixture parsing and simple execution paths.
- Code paths for metric calculations (WER/CER, RTF bounds), CLI table formatting, error handling on corrupt WAV fixtures,
  and mock ASR runner dispatch remain untested.

## 3. Implementation & Verification Plan
- Add unit tests for RTF calculation, metric aggregation, and report rendering with synthetic benchmark results.
- Add test coverage for runner error paths (missing/unsupported models, invalid sample directories).
- Verify via `go test -cover ./internal/bench/...` to raise statement coverage to $\ge 70\%$.
