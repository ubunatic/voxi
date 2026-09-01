# 036: Modularize Monitor Collector and TUI Renderer

**Status**: Complete — split into collector.go/render.go/monitor.go
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Code Quality / Refactor
**Related**: [internal/monitor/monitor.go](../internal/monitor/monitor.go), [internal/monitor/collector.go](../internal/monitor/collector.go), [internal/monitor/render.go](../internal/monitor/render.go), [internal/monitor/monitor_test.go](../internal/monitor/monitor_test.go)

---

## 1. Problem & Motivation

`harnez assess voxi` flags:
```
internal/monitor/monitor.go: High LOC file (>500 LOC, consider splitting/modularizing)
internal/monitor/monitor.go: High token weight (>4k tokens, agent context heavy)
```

At 1,020 lines, `internal/monitor/monitor.go` combines two separate responsibilities:
1. **Resource & telemetry collection**: Polling CPU/GPU metrics, probing active daemons, gathering ASR speeds, and managing monitoring state.
2. **Terminal layout and rendering**: ANSI box formatting, Braille graph generation, terminal sizing, and interactive key loop handlers.

## 2. Technical Specification

Decompose `internal/monitor/monitor.go` within `package monitor` into distinct files:

1. **`internal/monitor/collector.go`**:
   - Daemon state checking (`checkDaemonStatus`, `checkASRStatus`, `checkPiperStatus`, etc.)
   - Hardware resource polling (CPU usage, GPU utilization, VRAM usage)
   - Speed and latency sample accumulation
2. **`internal/monitor/render.go`**:
   - Box rendering (`drawSpeedBox`, `drawHardwareBox`, `drawTranscriptBox`, `drawDaemonBox`)
   - ANSI formatting and layout calculation
3. **`internal/monitor/monitor.go`**:
   - Core `RunMonitor` entry point, signal handling, tick loop, and `ResourceSections` definitions.

### Constraints
- Keep all types and functions within `package monitor` with identical signatures to preserve public API compatibility and existing tests in `monitor_test.go`.
- Avoid any visual or functional regression in `voxi monitor`.

## 3. Verification Plan

1. Run `go test ./internal/monitor/...` in `voxi`.
2. Run `harnez assess voxi` to confirm `internal/monitor/monitor.go` is decomposed below the 500 LOC warning threshold.
3. Run `make install` and test `voxi monitor` interactively.

## 4. Resolution Notes

- The spec's function names (`checkDaemonStatus`, `drawSpeedBox`, `RunMonitor`, etc.) did not match the actual codebase; the real collection/render/loop entry points are `CollectVoiceResources`, `PrintVoiceResourceReport`, and `RunWatchResources`. Split along the same collector/render/core-loop responsibility boundaries using the real names.
- Result: `monitor.go` 155 LOC, `collector.go` 395 LOC, `render.go` 492 LOC — all under the 500 LOC threshold. `go build ./...`, `go test ./...`, `gofmt -l`, `go vet` all clean; `make install` succeeded.
