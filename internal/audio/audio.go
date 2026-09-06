package audio

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"strings"
)

// SegmenterOptions holds configuration parameters for audio segmentation.
type SegmenterOptions struct {
	ThresholdRMS       int
	SilenceMs          int
	PreRollMs          int
	MinSpeechMs        int
	MaxWindowMs        int
	MinVoicedFrames    int // Minimum number of voiced frames (>= ThresholdRMS) across the segment (default: 8 = 160ms)
	MinVoicedRunFrames int // Minimum consecutive voiced frames required to accept a segment (default: 7 = 140ms)
	MinMeanRMS         int // Minimum average RMS across candidate segment (default: 120)
}

// DefaultSegmenterOptions returns standard defaults.
func DefaultSegmenterOptions() SegmenterOptions {
	return SegmenterOptions{
		ThresholdRMS:       150,
		SilenceMs:          800,
		PreRollMs:          500,
		MinSpeechMs:        200,
		MaxWindowMs:        8000,
		MinVoicedFrames:    8,
		MinVoicedRunFrames: 7,
		MinMeanRMS:         120,
	}
}

// AudioSegmenter processes incoming PCM frames (16kHz mono S16_LE) and identifies speech segments.
type AudioSegmenter struct {
	opts                SegmenterOptions
	frameBytes          int
	preRollFrames       int
	postRollFrames      int
	silenceFramesNeeded int
	minSpeechFrames     int
	maxWindowFrames     int

	preRollBuffer      [][]byte
	speechFrames       [][]byte
	isSpeaking         bool
	consecutiveSilence int
}

// NewAudioSegmenter creates a new AudioSegmenter with the given configuration.
func NewAudioSegmenter(opts SegmenterOptions) *AudioSegmenter {
	const (
		sampleRate = 16000
		frameMs    = 20
		frameBytes = (sampleRate * frameMs / 1000) * 2 // 640 bytes (320 samples @ 16-bit)
	)

	preRollFrames := (opts.PreRollMs + frameMs - 1) / frameMs
	if preRollFrames < 1 {
		preRollFrames = 1
	}

	silenceFramesNeeded := (opts.SilenceMs + frameMs - 1) / frameMs
	if silenceFramesNeeded < 1 {
		silenceFramesNeeded = 1
	}

	// Keep up to 350ms of trailing post-roll audio padding to preserve quiet trailing consonants
	postRollFrames := (350 + frameMs - 1) / frameMs
	if postRollFrames >= silenceFramesNeeded {
		postRollFrames = silenceFramesNeeded / 2
	}

	minSpeechFrames := (opts.MinSpeechMs + frameMs - 1) / frameMs
	if minSpeechFrames < 1 {
		minSpeechFrames = 1
	}

	maxWindowFrames := (opts.MaxWindowMs + frameMs - 1) / frameMs

	if opts.MinVoicedFrames <= 0 {
		opts.MinVoicedFrames = 8
	}
	if opts.MinVoicedRunFrames <= 0 {
		opts.MinVoicedRunFrames = 7
	}
	if opts.MinMeanRMS <= 0 {
		opts.MinMeanRMS = 120
	}

	return &AudioSegmenter{
		opts:                opts,
		frameBytes:          frameBytes,
		preRollFrames:       preRollFrames,
		postRollFrames:      postRollFrames,
		silenceFramesNeeded: silenceFramesNeeded,
		minSpeechFrames:     minSpeechFrames,
		maxWindowFrames:     maxWindowFrames,
		preRollBuffer:       make([][]byte, 0, preRollFrames),
		speechFrames:        make([][]byte, 0, 200),
		isSpeaking:          false,
		consecutiveSilence:  0,
	}
}

// SegmentCandidate holds an extracted audio slice along with whether it passes acoustic plausibility
// and its rejection reason if not.
type SegmentCandidate struct {
	Audio           []byte
	Plausible       bool
	RejectionReason string
	Stats           AudioStats
}

// CheckCandidateAcoustics checks whether frames represent genuine speech rather than transients.
func (s *AudioSegmenter) CheckCandidateAcoustics(frames [][]byte) (bool, string, AudioStats) {
	if len(frames) == 0 {
		return false, "empty", AudioStats{}
	}
	pcm := FlattenAudioFrames(frames)
	stats := AnalyzePCM(pcm, s.opts.ThresholdRMS)

	if stats.VoicedFrames < s.opts.MinVoicedFrames {
		return false, "low_energy_transient", stats
	}
	if stats.MaxVoicedRun < s.opts.MinVoicedRunFrames {
		return false, "unvoiced_transient", stats
	}
	if s.opts.MinMeanRMS > 0 && stats.MeanRMS < s.opts.MinMeanRMS {
		return false, "low_energy_transient", stats
	}
	return true, "", stats
}

