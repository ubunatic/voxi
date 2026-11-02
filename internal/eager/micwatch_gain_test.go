package eager

import (
	"context"
	"errors"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/spec"
)

// pwDumpDefault is a minimal pw-dump with one Audio/Source node, which is the
// default; a bluez node name makes it a Bluetooth input (issue 210).
func pwDumpDefault(node string) string {
	return `[
	{"type":"PipeWire:Interface:Metadata","id":0,"props":{"metadata.name":"default"},
	 "metadata":[{"key":"default.audio.source","value":{"name":"` + node + `"}}]},
	{"type":"PipeWire:Interface:Node","id":42,"info":{"props":{"media.class":"Audio/Source","node.name":"` + node + `","device.api":"bluez5"}}}
	]`
}

// gainWatchDeps answers pw-dump with out (or err). block, when non-nil, holds
// the pw-dump call until it is closed, so a test can prove the watch does not
// wait for the probe.
func gainWatchDeps(out string, err error, havePwDump bool, block <-chan struct{}) deps.Dependencies {
	return deps.Dependencies{
		LookPath: func(name string) (string, error) {
			if name == "pw-dump" && !havePwDump {
				return "", errors.New("not found")
			}
			return "/usr/bin/" + name, nil
		},
		RunOutput: func(_ context.Context, name string, _ ...string) (string, error) {
			if name != "pw-dump" {
				return "", errors.New("unexpected command " + name)
			}
			if block != nil {
				<-block
			}
			return out, err
		},
		Run: func(context.Context, string, ...string) error { return nil },
	}
}

// waitResolved fails the test instead of hanging when the gain never resolves.
func waitResolved(t *testing.T, w *micWatch) {
	t.Helper()
	select {
	case <-w.resolved:
	case <-time.After(5 * time.Second):
		t.Fatal("gain was never resolved")
	}
}

func newGainWatch(t *testing.T, d deps.Dependencies) (*micWatch, *spec.EagerSpec) {
	t.Helper()
	es, err := spec.LoadEager()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	w := startMicWatch(ctx, d, es, 32000, func() {})
	t.Cleanup(w.stop)
	return w, es
}

func TestMicWatchResolvesBluetoothGain(t *testing.T) {
	w, es := newGainWatch(t, gainWatchDeps(pwDumpDefault("bluez_input.AA"), nil, true, nil))
	waitResolved(t, w)
	if got := w.Gain(); got != es.BluetoothMic.GainFactor {
		t.Fatalf("Gain() = %v, want the spec gain %v", got, es.BluetoothMic.GainFactor)
	}
}

// The probe runs off the read loop: startMicWatch returns while pw-dump is
// still blocked, and until then frames get no gain.
func TestMicWatchDoesNotBlockOnGainProbe(t *testing.T) {
	block := make(chan struct{})
	w, es := newGainWatch(t, gainWatchDeps(pwDumpDefault("bluez_input.AA"), nil, true, block))
	if got := w.Gain(); got != 1 {
		t.Fatalf("Gain() before the probe finished = %v, want 1", got)
	}
	select {
	case <-w.resolved:
		t.Fatal("gain resolved while the probe was still blocked")
	default:
	}
	close(block)
	waitResolved(t, w)
	if got := w.Gain(); got != es.BluetoothMic.GainFactor {
		t.Fatalf("Gain() after the probe = %v, want %v", got, es.BluetoothMic.GainFactor)
	}
}

// Negatives: gain is exactly 1 unless the default is a Bluetooth input that the
// spec matches. Each case still resolves, so the frame loop never waits.
func TestMicWatchGainIsOneWithoutBluetoothDefault(t *testing.T) {
	cases := map[string]deps.Dependencies{
		"built-in default":    gainWatchDeps(pwDumpDefault("alsa_input.pci"), nil, true, nil),
		"pw-dump fails":       gainWatchDeps("", errors.New("pipewire down"), true, nil),
		"pw-dump not on PATH": gainWatchDeps(pwDumpDefault("bluez_input.AA"), nil, false, nil),
		"garbage pw-dump":     gainWatchDeps("not json", nil, true, nil),
		"no RunOutput":        {LookPath: func(string) (string, error) { return "/usr/bin/pw-dump", nil }},
	}
	for name, d := range cases {
		t.Run(name, func(t *testing.T) {
			w, _ := newGainWatch(t, d)
			waitResolved(t, w)
			if got := w.Gain(); got != 1 {
				t.Fatalf("Gain() = %v, want 1", got)
			}
		})
	}
}

// Negative: a Bluetooth default the spec does not match gets no gain.
func TestMicWatchGainRespectsSpecNodeMatches(t *testing.T) {
	w, _ := newGainWatch(t, gainWatchDeps(pwDumpDefault("bluez_output.AA"), nil, true, nil))
	waitResolved(t, w)
	if got := w.Gain(); got != 1 {
		t.Fatalf("Gain() for bluez_output.AA = %v, want 1", got)
	}
}
