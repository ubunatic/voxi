package eager

import (
	"context"
	"errors"
	"testing"

	"ubunatic.com/voxi/spec"
)

// The meter (issue 214) asks ProbeCaptureGain for the gain dictation applies,
// so these cases pin the same resolution the session uses.
func TestProbeCaptureGain(t *testing.T) {
	es, err := spec.LoadEager()
	if err != nil {
		t.Fatal(err)
	}
	def := es.BluetoothMic.GainFactor
	bt := pwDumpDefault("bluez_input.AA")
	builtin := pwDumpDefault("alsa_input.pci-0000_00_1f.3.analog-stereo")

	cases := map[string]struct {
		out    string
		err    error
		pwDump bool
		home   string
		want   float64
	}{
		"bluetooth default uses spec gain":       {bt, nil, true, t.TempDir(), def},
		"bluetooth default uses config override": {bt, nil, true, writeGainConfig(t, "bluetooth_mic_gain: 1.5\n"), 1.5},
		"override of 1 turns the gain off":       {bt, nil, true, writeGainConfig(t, "bluetooth_mic_gain: 1\n"), 1},
		"built-in default is never boosted":      {builtin, nil, true, t.TempDir(), 1},
		"failed probe gives unity":               {"", errors.New("pw-dump failed"), true, t.TempDir(), 1},
		"no pw-dump gives unity":                 {bt, nil, false, t.TempDir(), 1},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			d := gainWatchDeps(c.out, c.err, c.pwDump, nil)
			d.Getenv = sessionGainDeps(c.home).Getenv
			if got := ProbeCaptureGain(context.Background(), d, es); got != c.want {
				t.Fatalf("ProbeCaptureGain() = %v, want %v", got, c.want)
			}
		})
	}
}

// With no HOME the probe still applies the spec gain and never reads config.
func TestProbeCaptureGainNoHomeUsesSpec(t *testing.T) {
	es, err := spec.LoadEager()
	if err != nil {
		t.Fatal(err)
	}
	d := gainWatchDeps(pwDumpDefault("bluez_input.AA"), nil, true, nil)
	d.Getenv = func(string) string { return "" }
	if got := ProbeCaptureGain(context.Background(), d, es); got != es.BluetoothMic.GainFactor {
		t.Fatalf("ProbeCaptureGain() = %v, want spec gain %v", got, es.BluetoothMic.GainFactor)
	}
}
