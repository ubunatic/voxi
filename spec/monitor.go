// Package spec also loads spec/monitor.yaml: `voxi monitor -w`'s screen
// redraw rate and its live mic loudness meter's sampling/ballistics. It is
// the single source of truth for that tuning — see docs/Spec.md.
// Application code must not hardcode values that duplicate or shadow it.
package spec

import (
	_ "embed"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed monitor.yaml
var monitorYAML []byte

// PaintSpec is the monitor TUI's screen redraw cadence.
type PaintSpec struct {
	FPS float64 `yaml:"fps"`
}

// MicLevelSpec is the live mic-level meter's capture rate/chunking and
// VU-meter ballistics.
type MicLevelSpec struct {
	SampleRateHz int     `yaml:"sample_rate_hz"`
	ChunkMs      float64 `yaml:"chunk_ms"`
	WindowMs     float64 `yaml:"window_ms"`
	DecayMs      float64 `yaml:"decay_ms"`
}

// MonitorSpec is the parsed contents of spec/monitor.yaml.
type MonitorSpec struct {
	Paint    PaintSpec    `yaml:"paint"`
	MicLevel MicLevelSpec `yaml:"mic_level"`
}

// LoadMonitor parses the embedded monitor tuning spec.
func LoadMonitor() (*MonitorSpec, error) {
	return parseMonitorSpec(monitorYAML)
}

// parseMonitorSpec unmarshals and validates a monitor.yaml document. Split
// out from LoadMonitor so tests can exercise validation against ad hoc YAML
// without touching the embedded spec.
func parseMonitorSpec(data []byte) (*MonitorSpec, error) {
	var s MonitorSpec
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("spec: parse monitor.yaml: %w", err)
	}
	if s.Paint.FPS <= 0 {
		return nil, fmt.Errorf("spec: paint.fps must be positive")
	}
	if s.MicLevel.SampleRateHz <= 0 {
		return nil, fmt.Errorf("spec: mic_level.sample_rate_hz must be positive")
	}
	if s.MicLevel.ChunkMs <= 0 {
		return nil, fmt.Errorf("spec: mic_level.chunk_ms must be positive")
	}
	if s.MicLevel.WindowMs <= 0 {
		return nil, fmt.Errorf("spec: mic_level.window_ms must be positive")
	}
	if s.MicLevel.DecayMs <= 0 {
		return nil, fmt.Errorf("spec: mic_level.decay_ms must be positive")
	}
	return &s, nil
}

// PaintInterval converts Paint.FPS to a redraw ticker interval.
func (s *MonitorSpec) PaintInterval() time.Duration {
	return time.Duration(float64(time.Second) / s.Paint.FPS)
}

// ChunkBytes returns the byte length of one raw capture chunk at
// MicLevel.SampleRateHz/ChunkMs, for 16-bit little-endian mono PCM (2 bytes
// per sample).
func (s *MonitorSpec) ChunkBytes() int {
	return int(float64(s.MicLevel.SampleRateHz) * s.MicLevel.ChunkMs / 1000.0 * 2)
}

// Window converts MicLevel.WindowMs to a time.Duration.
func (s *MonitorSpec) Window() time.Duration {
	return time.Duration(s.MicLevel.WindowMs * float64(time.Millisecond))
}

// Decay converts MicLevel.DecayMs to a time.Duration.
func (s *MonitorSpec) Decay() time.Duration {
	return time.Duration(s.MicLevel.DecayMs * float64(time.Millisecond))
}
