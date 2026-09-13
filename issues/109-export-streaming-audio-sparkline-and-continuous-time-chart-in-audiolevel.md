# 109 — Export Streaming Audio Sparkline and Continuous Time-Chart in `audiolevel`

**Status**: Closed — implemented and verified: audiolevel.RenderSparkline, SparklineStream, Meter/Manager integration, tests passing
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Feature
**Related**: `audiolevel/audiolevel.go`, `internal/audio/audio.go`, `internal/chunks/command.go`, `docs/LiveMicMeter.md`, `docs/ChunkDiagnostics.md`

---

## 1. Problem & Motivation

The `ubunatic.com/voxi/audiolevel` package is Voxi's exported public Go library for live microphone capture and audio level measurement, shared between Voxi and external consumers (such as `harnez`).

Currently, `audiolevel` only exposes scalar amplitude readings (`Reading.Level` across `[0.0, 100.0]`). In contrast, Voxi's 2x-resolution packed Braille volume sparkline generator (`RenderVolumeSparkline`) is unexported in `internal/audio/` and only operates on finalized, discrete audio chunk slices.

External tools (and Voxi's own live monitor) want to display not just an instantaneous scalar volume bar, but a continuous, real-time scrolling speech time-chart/waveform (e.g. `[⣄⣶⣶⣦⣴⣤⣴⣤⣠⣴]`) as speech is progressing.

---

## 2. Technical Specification

### 2.1 Static Sparkline Primitives in `audiolevel`
Export standalone Braille sparkline generation:
```go
// RenderSparkline generates a 2x-resolution Braille sparkline from raw PCM16LE audio.
func RenderSparkline(pcmData []byte, width int) string
```
- Uses dual-column packed Braille glyphs ($U+2800..U+28FF$) providing 2 data points per terminal character.
- Logarithmic RMS energy quantization with calibrated conversational speech floor and ceiling.

### 2.2 Streaming Rolling Waveform (`SparklineStream`)
Provide a thread-safe, sliding-window streaming accumulator:
```go
type SparklineOptions struct {
    Width        int           // Number of Braille characters (default: 10)
    SampleRate   int           // Capture sample rate (default: 8000)
    Window       time.Duration // Time span displayed across the width (default: 3s)
    FloorRMS     int           // Noise floor cutoff
    CeilingRMS   int           // Peak scale ceiling
}

type SparklineStream struct {
    // contains filtered or unexported fields
}

func NewSparklineStream(opts SparklineOptions) *SparklineStream
func (s *SparklineStream) WritePCM(pcm []byte)
func (s *SparklineStream) Sparkline() string
```

### 2.3 Integration with `audiolevel.Meter`
Allow `audiolevel.Meter` to optionally maintain and update a rolling `SparklineStream` during live capture loops without additional subprocess overhead.

---

## 3. Implementation Plan

1. Export the 2x packed Braille glyph math and RMS quantizer from `internal/audio` into `audiolevel/sparkline.go`.
2. Implement `SparklineStream` circular sample buffer that updates as incoming audio frames arrive and renders the rolling time chart.
3. Add optional sparkline integration to `audiolevel.Meter`.
4. Add comprehensive unit tests and benchmarks in `audiolevel/sparkline_test.go`.

---

## 4. Acceptance Criteria

- [x] `audiolevel.RenderSparkline` is exported and produces identical 2x-resolution Braille sparklines as `internal/audio.RenderVolumeSparkline`.
- [x] `audiolevel.NewSparklineStream` accepts streaming PCM frames and returns sliding-window Braille strings.
- [x] External consumers (like `harnez`) can import `ubunatic.com/voxi/audiolevel` and display live scrolling speech waveforms.
- [x] Unit tests verify time window sliding, zero-allocation rendering paths where applicable, and logarithmic scaling accuracy.

---

## 5. Resolution

Implemented in `audiolevel/sparkline.go`:
- `RenderSparkline(pcmData []byte, width int) string` — standalone Braille
  sparkline, duplicating `internal/audio.RenderVolumeSparkline`'s glyph/RMS
  math (not calling it directly — `audiolevel` already imports
  `internal/audio` for `ComputeAudioRMS`, so the reverse import would cycle).
  Verified byte-identical to `internal/audio.RenderVolumeSparkline` across
  silence/quiet/loud/short/odd-length inputs and multiple widths.
- `SparklineOptions` / `SparklineStream` (`NewSparklineStream`, `WritePCM`,
  `Sparkline`) — thread-safe sliding-window PCM accumulator capped at
  `SampleRate * Window` bytes; old samples evict as new ones arrive.
- `Meter.EnableSparkline` / `Meter.WritePCM` / `Meter.Sparkline`, with
  `Manager` passthroughs (nil-safe) — `RunCapture`'s existing read loop now
  also feeds each chunk into the meter's `SparklineStream` when enabled, no
  extra subprocess or capture stream.

Tests added in `audiolevel/sparkline_test.go`: equivalence against
`internal/audio.RenderVolumeSparkline`, default-option filling, sliding-
window eviction, live-audio reflection, `Meter`/`Manager` wiring and nil
safety, plus allocation-reporting benchmarks (`go test -bench` with
`-benchmem`). Full repo `go build ./...`, `go vet ./...`, and `go test ./...`
pass; `make install` run per project convention (no live-daemon code path
touched, so no `make restart-service` needed).
