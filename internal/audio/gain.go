package audio

import (
	"encoding/binary"
	"math"
)

// ApplyGain returns a new little-endian PCM16 buffer with every sample scaled
// by factor and rounded to the nearest integer, clamped to the int16 range.
// A trailing odd byte is copied unchanged. The input is never modified, so
// callers may keep reusing their capture buffer (issue 210).
func ApplyGain(pcm []byte, factor float64) []byte {
	out := make([]byte, len(pcm))
	copy(out, pcm)
	for i := 0; i+1 < len(pcm); i += 2 {
		v := math.Round(float64(int16(binary.LittleEndian.Uint16(pcm[i:]))) * factor)
		switch {
		case v > math.MaxInt16:
			v = math.MaxInt16
		case v < math.MinInt16:
			v = math.MinInt16
		}
		binary.LittleEndian.PutUint16(out[i:], uint16(int16(v)))
	}
	return out
}
