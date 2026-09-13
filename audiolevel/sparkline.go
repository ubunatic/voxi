package audiolevel

import (
	"math"
	"strings"
	"sync"
	"time"

	"ubunatic.com/voxi/internal/audio"
)

// sparklineFloorRMS, sparklineCeilingRMS, sparklineMinLevel and
// sparklineLevels mirror internal/audio's tuned constants of the same name
// (see internal/audio/audio.go for the full calibration rationale) so
// RenderSparkline produces byte-identical output to
// internal/audio.RenderVolumeSparkline for the same input. They're
// duplicated rather than imported because internal/audio.ComputeAudioRMS is
// the only piece this package actually shares with internal/audio — pulling
// the rest across the package boundary would require internal/audio to
// depend back on audiolevel (a cycle), since audiolevel already imports
// internal/audio.
const (
	sparklineFloorRMS   = 80
	sparklineCeilingRMS = 8192
	sparklineMinLevel   = 1
	sparklineLevels     = 4
)

// RenderSparkline generates a fixed-width, 2x-resolution Braille sparkline
// (U+2800..U+28FF) from raw PCM16LE mono audio, identical in algorithm and
// output to internal/audio.RenderVolumeSparkline: the buffer is split into
// 2*width equal time slices, each slice's RMS is quantized onto a
// logarithmic scale between sparklineFloorRMS and sparklineCeilingRMS, and
// each consecutive pair of slices is packed into one Braille cell (left
// dot-column = earlier slice, right dot-column = later slice). A slice with
// no data (buffer too short) renders as a literal space, distinct from a
// measured-but-silent slice (the quietest glyph, "⣀").
func RenderSparkline(pcmData []byte, width int) string {
	if width <= 0 {
		width = 10
	}
	if len(pcmData) < 2 {
		return strings.Repeat(" ", width)
	}

	subBuckets := width * 2
	totalSamples := len(pcmData) / 2
	samplesPerBucket := totalSamples / subBuckets
	if samplesPerBucket < 1 {
		samplesPerBucket = 1
	}

	subLevel := func(b int) int {
		start := b * samplesPerBucket * 2
		if start >= len(pcmData) {
			return 0
		}
		end := start + samplesPerBucket*2
		if b == subBuckets-1 || end > len(pcmData) {
			end = len(pcmData)
		}
		return sparklineLevel(audio.ComputeAudioRMS(pcmData[start:end]))
	}

	var sb strings.Builder
	for i := 0; i < width; i++ {
		left := subLevel(2 * i)
		right := subLevel(2*i + 1)
		if left == 0 && right == 0 {
			sb.WriteRune(' ')
			continue
		}
		sb.WriteRune(packedSparklineGlyph(left, right))
	}
	return sb.String()
}

// sparklineLevel quantizes rms onto sparklineMinLevel..sparklineLevels using
// a logarithmic scale between sparklineFloorRMS and sparklineCeilingRMS. See
// internal/audio.sparklineLevel for the full rationale (log vs. linear).
func sparklineLevel(rms int) int {
	return sparklineLevelBetween(rms, sparklineFloorRMS, sparklineCeilingRMS)
}

// sparklineLevelBetween is sparklineLevel generalized to an arbitrary
// floor/ceiling, so SparklineStream can honor caller-supplied
// SparklineOptions.FloorRMS/CeilingRMS instead of only the package
// defaults.
func sparklineLevelBetween(rms, floorRMS, ceilingRMS int) int {
	if rms <= floorRMS {
		return sparklineMinLevel
	}
	if rms >= ceilingRMS {
		return sparklineLevels
	}
	ratio := math.Log2(float64(rms)/float64(floorRMS)) / math.Log2(float64(ceilingRMS)/float64(floorRMS))
	level := sparklineMinLevel + int(ratio*float64(sparklineLevels-sparklineMinLevel))
	if level > sparklineLevels {
		level = sparklineLevels
	}
	if level < sparklineMinLevel {
		level = sparklineMinLevel
	}
	return level
}

// leftColumnDots and rightColumnDots fill one dot-column of a Braille cell
// bottom-up for a given height level (0..sparklineLevels; 0 means no dots).
// See internal/audio.leftColumnDots/rightColumnDots for the bit-layout
// rationale.
func leftColumnDots(level int) byte {
	switch {
	case level >= sparklineLevels:
		return 0x47 // dots 1+3+7
	case level == 3:
		return 0x46 // dots 3+7
	case level == 2:
		return 0x44 // dots 3+7
	case level == 1:
		return 0x40 // dot 7 only
	default:
		return 0x00
	}
}

