package audiolevel

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// pcm16LE encodes signed 16-bit samples as little-endian bytes, matching
// what `parec --format=s16le` streams and what AmplitudeFromPCM16LE
// consumes.
func pcm16LE(samples []int16) []byte {
	buf := make([]byte, len(samples)*2)
	for i, s := range samples {
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(s))
	}
	return buf
}

func TestAmplitudeFromPCM16LESilence(t *testing.T) {
	buf := pcm16LE(make([]int16, 800))
	if got := AmplitudeFromPCM16LE(buf, DefaultMinDBFS); got != 0 {
		t.Errorf("silence = %v, want 0", got)
	}
}

func TestAmplitudeFromPCM16LEEmpty(t *testing.T) {
	if got := AmplitudeFromPCM16LE(nil, DefaultMinDBFS); got != 0 {
		t.Errorf("empty = %v, want 0", got)
	}
	if got := AmplitudeFromPCM16LE([]byte{0x01}, DefaultMinDBFS); got != 0 {
		t.Errorf("single odd byte = %v, want 0", got)
	}
}

func TestAmplitudeFromPCM16LEFullScale(t *testing.T) {
	samples := make([]int16, 800)
	for i := range samples {
		if i%2 == 0 {
			samples[i] = math.MaxInt16
		} else {
			samples[i] = math.MinInt16
		}
	}
	got := AmplitudeFromPCM16LE(pcm16LE(samples), DefaultMinDBFS)
	if got < 99 || got > 100 {
		t.Errorf("full-scale square wave = %v, want ~100", got)
	}
}

func TestAmplitudeFromPCM16LESineWaveOrdering(t *testing.T) {
	sine := func(amplitude int16) []byte {
		samples := make([]int16, 800)
		for i := range samples {
			samples[i] = int16(float64(amplitude) * math.Sin(2*math.Pi*float64(i)/40))
		}
		return pcm16LE(samples)
	}

	quiet := AmplitudeFromPCM16LE(sine(1000), DefaultMinDBFS)
	loud := AmplitudeFromPCM16LE(sine(20000), DefaultMinDBFS)

	if quiet <= 0 {
		t.Errorf("quiet sine amplitude = %v, want > 0", quiet)
	}
	if loud <= quiet {
		t.Errorf("loud sine amplitude (%v) should exceed quiet (%v)", loud, quiet)
	}
	if loud > 100 {
		t.Errorf("loud sine amplitude = %v, want <= 100", loud)
	}
}

