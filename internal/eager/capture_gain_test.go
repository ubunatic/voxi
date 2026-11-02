package eager

import (
	"context"
	"errors"
	"testing"

	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/spec"
)

// pwDumpDefault is a minimal pw-dump with one Audio/Source node, which is the
// default; the bluez one is a Bluetooth input (issue 210).
func pwDumpDefault(node string, bluetooth bool) string {
	api := `"device.api": "alsa"`
	if bluetooth {
		api = `"device.api": "bluez5"`
	}
	return `[
	{"type":"PipeWire:Interface:Metadata","id":0,"props":{"metadata.name":"default"},
	 "metadata":[{"key":"default.audio.source","value":{"name":"` + node + `"}}]},
	{"type":"PipeWire:Interface:Node","id":42,"info":{"props":{"media.class":"Audio/Source","node.name":"` + node + `",` + api + `}}}
	]`
}

func gainDeps(out string, err error, havePwDump bool) deps.Dependencies {
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
			return out, err
		},
	}
}

func TestCaptureGainForBluetoothDefault(t *testing.T) {
	es, err := spec.LoadEager()
	if err != nil {
		t.Fatal(err)
	}
	d := gainDeps(pwDumpDefault("bluez_input.AA", true), nil, true)
	if got := captureGain(context.Background(), d, es); got != es.BluetoothMic.GainFactor {
		t.Fatalf("captureGain = %v, want spec gain %v", got, es.BluetoothMic.GainFactor)
	}
}

// Negatives: gain must be exactly 1 (no change) unless the default is Bluetooth.
func TestCaptureGainNoOpWithoutBluetoothDefault(t *testing.T) {
	es, err := spec.LoadEager()
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]deps.Dependencies{
		"built-in default":    gainDeps(pwDumpDefault("alsa_input.pci", false), nil, true),
		"pw-dump fails":       gainDeps("", errors.New("pipewire down"), true),
		"pw-dump not on PATH": gainDeps(pwDumpDefault("bluez_input.AA", true), nil, false),
		"garbage pw-dump":     gainDeps("not json", nil, true),
		"no RunOutput":        {LookPath: func(string) (string, error) { return "/usr/bin/pw-dump", nil }},
	}
	for name, d := range cases {
		t.Run(name, func(t *testing.T) {
			if got := captureGain(context.Background(), d, es); got != 1 {
				t.Fatalf("captureGain = %v, want 1", got)
			}
		})
	}
}
