# 133 — Bench crispasr (cohere-transcribe) alongside whisper models in voxi bench

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Performance
**Related**: 074 (Cohere backend), 131 (R2T2 evaluation needs a Cohere baseline)

---

## 1. Problem & Motivation

`voxi bench` only measures whisper-engine models through voxtype
(`internal/bench/bench.go:1-6`, the default filter is at `:140`). The default ASR engine,
`cohere-transcribe`, runs through the external `crispasr` binary and is not
benched, so its RTF cannot be compared against the other models.

The comparison matters now because:
- Issue 131 needs a Cohere baseline to judge R2T2.
- On 2026-09-22, `crispasr --diagnostics` on `~/.local/bin/crispasr` 0.8.32
  reported `ggml backends : cpu`, even though Cohere was assumed to run on the
  GPU. A bench row with a detected-backend column would make this visible.

## 2. /goal

`voxi bench` with no arguments includes the `cohere-transcribe` models in the
same results table as the whisper models. Each row shows the model name, the
requested backend (gpu/cpu), RTF, speedup, and the backend crispasr actually
reported. Failures, such as a missing binary or no GPU backend, appear as
error rows, as they do for whisper. The bench drives crispasr the same way
eager mode does (`crispASRTranscribeArgs`, `internal/eager/cohere.go`). Model
and engine values come from `spec/models.yaml` and are not duplicated in Go.
Tests cover the engine dispatch and crispasr's backend detection.

## 3. Notes / Uncertainties

- Re-verify the current `internal/bench` and `internal/eager/cohere.go`
  before starting; this ticket may lag the code.
- Unknown: how crispasr reports its active backend per run. Probe its output
  (canary-first) before writing the detection regex.
- Unknown: how crispasr selects GPU vs CPU (env var, flag, or build). If the
  installed build is CPU-only, the GPU row must say so rather than silently
  running on CPU.
