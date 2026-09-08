package spec

import (
	"math"
	"testing"
)

func TestLoadMonitor(t *testing.T) {
	s, err := LoadMonitor()
	if err != nil {
		t.Fatalf("LoadMonitor() error = %v", err)
	}
	if s.Paint.FPS <= 0 {
		t.Errorf("Paint.FPS = %v, want positive", s.Paint.FPS)
	}
	if s.MicLevel.SampleRateHz <= 0 {
		t.Errorf("MicLevel.SampleRateHz = %v, want positive", s.MicLevel.SampleRateHz)
	}
}

func TestMonitorDerivedValues(t *testing.T) {
	s, err := LoadMonitor()
	if err != nil {
		t.Fatalf("LoadMonitor() error = %v", err)
	}
	// time.Duration rounds to nanoseconds, so compare with tolerance rather than
	// exact float equality (e.g. 1/30s is not exactly representable either way).
	if got, want := s.PaintInterval().Seconds(), 1.0/s.Paint.FPS; math.Abs(got-want) > 1e-9 {
		t.Errorf("PaintInterval() = %v, want %v", got, want)
	}
	wantChunkBytes := int(float64(s.MicLevel.SampleRateHz) * s.MicLevel.ChunkMs / 1000.0 * 2)
	if got := s.ChunkBytes(); got != wantChunkBytes {
		t.Errorf("ChunkBytes() = %d, want %d", got, wantChunkBytes)
	}
}

func TestParseMonitorSpecRejectsNonPositiveValues(t *testing.T) {
	cases := []string{
		`paint: {fps: 0}
mic_level: {sample_rate_hz: 8000, chunk_ms: 50, window_ms: 150, attack_ms: 30, decay_ms: 250}`,
		`paint: {fps: 12.5}
mic_level: {sample_rate_hz: 0, chunk_ms: 50, window_ms: 150, attack_ms: 30, decay_ms: 250}`,
		`paint: {fps: 12.5}
mic_level: {sample_rate_hz: 8000, chunk_ms: 0, window_ms: 150, attack_ms: 30, decay_ms: 250}`,
		`paint: {fps: 12.5}
mic_level: {sample_rate_hz: 8000, chunk_ms: 50, window_ms: 0, attack_ms: 30, decay_ms: 250}`,
		`paint: {fps: 12.5}
mic_level: {sample_rate_hz: 8000, chunk_ms: 50, window_ms: 150, attack_ms: 0, decay_ms: 250}`,
		`paint: {fps: 12.5}
mic_level: {sample_rate_hz: 8000, chunk_ms: 50, window_ms: 150, attack_ms: 30, decay_ms: 0}`,
	}
	for _, doc := range cases {
		if _, err := parseMonitorSpec([]byte(doc)); err == nil {
			t.Errorf("parseMonitorSpec(%q) expected an error, got nil", doc)
		}
	}
}
