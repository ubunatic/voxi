# 133 — Bench Cohere Transcribe via crispasr alongside whisper models in voxi bench

**Status**: Closed — Added crispasr Cohere Transcribe model to voxi bench with non-aborting multi-engine dispatch, build/run backend detection, and passing tests
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

`voxi bench` with no arguments includes the Cohere Transcribe model, run through
crispasr (e.g. row `cohere-transcribe`, detected `crispasr:cpu`), in the
same results table as the whisper models. Each row shows the model name, the
requested backend (gpu/cpu), RTF, speedup, and the backend crispasr actually
reported. Failures, such as a missing binary or no GPU backend, appear as
error rows, as they do for whisper. The bench drives crispasr the same way
eager mode does (`crispASRTranscribeArgs`, `internal/eager/cohere.go`). Model
and engine values come from `spec/models.yaml` and are not duplicated in Go.
Tests cover the engine dispatch and crispasr's backend detection.

## 3. Refined Architecture & Milestones (by opus:high)

### Scope Boundaries
- **Files to touch**:
  - `internal/crispasr/crispasr.go` (new) + `_test.go` + `testdata/`: Extract shared crispasr CLI invocation and weight management from eager. Add backend detection (`ParseBuildBackends`, `DetectRunBackend`).
  - `internal/eager/cohere.go`, `cohere_test.go`, `transcribe_sample.go`: Delegate to `internal/crispasr`.
  - `spec/models.go`: Add `Engine(name) string` and engine-name constants next to `IsWhisperEngine`.
  - `internal/bench/bench.go`, `bench_test.go`: Multi-engine dispatch; include whisper + cohere-transcribe by default; per-row missing binary errors (missing voxtype no longer aborts whole run).
  - `cmd/voxi/main.go`: Sizing model column dynamically / wide enough for `cohere-transcribe-03-2026`, update help text.
  - `docs/BenchBaseline.md`: Update baseline with Cohere rows.
- **Files NOT to touch**:
  - `spec/models.yaml` (keep schema as-is)
  - `openai-transcribe` models (excluded from default run)

### Sequential Milestones
- **M1 – Canary Probes & Testdata**:
  - Capture `crispasr --version` and `--diagnostics` outputs as committed fixtures in `internal/crispasr/testdata/`.
  - Probe GPU flag behavior (`-ng`, `--gpu-backend vulkan`) on the installed build. Determine exact backend detection regex.
- **M2 – Extract `internal/crispasr`**:
  - Extract `crispASRTranscribeArgs`, weight downloading/locating from `internal/eager/cohere.go` into `internal/crispasr`.
  - Update `internal/eager` to delegate to `internal/crispasr` with zero regression. Add engine spec helpers in `spec/models.go`.
- **M3 – Backend Detection Logic**:
  - Implement `ParseBuildBackends` and `DetectRunBackend` with table-driven tests against fixtures.
- **M4 – Bench Multi-Engine Dispatch**:
  - Update `internal/bench` to bench both whisper and crispasr engines.
  - Turn missing binary or missing GPU backend into per-row error entries without crashing the benchmark.
- **M5 – CLI Display, Verification & Re-baseline**:
  - Format bench output column width, update docs, verify via `make test-q1`, run `make restart-service`.

### Edge Cases & Negative Tests
- Missing `crispasr` binary -> reports error row, does not abort Whisper benchmarks.
- Missing `voxtype` binary -> reports error row, does not abort Cohere benchmarks.
- CPU-only `crispasr` build -> GPU bench row explicitly reports missing GPU backend error, never silently runs CPU.
- Non-zero exit code -> reports first error line in error column.

