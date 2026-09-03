package audio

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func generateSineFrame(samples int, freq float64, amplitude int16) []byte {
	buf := make([]byte, samples*2)
	for i := 0; i < samples; i++ {
		val := amplitude // square/constant for simple RMS check
		binary.LittleEndian.PutUint16(buf[i*2:i*2+2], uint16(val))
	}
	return buf
}

func TestComputeAudioRMS(t *testing.T) {
	silence := make([]byte, 640)
	if rms := ComputeAudioRMS(silence); rms != 0 {
		t.Fatalf("expected 0 RMS for silence, got %d", rms)
	}

	signal := generateSineFrame(320, 440, 1000)
	rms := ComputeAudioRMS(signal)
	if rms != 1000 {
		t.Fatalf("expected 1000 RMS, got %d", rms)
	}
}

func TestAudioSegmenter(t *testing.T) {
	opts := SegmenterOptions{
		ThresholdRMS:       500,
		SilenceMs:          60,  // 3 frames @ 20ms
		PreRollMs:          40,  // 2 frames
		MinSpeechMs:        40,  // 2 frames
		MaxWindowMs:        200, // 10 frames
		MinVoicedFrames:    2,
		MinVoicedRunFrames: 2,
		MinMeanRMS:         200,
	}

	segmenter := NewAudioSegmenter(opts)
	silenceFrame := make([]byte, 640)
	speechFrame := generateSineFrame(320, 440, 1000) // RMS = 1000 > 500

	// 1. Send 3 silence frames (pre-roll buffer filling)
	for i := 0; i < 3; i++ {
		cand, started, speaking := segmenter.ProcessFrame(silenceFrame)
		if len(cand.Audio) > 0 || started || speaking {
			t.Fatalf("unexpected state during initial silence: cand=%v, started=%v, speaking=%v", cand, started, speaking)
		}
	}

	// 2. Send 3 speech frames
	for i := 0; i < 3; i++ {
		cand, started, speaking := segmenter.ProcessFrame(speechFrame)
		if i == 0 && !started {
			t.Fatalf("expected speechStarted on first speech frame")
		}
		if !speaking {
			t.Fatalf("expected speaking true")
		}
		if len(cand.Audio) > 0 {
			t.Fatalf("unexpected segment before silence")
		}
	}

	// 3. Send 3 silence frames (trigger pause cutoff)
	var finalCand SegmentCandidate
	for i := 0; i < 3; i++ {
		cand, started, speaking := segmenter.ProcessFrame(silenceFrame)
		if started {
			t.Fatalf("unexpected started during silence")
		}
		if i < 2 && !speaking {
			t.Fatalf("expected still speaking during silence buffer")
		}
		if i == 2 {
			finalCand = cand
			if speaking {
				t.Fatalf("expected speaking false after silence threshold reached")
			}
		}
	}

	if len(finalCand.Audio) == 0 {
		t.Fatalf("expected non-empty speech segment on pause")
	}
	if !finalCand.Plausible {
		t.Fatalf("expected plausible candidate, got rejected: %s", finalCand.RejectionReason)
	}
}

func TestAcousticGatingRejectsIsolatedSpike(t *testing.T) {
	opts := SegmenterOptions{
		ThresholdRMS:       500,
		SilenceMs:          60,
		PreRollMs:          40,
		MinSpeechMs:        40,
		MaxWindowMs:        400,
		MinVoicedFrames:    4,
		MinVoicedRunFrames: 3,
		MinMeanRMS:         200,
	}
	segmenter := NewAudioSegmenter(opts)
	silenceFrame := make([]byte, 640)
	spikeFrame := generateSineFrame(320, 440, 2000) // RMS = 2000

	// 1 spike frame followed by silence (resembling keyboard click / breath spike)
	segmenter.ProcessFrame(silenceFrame)
	segmenter.ProcessFrame(silenceFrame)
	segmenter.ProcessFrame(spikeFrame)

	var candidate SegmentCandidate
	for i := 0; i < 3; i++ {
		cand, _, _ := segmenter.ProcessFrame(silenceFrame)
		if len(cand.Audio) > 0 {
			candidate = cand
		}
	}

	if len(candidate.Audio) == 0 {
		t.Fatal("expected candidate to be emitted for diagnostic tracking")
	}
	if candidate.Plausible {
		t.Fatalf("expected isolated spike to be rejected as implausible, got plausible")
	}
	if candidate.RejectionReason != "low_energy_transient" && candidate.RejectionReason != "unvoiced_transient" {
		t.Fatalf("unexpected rejection reason: %s", candidate.RejectionReason)
	}
}

