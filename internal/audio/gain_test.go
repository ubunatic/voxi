package audio

import (
	"encoding/binary"
	"testing"

	"ubunatic.com/voxi/spec"
)

// gainPCM builds little-endian PCM16 from samples.
func gainPCM(samples ...int16) []byte {
	b := make([]byte, 2*len(samples))
	for i, s := range samples {
		binary.LittleEndian.PutUint16(b[2*i:], uint16(s))
	}
	return b
}

// gainSamples decodes little-endian PCM16 (a trailing odd byte is ignored).
func gainSamples(b []byte) []int16 {
	out := make([]int16, len(b)/2)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(b[2*i:]))
	}
	return out
}

func TestApplyGainScalesSamples(t *testing.T) {
	got := gainSamples(ApplyGain(gainPCM(100, -100, 1000), 2))
	want := []int16{200, -200, 2000}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sample %d = %d, want %d (all %v)", i, got[i], want[i], got)
		}
	}
}

func TestApplyGainFactorOneIsIdentity(t *testing.T) {
	in := gainPCM(-32768, -1, 0, 1, 32767)
	out := ApplyGain(in, 1)
	if string(out) != string(in) {
		t.Fatalf("ApplyGain(x, 1) changed the bytes: %v -> %v", in, out)
	}
}

func TestApplyGainClampsToInt16(t *testing.T) {
	got := gainSamples(ApplyGain(gainPCM(20000, -20000, 5000), 6))
	if got[0] != 32767 || got[1] != -32768 {
		t.Fatalf("clamp = %d, %d; want 32767, -32768", got[0], got[1])
	}
	if got[2] != 30000 {
		t.Fatalf("unclamped sample = %d, want 30000", got[2])
	}
}

func TestApplyGainDoesNotAliasInput(t *testing.T) {
	in := gainPCM(10, 20)
	before := append([]byte(nil), in...)
	out := ApplyGain(in, 3)
	if string(in) != string(before) {
		t.Fatal("ApplyGain modified its input")
	}
	if &out[0] == &in[0] {
		t.Fatal("ApplyGain returned the input slice")
	}
}

func TestApplyGainKeepsTrailingOddByte(t *testing.T) {
	in := append(gainPCM(100), 0x7f)
	out := ApplyGain(in, 2)
	if len(out) != len(in) {
		t.Fatalf("len = %d, want %d", len(out), len(in))
	}
	if out[len(out)-1] != 0x7f {
		t.Fatalf("trailing byte = %#x, want 0x7f", out[len(out)-1])
	}
	if got := gainSamples(out)[0]; got != 200 {
		t.Fatalf("sample = %d, want 200", got)
	}
}

// TestBluetoothGainLiftsSoftSpeechPastGate checks the configured factor
// against the 2026-11-02 host measurement (issue 210): soft speech measured at
// chunk RMS 83-109 must clear the acoustic gate's MinMeanRMS (120) after gain.
func TestBluetoothGainLiftsSoftSpeechPastGate(t *testing.T) {
	es, err := spec.LoadEager()
	if err != nil {
		t.Fatal(err)
	}
	gate := DefaultSegmenterOptions().MinMeanRMS
	for _, measured := range []int16{83, 109} {
		flat := make([]int16, 1600) // 100ms at 16 kHz, constant level: RMS equals the level
		for i := range flat {
			flat[i] = measured
		}
		got := ComputeAudioRMS(ApplyGain(gainPCM(flat...), es.BluetoothMic.GainFactor))
		if got < gate {
			t.Errorf("RMS %d * %v = %d, below gate MinMeanRMS %d", measured, es.BluetoothMic.GainFactor, got, gate)
		}
	}
}