// ProcessFrame ingests a 20ms frame of S16_LE PCM audio.
// Returns a SegmentCandidate if an utterance chunk has finished/triggered, along with speechStarted and isSpeaking flags.
func (s *AudioSegmenter) ProcessFrame(frame []byte) (candidate SegmentCandidate, speechStarted bool, isSpeaking bool) {
	if len(frame) < s.frameBytes {
		return SegmentCandidate{}, false, s.isSpeaking
	}

	frameCopy := make([]byte, s.frameBytes)
	copy(frameCopy, frame[:s.frameBytes])

	rms := ComputeAudioRMS(frameCopy)

	if rms >= s.opts.ThresholdRMS {
		if !s.isSpeaking {
			s.isSpeaking = true
			speechStarted = true
			s.consecutiveSilence = 0
			s.speechFrames = make([][]byte, 0, len(s.preRollBuffer)+100)
			// Prepend circular pre-roll buffer to retain initial consonants
			s.speechFrames = append(s.speechFrames, s.preRollBuffer...)
			s.speechFrames = append(s.speechFrames, frameCopy)
		} else {
			s.speechFrames = append(s.speechFrames, frameCopy)
			s.consecutiveSilence = 0
		}
	} else {
		if s.isSpeaking {
			s.speechFrames = append(s.speechFrames, frameCopy)
			s.consecutiveSilence++

			if s.consecutiveSilence >= s.silenceFramesNeeded {
				// Completed utterance on silence pause
				s.isSpeaking = false
				s.consecutiveSilence = 0

				// Trim only excess silence beyond postRollFrames to keep trailing consonants
				trimFrames := s.silenceFramesNeeded - s.postRollFrames
				if trimFrames < 0 {
					trimFrames = 0
				}
				actualSpeechLen := len(s.speechFrames) - trimFrames

				if actualSpeechLen >= s.minSpeechFrames {
					trimmedFrames := s.speechFrames[:actualSpeechLen]
					plausible, reason, stats := s.CheckCandidateAcoustics(trimmedFrames)
					candidate = SegmentCandidate{
						Audio:           FlattenAudioFrames(trimmedFrames),
						Plausible:       plausible,
						RejectionReason: reason,
						Stats:           stats,
					}
				}
				s.speechFrames = nil
			}
		} else {
			// Maintain rolling pre-roll buffer
			if len(s.preRollBuffer) >= s.preRollFrames {
				s.preRollBuffer = s.preRollBuffer[1:]
			}
			s.preRollBuffer = append(s.preRollBuffer, frameCopy)
		}
	}

	// Safety check: if utterance runs continuously longer than MaxWindowMs, force chunk
	if s.isSpeaking && s.maxWindowFrames > 0 && len(s.speechFrames) >= s.maxWindowFrames {
		plausible, reason, stats := s.CheckCandidateAcoustics(s.speechFrames)
		candidate = SegmentCandidate{
			Audio:           FlattenAudioFrames(s.speechFrames),
			Plausible:       plausible,
			RejectionReason: reason,
			Stats:           stats,
		}
		overlapFrames := s.preRollFrames
		if len(s.speechFrames) < overlapFrames {
			overlapFrames = len(s.speechFrames)
		}
		overlap := make([][]byte, overlapFrames)
		copy(overlap, s.speechFrames[len(s.speechFrames)-overlapFrames:])
		s.speechFrames = overlap
	}

	return candidate, speechStarted, s.isSpeaking
}

// Flush forces any currently buffered speech into a segment candidate.
func (s *AudioSegmenter) Flush() SegmentCandidate {
	if s.isSpeaking && len(s.speechFrames) >= s.minSpeechFrames {
		plausible, reason, stats := s.CheckCandidateAcoustics(s.speechFrames)
		res := SegmentCandidate{
			Audio:           FlattenAudioFrames(s.speechFrames),
			Plausible:       plausible,
			RejectionReason: reason,
			Stats:           stats,
		}
		s.speechFrames = nil
		s.isSpeaking = false
		return res
	}
	s.speechFrames = nil
	s.isSpeaking = false
	return SegmentCandidate{}
}

// ComputeAudioRMS calculates the Root Mean Square amplitude of 16-bit PCM audio frame.
func ComputeAudioRMS(frame []byte) int {
	var sumSquares int64
	numSamples := len(frame) / 2
	if numSamples == 0 {
		return 0
	}

	for i := 0; i < len(frame); i += 2 {
		sample := int16(binary.LittleEndian.Uint16(frame[i : i+2]))
		sumSquares += int64(sample) * int64(sample)
	}

	meanSquare := sumSquares / int64(numSamples)
	return int(math.Sqrt(float64(meanSquare)))
}