func TestAcousticGatingAcceptsGenuineShortSpeech(t *testing.T) {
	opts := SegmenterOptions{
		ThresholdRMS:       150,
		SilenceMs:          60,
		PreRollMs:          40,
		MinSpeechMs:        100, // 5 frames
		MaxWindowMs:        800,
		MinVoicedFrames:    8,
		MinVoicedRunFrames: 7,
		MinMeanRMS:         120,
	}
	segmenter := NewAudioSegmenter(opts)
	silenceFrame := make([]byte, 640)
	speechFrame := generateSineFrame(320, 440, 400) // RMS = 400 > 150

	// Pre-roll
	segmenter.ProcessFrame(silenceFrame)
	segmenter.ProcessFrame(silenceFrame)

	// Sustained word (e.g. 10 frames = 200ms of "Stop" or "Yes")
	for i := 0; i < 10; i++ {
		segmenter.ProcessFrame(speechFrame)
	}

	var candidate SegmentCandidate
	for i := 0; i < 3; i++ {
		cand, _, _ := segmenter.ProcessFrame(silenceFrame)
		if len(cand.Audio) > 0 {
			candidate = cand
		}
	}

	if len(candidate.Audio) == 0 {
		t.Fatal("expected candidate for short speech")
	}
	if !candidate.Plausible {
		t.Fatalf("expected sustained short speech to be accepted as plausible, rejected with: %s (stats: %+v)", candidate.RejectionReason, candidate.Stats)
	}
}

func TestAcousticGatingFlushAndMaxWindow(t *testing.T) {
	opts := SegmenterOptions{
		ThresholdRMS:       200,
		SilenceMs:          100,
		PreRollMs:          40,
		MinSpeechMs:        40,
		MaxWindowMs:        100, // 5 frames forces chunk
		MinVoicedFrames:    3,
		MinVoicedRunFrames: 3,
		MinMeanRMS:         150,
	}
	segmenter := NewAudioSegmenter(opts)
	speechFrame := generateSineFrame(320, 440, 500)

	// 1. MaxWindow trigger
	var maxWinCand SegmentCandidate
	for i := 0; i < 5; i++ {
		cand, _, _ := segmenter.ProcessFrame(speechFrame)
		if len(cand.Audio) > 0 {
			maxWinCand = cand
		}
	}
	if len(maxWinCand.Audio) == 0 || !maxWinCand.Plausible {
		t.Fatalf("expected plausible candidate from MaxWindow trigger, got %+v", maxWinCand)
	}

	// 2. Flush trigger
	segmenter.ProcessFrame(speechFrame)
	segmenter.ProcessFrame(speechFrame)
	flushCand := segmenter.Flush()
	if len(flushCand.Audio) == 0 || !flushCand.Plausible {
		t.Fatalf("expected plausible candidate from Flush, got %+v", flushCand)
	}
}

func TestWriteWAVAudio(t *testing.T) {
	dir := t.TempDir()
	wavFile := filepath.Join(dir, "test.wav")
	pcm := make([]byte, 3200) // 0.1s of 16kHz audio

	if err := WriteWAVAudio(wavFile, pcm, 16000); err != nil {
		t.Fatalf("WriteWAVAudio failed: %v", err)
	}

	data, err := os.ReadFile(wavFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 44+3200 {
		t.Fatalf("expected %d bytes, got %d", 44+3200, len(data))
	}
	if string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		t.Fatalf("invalid WAV header: %q", data[:12])
	}
}

func TestRenderAudioLevelMeter(t *testing.T) {
	m0 := RenderAudioLevelMeter(0, 100)
	if m0 != "··········" {
		t.Fatalf("expected empty meter, got %q", m0)
	}

	mFull := RenderAudioLevelMeter(300, 100)
	if mFull != "■■■■■■■■■■" {
		t.Fatalf("expected full meter, got %q", mFull)
	}
}
