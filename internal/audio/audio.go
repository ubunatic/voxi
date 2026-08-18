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
	ThresholdRMS int
	SilenceMs    int
	PreRollMs    int
	MinSpeechMs  int
	MaxWindowMs  int
}

// DefaultSegmenterOptions returns standard defaults.
func DefaultSegmenterOptions() SegmenterOptions {
	return SegmenterOptions{
		ThresholdRMS: 150,
		SilenceMs:    800,
		PreRollMs:    500,
		MinSpeechMs:  200,
		MaxWindowMs:  8000,
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

// ProcessFrame ingests a 20ms frame of S16_LE PCM audio.
// Returns a non-nil speech segment slice if an utterance chunk has finished/triggered.
func (s *AudioSegmenter) ProcessFrame(frame []byte) (speechSegment []byte, speechStarted bool, isSpeaking bool) {
	if len(frame) < s.frameBytes {
		return nil, false, s.isSpeaking
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
					speechSegment = FlattenAudioFrames(trimmedFrames)
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
		speechSegment = FlattenAudioFrames(s.speechFrames)
		overlapFrames := s.preRollFrames
		if len(s.speechFrames) < overlapFrames {
			overlapFrames = len(s.speechFrames)
		}
		overlap := make([][]byte, overlapFrames)
		copy(overlap, s.speechFrames[len(s.speechFrames)-overlapFrames:])
		s.speechFrames = overlap
	}

	return speechSegment, speechStarted, s.isSpeaking
}

// Flush forces any currently buffered speech into a segment.
func (s *AudioSegmenter) Flush() []byte {
	if s.isSpeaking && len(s.speechFrames) >= s.minSpeechFrames {
		res := FlattenAudioFrames(s.speechFrames)
		s.speechFrames = nil
		s.isSpeaking = false
		return res
	}
	s.speechFrames = nil
	s.isSpeaking = false
	return nil
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
