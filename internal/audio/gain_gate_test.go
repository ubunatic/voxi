package audio

import (
	"math"
	"testing"

	"ubunatic.com/voxi/spec"
)

// gateFrameSamples is one 20 ms frame of S16_LE mono at 16 kHz (640 bytes).
const gateFrameSamples = 320

// levelFrame is one 20 ms frame at a constant level, so its RMS equals level.
func levelFrame(level int16) []byte {
	s := make([]int16, gateFrameSamples)
	for i := range s {
		s[i] = level
	}
	return gainPCM(s...)
}

// feedGated runs frames through a segmenter built from the default options and
// reports whether speech ever started and every candidate it returned.
func feedGated(frames [][]byte) (started bool, cands []SegmentCandidate) {
	seg := NewAudioSegmenter(DefaultSegmenterOptions())
	for _, f := range frames {
		c, s, _ := seg.ProcessFrame(f)
		started = started || s
		if len(c.Audio) > 0 {
			cands = append(cands, c)
		}
	}
	cands = append(cands, func() []SegmentCandidate {
		if c := seg.Flush(); len(c.Audio) > 0 {
			return []SegmentCandidate{c}
		}
		return nil
	}()...)
	return started, cands
}

func repeatFrame(f []byte, n int) [][]byte {
	out := make([][]byte, n)
	for i := range out {
		out[i] = f
	}
	return out
}

// Negative: a quiet room (raw RMS 13-20) becomes 36-55 after the spec gain and
// must never start speech.
func TestBluetoothGainKeepsRoomNoiseBelowGate(t *testing.T) {
	es, err := spec.LoadEager()
	if err != nil {
		t.Fatal(err)
	}
	for raw := int16(13); raw <= 20; raw++ {
		frame := ApplyGain(levelFrame(raw), es.BluetoothMic.GainFactor)
		// 250 frames is 5 s of steady noise.
		started, cands := feedGated(repeatFrame(frame, 250))
		if started || len(cands) > 0 {
			t.Fatalf("raw noise RMS %d gained to %d: started=%v candidates=%d",
				raw, ComputeAudioRMS(frame), started, len(cands))
		}
	}
}

// Negative: a 1 s raw burst at RMS 50 (about 138 after gain) stays under
// ThresholdRMS 150 and must not open an utterance. This is the breath or
// bump class of noise that gain can newly reach (issue 210 risk).
func TestBluetoothGainBurstBelowThresholdIsIgnored(t *testing.T) {
	es, err := spec.LoadEager()
	if err != nil {
		t.Fatal(err)
	}
	burst := ApplyGain(levelFrame(50), es.BluetoothMic.GainFactor)
	if rms := ComputeAudioRMS(burst); rms >= DefaultSegmenterOptions().ThresholdRMS {
		t.Fatalf("raw 50 gained to RMS %d, expected below the threshold", rms)
	}
	frames := append(repeatFrame(burst, 50), repeatFrame(levelFrame(0), 50)...)
	started, cands := feedGated(frames)
	if started || len(cands) > 0 {
		t.Fatalf("burst started=%v candidates=%d, want neither", started, len(cands))
	}
}

// Positive: soft speech (raw RMS 100, gained to about 275) followed by 800 ms
// of silence closes one candidate with the speech in it.
func TestBluetoothGainSoftSpeechYieldsCandidate(t *testing.T) {
	es, err := spec.LoadEager()
	if err != nil {
		t.Fatal(err)
	}
	speech := ApplyGain(levelFrame(100), es.BluetoothMic.GainFactor)
	frames := append(repeatFrame(speech, 25), repeatFrame(levelFrame(0), 40)...)
	started, cands := feedGated(frames)
	if !started {
		t.Fatal("soft speech did not start an utterance")
	}
	if len(cands) != 1 {
		t.Fatalf("candidates = %d, want 1", len(cands))
	}
	if len(cands[0].Audio) < 25*len(speech) {
		t.Fatalf("candidate has %d bytes, want at least the 25 speech frames", len(cands[0].Audio))
	}
}

// Negative: a speech-like sine at RMS 1000 gained by 2.75 keeps its shape
// under the int16 clamp: well under 1% of samples are clipped.
func TestBluetoothGainSpeechLevelClipsRarely(t *testing.T) {
	es, err := spec.LoadEager()
	if err != nil {
		t.Fatal(err)
	}
	const n = 16000          // 1 s at 16 kHz
	amp := 1000 * math.Sqrt2 // peak for RMS 1000
	s := make([]int16, n)
	for i := range s {
		s[i] = int16(amp * math.Sin(2*math.Pi*180*float64(i)/16000))
	}
	out := gainSamples(ApplyGain(gainPCM(s...), es.BluetoothMic.GainFactor))
	clipped := 0
	for _, v := range out {
		if v == 32767 || v == -32768 {
			clipped++
		}
	}
	if limit := n / 100; clipped >= limit {
		t.Fatalf("clipped %d of %d samples, want fewer than %d", clipped, n, limit)
	}
}