func rightColumnDots(level int) byte {
	switch {
	case level >= sparklineLevels:
		return 0xB8 // dots 4+5+6+8
	case level == 3:
		return 0xB0 // dots 5+6+8
	case level == 2:
		return 0xA0 // dots 6+8
	case level == 1:
		return 0x80 // dot 8 only
	default:
		return 0x00
	}
}

// packedSparklineGlyph builds one Braille Pattern codepoint whose left
// dot-column encodes leftLevel and right dot-column encodes rightLevel
// independently. See internal/audio.packedSparklineGlyph.
func packedSparklineGlyph(leftLevel, rightLevel int) rune {
	dots := leftColumnDots(leftLevel) | rightColumnDots(rightLevel)
	return rune(0x2800 + int(dots))
}

// SparklineOptions configures a SparklineStream.
type SparklineOptions struct {
	Width      int           // Number of Braille characters (default: 10)
	SampleRate int           // Capture sample rate (default: DefaultSampleRate)
	Window     time.Duration // Time span displayed across Width (default: 3s)
	FloorRMS   int           // Noise floor cutoff (default: sparklineFloorRMS)
	CeilingRMS int           // Peak scale ceiling (default: sparklineCeilingRMS)
}

// withDefaults fills in zero-valued fields with the package defaults.
func (o SparklineOptions) withDefaults() SparklineOptions {
	if o.Width <= 0 {
		o.Width = 10
	}
	if o.SampleRate <= 0 {
		o.SampleRate = DefaultSampleRate
	}
	if o.Window <= 0 {
		o.Window = 3 * time.Second
	}
	if o.FloorRMS <= 0 {
		o.FloorRMS = sparklineFloorRMS
	}
	if o.CeilingRMS <= 0 {
		o.CeilingRMS = sparklineCeilingRMS
	}
	return o
}

// SparklineStream is a thread-safe sliding-window accumulator that renders a
// continuously-scrolling Braille time-chart from PCM16LE frames arriving
// live (e.g. from RunCapture's read loop), rather than from one finalized
// chunk the way RenderSparkline is normally used. It keeps only the last
// Window's worth of raw samples; older samples fall off as new ones arrive,
// so Sparkline() always reflects the most recent Window of audio.
type SparklineStream struct {
	mu       sync.Mutex
	opts     SparklineOptions
	buf      []byte
	maxBytes int
}

// NewSparklineStream creates a SparklineStream, applying SparklineOptions
// defaults (see SparklineOptions field comments) for any zero-valued field.
func NewSparklineStream(opts SparklineOptions) *SparklineStream {
	opts = opts.withDefaults()
	maxSamples := int(float64(opts.SampleRate) * opts.Window.Seconds())
	maxBytes := maxSamples * 2
	if maxBytes < 2 {
		maxBytes = 2
	}
	return &SparklineStream{opts: opts, maxBytes: maxBytes}
}

// WritePCM appends raw PCM16LE mono samples to the rolling window, evicting
// the oldest bytes once the window's capacity (SampleRate * Window) is
// exceeded.
func (s *SparklineStream) WritePCM(pcm []byte) {
	if len(pcm) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf = append(s.buf, pcm...)
	if len(s.buf) > s.maxBytes {
		s.buf = s.buf[len(s.buf)-s.maxBytes:]
	}
}

// Sparkline renders the current rolling window as a fixed-width Braille
// time-chart, using this stream's configured Width/FloorRMS/CeilingRMS. As
// with RenderSparkline, a slice the window hasn't yet accumulated enough
// samples for renders as a space rather than a false "silent" glyph.
func (s *SparklineStream) Sparkline() string {
	s.mu.Lock()
	buf := s.buf
	opts := s.opts
	s.mu.Unlock()

	if len(buf) < 2 {
		return strings.Repeat(" ", opts.Width)
	}

	subBuckets := opts.Width * 2
	totalSamples := len(buf) / 2
	samplesPerBucket := totalSamples / subBuckets
	if samplesPerBucket < 1 {
		samplesPerBucket = 1
	}

	subLevel := func(b int) int {
		start := b * samplesPerBucket * 2
		if start >= len(buf) {
			return 0
		}
		end := start + samplesPerBucket*2
		if b == subBuckets-1 || end > len(buf) {
			end = len(buf)
		}
		return sparklineLevelBetween(audio.ComputeAudioRMS(buf[start:end]), opts.FloorRMS, opts.CeilingRMS)
	}

	var sb strings.Builder
	for i := 0; i < opts.Width; i++ {
		left := subLevel(2 * i)
		right := subLevel(2*i + 1)
		if left == 0 && right == 0 {
			sb.WriteRune(' ')
			continue
		}
		sb.WriteRune(packedSparklineGlyph(left, right))
	}
	return sb.String()
}
