package audio

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
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

func TestAnalyzePCMRecordsMeanPeakAndVoicedRatio(t *testing.T) {
	pcm := append(generateSineFrame(320, 0, 200), generateSineFrame(320, 0, 800)...)
	stats := AnalyzePCM(pcm, 500)
	if stats.TotalFrames != 2 || stats.VoicedFrames != 1 {
		t.Fatalf("frame counts = %+v, want total=2 voiced=1", stats)
	}
	if stats.MeanRMS != 500 || stats.PeakRMS != 800 {
		t.Fatalf("RMS metrics = mean %d peak %d, want 500/800", stats.MeanRMS, stats.PeakRMS)
	}
	if stats.VoicedRatio != 0.5 {
		t.Fatalf("voiced ratio = %v, want 0.5", stats.VoicedRatio)
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

func TestRenderVolumeSparkline(t *testing.T) {
	// Loud first half (RMS 10000, at/above sparklineCeilingRMS of 8192),
	// quiet second half (RMS 20, below sparklineFloorRMS) — 10 buckets over
	// 1000 samples means each bucket is exactly one half or the other.
	loudThenQuiet := append(generateSineFrame(500, 0, 10000), generateSineFrame(500, 0, 20)...)
	quietOnly := generateSineFrame(1000, 0, 20)
	// A realistic *accepted* chunk's RMS (comparable to what voxi chunks list
	// actually sees day to day, well below the old linear ceiling of 4000)
	// must render as real, measured audio — never as a run of "no data"
	// spaces. That was the original bug this test guards against: a linear
	// scale rounded every ordinary speech RMS down to the same glyph as
	// silence, and (separately) silence itself must not render identically
	// to a bucket with no data at all.
	moderateSpeech := generateSineFrame(1000, 0, 184)

	gotLoudThenQuiet := RenderVolumeSparkline(loudThenQuiet, 10)
	gotQuietOnly := RenderVolumeSparkline(quietOnly, 10)
	gotModerateSpeech := RenderVolumeSparkline(moderateSpeech, 10)

	// level(10000) saturates at sparklineLevels (>= ceiling 8192) -> glyph
	// 0x28FF ("⣿"); level(20) is below the floor (80) -> the minimum
	// *audible* glyph 0x28C0 ("⣀"), never blank/space — a quiet-but-measured
	// bucket must stay visually distinct from "no data was measured here".
	wantLoudThenQuiet := strings.Repeat("⣿", 5) + strings.Repeat("⣀", 5)
	wantQuietOnly := strings.Repeat("⣀", 10)

	if gotLoudThenQuiet != wantLoudThenQuiet {
		t.Fatalf("loud-then-quiet sparkline = %q, want %q", gotLoudThenQuiet, wantLoudThenQuiet)
	}
	if gotQuietOnly != wantQuietOnly {
		t.Fatalf("uniformly-quiet sparkline = %q, want %q", gotQuietOnly, wantQuietOnly)
	}
	if gotLoudThenQuiet == gotQuietOnly {
		t.Fatalf("expected loud-then-quiet and uniformly-quiet sparklines to be visibly different, both = %q", gotLoudThenQuiet)
	}
	if strings.Contains(gotQuietOnly, " ") || strings.Contains(gotLoudThenQuiet, " ") || strings.Contains(gotModerateSpeech, " ") {
		t.Fatalf("measured audio must never render the no-data space glyph: loud=%q quiet=%q moderate=%q", gotLoudThenQuiet, gotQuietOnly, gotModerateSpeech)
	}
	// RMS 184 lands just above the floor -> minimum audible glyph on every
	// bucket, same as the quiet-only case, but critically it's "⣀" and not
	// a run of no-data spaces.
	wantModerateSpeech := strings.Repeat("⣀", 10)
	if gotModerateSpeech != wantModerateSpeech {
		t.Fatalf("moderate-speech (RMS 184) sparkline = %q, want %q", gotModerateSpeech, wantModerateSpeech)
	}
	if got := len([]rune(gotLoudThenQuiet)); got != 10 {
		t.Fatalf("expected fixed-width 10-glyph sparkline, got %d glyphs", got)
	}
}

// TestSparklineLevelRealWorldCalibration locks in the ceiling chosen after
// inspecting real per-bucket RMS values from actual recordings (2026-09-06):
// a ceiling of 2048 made ordinary conversational speech (buckets commonly
// spiking to 600-1500) read as 2-3 out of 4 dots on every recording,
// regardless of how quiet the speaker actually was — everything looked
// "loud". These are real bucket RMS values pulled from an actual accepted
// chunk; none of them should reach the top glyph, since none represent
// audio anywhere near clipping (int16 full-scale is 32767).
func TestSparklineLevelRealWorldCalibration(t *testing.T) {
	realBucketRMS := []int{594, 709, 1324, 933, 648, 464, 867, 832, 1464, 925}
	for _, rms := range realBucketRMS {
		if level := sparklineLevel(rms); level >= sparklineLevels {
			t.Fatalf("RMS %d (typical conversational speech) hit the max level %d — ceiling too low", rms, level)
		}
	}
	// And genuinely loud/near-clipping audio must still reach the top.
	if level := sparklineLevel(sparklineCeilingRMS); level != sparklineLevels {
		t.Fatalf("RMS at the ceiling (%d) should saturate at max level %d, got %d", sparklineCeilingRMS, sparklineLevels, level)
	}
}

func TestRenderVolumeSparklineNoData(t *testing.T) {
	// A buffer too short to contain even one sample renders as literal
	// spaces ("no data"), never as the quietest-audible glyph ("⣀") — the
	// two must stay visually distinguishable.
	got := RenderVolumeSparkline(nil, 10)
	want := strings.Repeat(" ", 10)
	if got != want {
		t.Fatalf("empty-buffer sparkline = %q, want %q (all spaces)", got, want)
	}
	if strings.ContainsAny(got, "⣀⣤⣶⣦⣿") {
		t.Fatalf("empty-buffer sparkline must contain no Braille glyphs, got %q", got)
	}
}
