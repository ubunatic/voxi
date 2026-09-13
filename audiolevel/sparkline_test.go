package audiolevel

import (
	"math"
	"strings"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/audio"
)

// sparklineSine generates n samples of a sine wave at the given amplitude,
// matching pcm16LE's little-endian encoding (see audiolevel_test.go).
func sparklineSine(n int, amplitude int16, period float64) []byte {
	samples := make([]int16, n)
	for i := range samples {
		samples[i] = int16(float64(amplitude) * math.Sin(2*math.Pi*float64(i)/period))
	}
	return pcm16LE(samples)
}

func TestRenderSparkline_MatchesInternalAudio(t *testing.T) {
	cases := []struct {
		name  string
		pcm   []byte
		width int
	}{
		{"empty", nil, 10},
		{"silence", pcm16LE(make([]int16, 1600)), 10},
		{"quiet sine", sparklineSine(1600, 500, 40), 10},
		{"loud sine", sparklineSine(1600, 20000, 40), 10},
		{"short buffer", sparklineSine(37, 20000, 10), 10},
		{"single byte", []byte{0x01}, 10},
		{"default width", sparklineSine(1600, 5000, 40), 0},
		{"wide", sparklineSine(4000, 5000, 40), 24},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := RenderSparkline(c.pcm, c.width)
			want := audio.RenderVolumeSparkline(c.pcm, c.width)
			if got != want {
				t.Errorf("RenderSparkline(%q, %d) = %q, want %q (internal/audio.RenderVolumeSparkline)", c.name, c.width, got, want)
			}
		})
	}
}

func TestRenderSparkline_DefaultWidth(t *testing.T) {
	got := RenderSparkline(sparklineSine(1600, 5000, 40), 0)
	if got == "" || len([]rune(got)) != 10 {
		t.Errorf("RenderSparkline width=0 = %q, want 10 runes (default)", got)
	}
}

func TestNewSparklineStream_Defaults(t *testing.T) {
	s := NewSparklineStream(SparklineOptions{})
	if s.opts.Width != 10 {
		t.Errorf("default Width = %d, want 10", s.opts.Width)
	}
	if s.opts.SampleRate != DefaultSampleRate {
		t.Errorf("default SampleRate = %d, want %d", s.opts.SampleRate, DefaultSampleRate)
	}
	if s.opts.Window != 3*time.Second {
		t.Errorf("default Window = %v, want 3s", s.opts.Window)
	}
	if s.opts.FloorRMS != sparklineFloorRMS {
		t.Errorf("default FloorRMS = %d, want %d", s.opts.FloorRMS, sparklineFloorRMS)
	}
	if s.opts.CeilingRMS != sparklineCeilingRMS {
		t.Errorf("default CeilingRMS = %d, want %d", s.opts.CeilingRMS, sparklineCeilingRMS)
	}
}

func TestSparklineStream_EmptyRendersSpaces(t *testing.T) {
	s := NewSparklineStream(SparklineOptions{Width: 5})
	got := s.Sparkline()
	if got != strings.Repeat(" ", 5) {
		t.Errorf("empty stream Sparkline() = %q, want 5 spaces", got)
	}
}

func TestSparklineStream_SlidingWindowEvicts(t *testing.T) {
	// 100 samples/sec, 1s window -> 100 samples -> 200 bytes max.
	s := NewSparklineStream(SparklineOptions{Width: 10, SampleRate: 100, Window: time.Second})
	if s.maxBytes != 200 {
		t.Fatalf("maxBytes = %d, want 200", s.maxBytes)
	}

	loud := sparklineSine(50, 20000, 10)
	s.WritePCM(loud)
	if len(s.buf) != len(loud) {
		t.Fatalf("buf len after first write = %d, want %d", len(s.buf), len(loud))
	}

	// Push enough additional silence to fully evict the loud samples from
	// the window.
	silence := pcm16LE(make([]int16, 400))
	s.WritePCM(silence)
	if len(s.buf) != s.maxBytes {
		t.Fatalf("buf len after eviction = %d, want maxBytes=%d", len(s.buf), s.maxBytes)
	}

	got := s.Sparkline()
	if strings.ContainsAny(got, "⣶⣤⣦⣄") {
		t.Errorf("Sparkline() after loud samples evicted = %q, want only quiet glyphs/spaces (loud audio should have scrolled out of the window)", got)
	}
}

func TestSparklineStream_ReflectsRecentAudio(t *testing.T) {
	s := NewSparklineStream(SparklineOptions{Width: 10, SampleRate: 8000, Window: 200 * time.Millisecond})

	quiet := s.Sparkline()
	s.WritePCM(sparklineSine(1600, 20000, 40))
	loud := s.Sparkline()

	if quiet == loud {
		t.Errorf("Sparkline() unchanged after writing loud PCM: %q", loud)
	}
}

func TestMeter_SparklineDisabledByDefault(t *testing.T) {
	var m Meter
	if got := m.Sparkline(); got != "" {
		t.Errorf("Sparkline() before EnableSparkline = %q, want empty", got)
	}
	m.WritePCM(sparklineSine(1600, 20000, 40)) // must not panic
}

func TestMeter_EnableSparklineFeedsFromWritePCM(t *testing.T) {
	var m Meter
	m.EnableSparkline(SparklineOptions{Width: 10, SampleRate: 8000, Window: 200 * time.Millisecond})

	before := m.Sparkline()
	m.WritePCM(sparklineSine(1600, 20000, 40))
	after := m.Sparkline()

	if before == after {
		t.Errorf("Sparkline() unchanged after WritePCM with loud audio: %q", after)
	}
}

func TestManager_SparklinePassthroughNilSafe(t *testing.T) {
	var m *Manager
	m.EnableSparkline(SparklineOptions{}) // must not panic
	if got := m.Sparkline(); got != "" {
		t.Errorf("nil Manager Sparkline() = %q, want empty", got)
	}
}

func BenchmarkRenderSparkline(b *testing.B) {
	pcm := sparklineSine(1600, 8000, 40)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RenderSparkline(pcm, 10)
	}
}

func BenchmarkSparklineStream_WritePCMAndRender(b *testing.B) {
	s := NewSparklineStream(SparklineOptions{Width: 10, SampleRate: 8000, Window: 3 * time.Second})
	chunk := sparklineSine(400, 8000, 40) // ~50ms at 8kHz
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.WritePCM(chunk)
		s.Sparkline()
	}
}
