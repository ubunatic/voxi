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
		ThresholdRMS: 500,
		SilenceMs:    60,  // 3 frames @ 20ms
		PreRollMs:    40,  // 2 frames
		MinSpeechMs:  40,  // 2 frames
		MaxWindowMs:  200, // 10 frames
	}

	segmenter := NewAudioSegmenter(opts)
	silenceFrame := make([]byte, 640)
	speechFrame := generateSineFrame(320, 440, 1000) // RMS = 1000 > 500

	// 1. Send 3 silence frames (pre-roll buffer filling)
	for i := 0; i < 3; i++ {
		seg, started, speaking := segmenter.ProcessFrame(silenceFrame)
		if seg != nil || started || speaking {
			t.Fatalf("unexpected state during initial silence: seg=%v, started=%v, speaking=%v", seg, started, speaking)
		}
	}

	// 2. Send 3 speech frames
	for i := 0; i < 3; i++ {
		seg, started, speaking := segmenter.ProcessFrame(speechFrame)
		if i == 0 && !started {
			t.Fatalf("expected speechStarted on first speech frame")
		}
		if !speaking {
			t.Fatalf("expected speaking true")
		}
		if seg != nil {
			t.Fatalf("unexpected segment before silence")
		}
	}

	// 3. Send 3 silence frames (trigger pause cutoff)
	var finalSeg []byte
	for i := 0; i < 3; i++ {
		seg, started, speaking := segmenter.ProcessFrame(silenceFrame)
		if started {
			t.Fatalf("unexpected started during silence")
		}
		if i < 2 && !speaking {
			t.Fatalf("expected still speaking during silence buffer")
		}
		if i == 2 {
			finalSeg = seg
			if speaking {
				t.Fatalf("expected speaking false after silence threshold reached")
			}
		}
	}

	if len(finalSeg) == 0 {
		t.Fatalf("expected non-empty speech segment on pause")
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