func TestAmplitudeFromPCM16LE_CalibratedVectors(t *testing.T) {
	constantBuffer := func(val int16) []byte {
		samples := make([]int16, 400)
		for i := range samples {
			samples[i] = val
		}
		return pcm16LE(samples)
	}

	tests := []struct {
		name    string
		buf     []byte
		wantMin float64
		wantMax float64
	}{
		{"absolute silence (zeroes)", pcm16LE(make([]int16, 400)), 0.0, 0.0},
		{"below noise floor (RMS 20 < -60 dBFS)", constantBuffer(20), 0.0, 0.0},
		{"quiet conversational speech (RMS 180 ~ -45.2 dBFS)", constantBuffer(180), 24.0, 25.5},
		{"normal conversational speech (RMS 550 ~ -35.5 dBFS)", constantBuffer(550), 40.0, 42.0},
		{"loud speech peak (RMS 2700 ~ -21.7 dBFS)", constantBuffer(2700), 63.0, 65.0},
		{"full scale clipping (RMS 32767 ~ -0.0 dBFS)", constantBuffer(32767), 99.9, 100.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AmplitudeFromPCM16LE(tt.buf, DefaultMinDBFS)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("AmplitudeFromPCM16LE() = %v, want in [%v, %v]", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestManagerNilSafe(t *testing.T) {
	var mgr *Manager
	mgr.Stop() // must not panic
	got := mgr.Snapshot()
	if got != (Reading{}) {
		t.Errorf("nil Snapshot() = %+v, want zero value", got)
	}
}

func TestApplyBallistics_InstantAttack(t *testing.T) {
	tau := 150 * time.Millisecond
	dt := 50 * time.Millisecond
	if got := ApplyBallistics(0.0, 45.0, dt, tau); got != 45.0 {
		t.Errorf("instant attack from 0 to 45 = %v, want 45", got)
	}
	if got := ApplyBallistics(30.0, 75.0, dt, tau); got != 75.0 {
		t.Errorf("instant attack from 30 to 75 = %v, want 75", got)
	}
	if got := ApplyBallistics(50.0, 50.0, dt, tau); got != 50.0 {
		t.Errorf("equal levels = %v, want 50", got)
	}
}

func TestApplyBallistics_SmoothDecay(t *testing.T) {
	level := 50.0
	tau := 150 * time.Millisecond
	dt := 50 * time.Millisecond
	expectedDecayFactor := math.Exp(-dt.Seconds() / tau.Seconds())
	for step := 1; step <= 5; step++ {
		next := ApplyBallistics(level, 0.0, dt, tau)
		expected := level * expectedDecayFactor
		if math.Abs(next-expected) > 1e-6 {
			t.Fatalf("step %d decay: got %v, want %v", step, next, expected)
		}
		if next >= level {
			t.Fatalf("step %d: level did not decrease (%v -> %v)", step, level, next)
		}
		level = next
	}
}

func TestApplyBallistics_CutoffToZero(t *testing.T) {
	tau := 150 * time.Millisecond
	dt := 50 * time.Millisecond
	got := ApplyBallistics(0.6, 0.0, dt, tau)
	if got != 0.0 {
		t.Errorf("decay below cutoff = %v, want 0.0", got)
	}
}

func TestApplyBallistics_ClampToTargetFloor(t *testing.T) {
	tau := 150 * time.Millisecond
	dt := 50 * time.Millisecond
	if got := ApplyBallistics(40.0, 35.0, dt, tau); got != 35.0 {
		t.Errorf("decay with floor: got %v, want 35.0", got)
	}
}

func TestApplyBallistics_DisabledDecay(t *testing.T) {
	dt := 50 * time.Millisecond
	if got := ApplyBallistics(50.0, 20.0, dt, 0); got != 20.0 {
		t.Errorf("decayDuration=0 got %v, want 20.0", got)
	}
	if got := ApplyBallistics(50.0, 20.0, dt, -1*time.Millisecond); got != 20.0 {
		t.Errorf("decayDuration<0 got %v, want 20.0", got)
	}
}

func TestApplyBallistics_ZeroOrNegativeDT(t *testing.T) {
	tau := 150 * time.Millisecond
	if got := ApplyBallistics(50.0, 20.0, 0, tau); got != 50.0 {
		t.Errorf("dt=0 got %v, want prev 50.0", got)
	}
	if got := ApplyBallistics(50.0, 20.0, -10*time.Millisecond, tau); got != 50.0 {
		t.Errorf("dt<0 got %v, want prev 50.0", got)
	}
}

func TestMeter_Update_WindowMetricsWithoutDecay(t *testing.T) {
	var m Meter
	t0 := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	window := 100 * time.Millisecond

	r1 := m.Update(40.0, true, t0, MetricMax, window, 0)
	if !r1.Available || r1.Level != 40.0 {
		t.Errorf("update speech: got %+v, want {Level: 40, Available: true}", r1)
	}
	if snap := m.Snapshot(); snap != r1 {
		t.Errorf("snapshot mismatch: got %+v, want %+v", snap, r1)
	}

	r2 := m.Update(20.0, true, t0.Add(20*time.Millisecond), MetricMax, window, 0)
	if r2.Level != 40.0 {
		t.Errorf("expected max level 40.0, got %v", r2.Level)
	}

	rAvg := m.Update(20.0, true, t0.Add(40*time.Millisecond), MetricAvg, window, 0)
	if rAvg.Level < 26.0 || rAvg.Level > 27.0 {
		t.Errorf("expected avg ~26.66, got %v", rAvg.Level)
	}

	rMin := m.Update(25.0, true, t0.Add(50*time.Millisecond), MetricMin, window, 0)
	if rMin.Level != 20.0 {
		t.Errorf("expected min level 20.0, got %v", rMin.Level)
	}

	rLive := m.Update(25.0, true, t0.Add(60*time.Millisecond), MetricLive, window, 0)
	if rLive.Level != 25.0 {
		t.Errorf("expected live level 25.0, got %v", rLive.Level)
	}

	rExpired := m.Update(15.0, true, t0.Add(200*time.Millisecond), MetricMax, window, 0)
	if rExpired.Level != 15.0 {
		t.Errorf("expected level 15.0 after window expiry, got %v", rExpired.Level)
	}

	rUnavail := m.Update(0.0, false, t0.Add(300*time.Millisecond), MetricMax, window, 0)
	if rUnavail.Available || rUnavail.Level != 0.0 {
		t.Errorf("update unavailable: got %+v, want {Level: 0, Available: false}", rUnavail)
	}
}

func TestMeter_Update_WithDecayBallistics(t *testing.T) {
	var m Meter
	t0 := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	window := 50 * time.Millisecond
	decay := 150 * time.Millisecond

	r1 := m.Update(50.0, true, t0, MetricMax, window, decay)
	if r1.Level != 50.0 || !r1.Available {
		t.Fatalf("initial onset: got %+v, want {Level: 50.0, Available: true}", r1)
	}

	t1 := t0.Add(50 * time.Millisecond)
	r2 := m.Update(0.0, true, t1, MetricLive, window, decay)
	want2 := 50.0 * math.Exp(-0.05/0.15)
	if math.Abs(r2.Level-want2) > 1e-4 {
		t.Errorf("step 1 decay: got %v, want %v", r2.Level, want2)
	}

	t2 := t1.Add(50 * time.Millisecond)
	r3 := m.Update(0.0, true, t2, MetricLive, window, decay)
	want3 := want2 * math.Exp(-0.05/0.15)
	if math.Abs(r3.Level-want3) > 1e-4 {
		t.Errorf("step 2 decay: got %v, want %v", r3.Level, want3)
	}

	t3 := t2.Add(50 * time.Millisecond)
	r4 := m.Update(70.0, true, t3, MetricLive, window, decay)
	if r4.Level != 70.0 {
		t.Errorf("instant rise: got %v, want 70.0", r4.Level)
	}

	cur := 70.0
	curTime := t3
	for i := 0; i < 20; i++ {
		curTime = curTime.Add(50 * time.Millisecond)
		r := m.Update(0.0, true, curTime, MetricLive, window, decay)
		if r.Level == 0.0 {
			break
		}
		if r.Level >= cur {
			t.Fatalf("expected decreasing level, got %v -> %v", cur, r.Level)
		}
		cur = r.Level
	}
	rFinal := m.Snapshot()
	if rFinal.Level != 0.0 {
		t.Errorf("expected clean snap to 0.0, got %v", rFinal.Level)
	}

	m.Update(0.0, false, curTime.Add(50*time.Millisecond), MetricLive, window, decay)
	if snap := m.Snapshot(); snap.Available || snap.Level != 0.0 {
		t.Errorf("expected unavailable state, got %+v", snap)
	}

	rRecon := m.Update(45.0, true, curTime.Add(100*time.Millisecond), MetricLive, window, decay)
	if !rRecon.Available || rRecon.Level != 45.0 {
		t.Errorf("reconnected onset: got %+v, want {Level: 45.0, Available: true}", rRecon)
	}
}

func TestIsGenuineRecording(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want bool
	}{
		{"empty", "", false},
		{
			"meter alone (exempt)",
			"Source Output #123\n\tProperties:\n\t\tapplication.id = \"org.gnome.VolumeControl\"\n\t\tnode.virtual = \"true\"\n",
			false,
		},
		{
			"pavucontrol alone (exempt)",
			"Source Output #5\n\tProperties:\n\t\tapplication.id = \"org.PulseAudio.pavucontrol\"\n",
			false,
		},
		{
			"peak detect media name (exempt)",
			"Source Output #7\n\tProperties:\n\t\tmedia.name = \"Peak detect\"\n",
			false,
		},
		{
			"real recorder present",
			"Source Output #123\n\tProperties:\n\t\tapplication.id = \"org.gnome.VolumeControl\"\n" +
				"Source Output #124\n\tProperties:\n\t\tapplication.id = \"com.example.recorder\"\n",
			true,
		},
		{
			"unrelated app only",
			"Source Output #1\n\tProperties:\n\t\tapplication.id = \"org.mozilla.firefox\"\n",
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsGenuineRecording(tt.out); got != tt.want {
				t.Errorf("IsGenuineRecording(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// writeFakeAudioBinary writes an executable shell script into dir named
// name that dumps payload to stdout and exits (clean EOF, no lingering
// process) — a stand-in for `parec`/`pw-record` that lets RunCapture's real
// exec.Command/StdoutPipe/io.ReadFull machinery run against a known byte
// stream instead of a real audio device.
func writeFakeAudioBinary(t *testing.T, dir, name string, payload []byte) string {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("fake audio binary script assumes a POSIX shell (linux CI)")
	}
	payloadPath := filepath.Join(dir, name+".pcm")
	if err := os.WriteFile(payloadPath, payload, 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	scriptPath := filepath.Join(dir, name)
	script := "#!/bin/sh\ncat " + payloadPath + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake binary %s: %v", name, err)
	}
	return scriptPath
}

// TestRunCapture_ParecAndPwRecordAgree is issue 265's original parity
// requirement, ported: PCM16LE data flowing through either ParecCommand or
// PwRecordCommand must produce the same amplitude reading, since both feed
// the same RunCapture -> AmplitudeFromPCM16LE pipeline.
func TestRunCapture_ParecAndPwRecordAgree(t *testing.T) {
	samples := make([]int16, DefaultChunkBytes/2)
	for i := range samples {
		samples[i] = 550 // matches the calibrated "normal conversational speech" vector
	}
	payload := pcm16LE(samples)
	wantLevel := AmplitudeFromPCM16LE(payload, DefaultMinDBFS)
	if wantLevel <= 0 {
		t.Fatalf("test fixture produced a zero amplitude, fixture is broken")
	}

	dir := t.TempDir()
	fakeBin := writeFakeAudioBinary(t, dir, "pw-record", payload)
	t.Setenv("PATH", filepath.Dir(fakeBin)+string(os.PathListSeparator)+os.Getenv("PATH"))

	var m Meter
	var got []Reading
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	spec := func() (Metric, time.Duration, time.Duration) { return MetricLive, 100 * time.Millisecond, 0 }
	RunCapture(ctx, PwRecordCommand(ctx, 0), &m, DefaultChunkBytes, DefaultMinDBFS, spec, func(r Reading) { got = append(got, r) })

	if len(got) != 1 {
		t.Fatalf("expected exactly one sample from the single-chunk fake binary, got %d: %+v", len(got), got)
	}
	if !got[0].Available {
		t.Fatalf("expected sample to be Available, got %+v", got[0])
	}
	if got[0].Level != wantLevel {
		t.Errorf("pw-record path amplitude = %v, want %v (same as AmplitudeFromPCM16LE on identical bytes)", got[0].Level, wantLevel)
	}
}

func TestStartManager_NilBuildCmdDegradesGracefully(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr := StartManager(ctx, nil, 0, 0, 0, func() (Metric, time.Duration, time.Duration) { return MetricMax, 0, 0 }, nil)
	t.Cleanup(mgr.Stop)
	if got := mgr.Snapshot(); got.Available {
		t.Errorf("nil buildCmd: got %+v, want Available: false", got)
	}
}
