# 109 — Export Streaming Audio Sparkline and Continuous Time-Chart in `audiolevel`

**Status**: In Progress
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

- [ ] `audiolevel.RenderSparkline` is exported and produces identical 2x-resolution Braille sparklines as `internal/audio.RenderVolumeSparkline`.
- [ ] `audiolevel.NewSparklineStream` accepts streaming PCM frames and returns sliding-window Braille strings.
- [ ] External consumers (like `harnez`) can import `ubunatic.com/voxi/audiolevel` and display live scrolling speech waveforms.
- [ ] Unit tests verify time window sliding, zero-allocation rendering paths where applicable, and logarithmic scaling accuracy.