// FlattenAudioFrames concatenates multiple audio frames into a contiguous byte slice.
func FlattenAudioFrames(frames [][]byte) []byte {
	totalLen := 0
	for _, f := range frames {
		totalLen += len(f)
	}
	res := make([]byte, 0, totalLen)
	for _, f := range frames {
		res = append(res, f...)
	}
	return res
}

// RenderAudioLevelMeter generates an ASCII audio level bar for console output.
func RenderAudioLevelMeter(rms int, threshold int) string {
	bars := 10
	level := (rms * bars) / (threshold * 2)
	if level > bars {
		level = bars
	}
	var sb strings.Builder
	for i := 0; i < bars; i++ {
		if i < level {
			sb.WriteString("■")
		} else {
			sb.WriteString("·")
		}
	}
	return sb.String()
}

// sparklineFloorRMS and sparklineCeilingRMS bound the RMS range
// RenderVolumeSparkline maps onto its Braille height steps. RMS at or below
// the floor still renders as the quietest *audible* glyph (sparklineMinLevel,
// "⣀") — not blank — since it represents a real, measured (if silent)
// bucket; RMS at or above the ceiling saturates at the loudest glyph. The
// floor sits just below the acoustic gate's MinMeanRMS (default 120, see
// SegmenterOptions) so a chunk the gate rejected as low_energy_transient
// still reads as visibly flat-and-low, just never as literally blank — a
// blank/space glyph is reserved exclusively for buckets with no data at all
// (see RenderVolumeSparkline). The floor-to-ceiling mapping is logarithmic,
// not linear (see sparklineLevel) — human speech RMS commonly spans tens to
// low-thousands, and a linear scale against any single ceiling crushes
// ordinary accepted speech (RMS ~150-2000) down to the minimum glyph on
// every bucket, making the sparkline useless for exactly the chunks it's
// meant to help diagnose.
//
// The ceiling was originally 2048, tuned only against synthetic test tones.
// Inspecting real per-bucket RMS from actual recordings showed ordinary
// conversational speech commonly spiking to 600-1500 per bucket (well below
// what a listener would call "loud"), which the 2048 ceiling already read as
// 2-3 out of 4 dots — everything looked "loud". 8192 (25% of int16
// full-scale headroom, 32767) leaves real speech room to breathe: typical
// buckets land around level 1-2, level 3 needs genuinely elevated volume,
// and level 4 is reserved for audio close to clipping.
const (
	sparklineFloorRMS   = 80
	sparklineCeilingRMS = 8192
)

// sparklineMinLevel and sparklineLevels bound the discrete height steps the
// sparkline maps *measured* RMS values onto — sparklineMinLevel (dots 7+8,
// glyph "⣀") is the quietest audible reading, sparklineLevels is the
// loudest. There is no "0 = blank" step in this range: blank/space is a
// separate "no data for this bucket" signal, produced by
// RenderVolumeSparkline directly, never by sparklineGlyph.
const (
	sparklineMinLevel = 1
	sparklineLevels   = 4
)

// RenderVolumeSparkline renders a fixed-width (buckets-character) Braille
// sparkline showing how RMS energy varies across the duration of a PCM
// buffer (16-bit signed, mono, any sample rate). Unlike RenderAudioLevelMeter
// (a single instantaneous live-level bar with no time axis), this splits the
// buffer into `buckets` equal time slices, computes the RMS of each slice,
// and maps each to a Braille cell height, giving an at-a-glance "loud
// throughout" vs "trails off to silence" vs "uniformly quiet" shape.
//
// A plain ASCII space means "no data for this bucket" (the buffer was too
// short to fill every requested bucket) — distinct from the quietest
// measured glyph ("⣀"), which means a bucket *was* measured and found
// silent. Conflating the two would make a genuinely-recorded silent moment
// indistinguishable from a gap where nothing was ever measured.
func RenderVolumeSparkline(pcmData []byte, buckets int) string {
	if buckets <= 0 {
		buckets = 10
	}
	if len(pcmData) < 2 {
		return strings.Repeat(" ", buckets)
	}

	totalSamples := len(pcmData) / 2
	samplesPerBucket := totalSamples / buckets
	if samplesPerBucket < 1 {
		samplesPerBucket = 1
	}

	var sb strings.Builder
	for b := 0; b < buckets; b++ {
		start := b * samplesPerBucket * 2
		end := start + samplesPerBucket*2
		if b == buckets-1 {
			end = len(pcmData)
		}
		if start >= len(pcmData) {
			sb.WriteRune(' ')
			continue
		}
		if end > len(pcmData) {
			end = len(pcmData)
		}
		rms := ComputeAudioRMS(pcmData[start:end])
		sb.WriteRune(sparklineGlyph(sparklineLevel(rms)))
	}
	return sb.String()
}

// sparklineLevel quantizes a measured raw RMS value into
// sparklineMinLevel..sparklineLevels using a logarithmic scale between
// sparklineFloorRMS and sparklineCeilingRMS — never 0/blank, since reaching
// this function at all means a real bucket was measured (see
// RenderVolumeSparkline for the separate "no data" path). A linear scale was
// tried first and rejected: against a ceiling high enough to leave headroom
// for genuinely loud audio, ordinary accepted speech (RMS in the low
// hundreds to low thousands) rounded down to the minimum glyph on every
// bucket, making quiet-but-accepted and rejected chunks look identical.
func sparklineLevel(rms int) int {
	if rms <= sparklineFloorRMS {
		return sparklineMinLevel
	}
	if rms >= sparklineCeilingRMS {
		return sparklineLevels
	}
	ratio := math.Log2(float64(rms)/sparklineFloorRMS) / math.Log2(float64(sparklineCeilingRMS)/sparklineFloorRMS)
	level := sparklineMinLevel + int(ratio*float64(sparklineLevels-sparklineMinLevel))
	if level > sparklineLevels {
		level = sparklineLevels
	}
	if level < sparklineMinLevel {
		level = sparklineMinLevel
	}
	return level
}

// sparklineGlyph maps a 0..sparklineLevels height step to a Braille Pattern
// codepoint (U+2800-U+28FF) by filling the cell's four dot-rows bottom-up,
// symmetrically across both dot-columns (dots 7+8, then 3+6, then 2+5, then
// 1+4 per the standard Braille Patterns dot numbering).
func sparklineGlyph(level int) rune {
	var dots byte
	switch {
	case level >= 4:
		dots = 0xFF // all 8 dots
	case level == 3:
		dots = 0xF6 // + dots 2,5 (row1)
	case level == 2:
		dots = 0xE4 // + dots 3,6 (row2)
	default:
		dots = 0xC0 // dots 7,8 (row3, bottom) — the quietest audible glyph;
		// sparklineLevel never emits below sparklineMinLevel (1), so this
		// also serves as the safe floor for any out-of-range input.
	}
	return rune(0x2800 + int(dots))
}

// WriteWAVAudio writes 16kHz 16-bit mono PCM data with a standard RIFF/WAVE header.
func WriteWAVAudio(path string, pcmData []byte, sampleRate int) error {
	var buf bytes.Buffer

	// RIFF header
	buf.WriteString("RIFF")
	totalSize := uint32(36 + len(pcmData))
	_ = binary.Write(&buf, binary.LittleEndian, totalSize)
	buf.WriteString("WAVE")

	// fmt chunk
	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16)) // subchunk1 size
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))  // PCM
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))  // 1 channel (mono)
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))
	byteRate := uint32(sampleRate * 1 * 2)
	_ = binary.Write(&buf, binary.LittleEndian, byteRate)
	_ = binary.Write(&buf, binary.LittleEndian, uint16(2))  // block align
	_ = binary.Write(&buf, binary.LittleEndian, uint16(16)) // bits per sample

	// data chunk
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(len(pcmData)))
	buf.Write(pcmData)

	return os.WriteFile(path, buf.Bytes(), 0600)
}

// AudioStats holds acoustic energy and voicing metrics for an audio segment.
type AudioStats struct {
	TotalFrames  int
	VoicedFrames int
	MaxVoicedRun int
	MeanRMS      int
	PeakRMS      int
	VoicedRatio  float64
}

// AnalyzePCM calculates acoustic energy and frame voicing metrics for a 16kHz S16_LE PCM slice.
func AnalyzePCM(pcmData []byte, thresholdRMS int) AudioStats {
	const frameBytes = 640 // 20ms @ 16kHz mono 16-bit
	totalFrames := len(pcmData) / frameBytes
	if totalFrames == 0 {
		return AudioStats{}
	}

	voicedFrames := 0
	maxVoicedRun := 0
	currVoicedRun := 0
	var sumRMS int64
	peakRMS := 0

	for i := 0; i+frameBytes <= len(pcmData); i += frameBytes {
		rms := ComputeAudioRMS(pcmData[i : i+frameBytes])
		sumRMS += int64(rms)
		if rms > peakRMS {
			peakRMS = rms
		}
		if rms >= thresholdRMS {
			voicedFrames++
			currVoicedRun++
			if currVoicedRun > maxVoicedRun {
				maxVoicedRun = currVoicedRun
			}
		} else {
			currVoicedRun = 0
		}
	}

	meanRMS := int(sumRMS / int64(totalFrames))
	ratio := float64(voicedFrames) / float64(totalFrames)

	return AudioStats{
		TotalFrames:  totalFrames,
		VoicedFrames: voicedFrames,
		MaxVoicedRun: maxVoicedRun,
		MeanRMS:      meanRMS,
		PeakRMS:      peakRMS,
		VoicedRatio:  ratio,
	}
}
