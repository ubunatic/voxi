package eager

import (
	"context"

	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/mic"
	"ubunatic.com/voxi/spec"
)

// captureGain returns the digital gain for one recording (issue 210): the
// spec's bluetooth_mic.gain_factor when the default source is a Bluetooth
// input, otherwise 1. The system volume is never changed. Without a usable
// PipeWire probe the audio is left untouched, so a failed check never boosts
// a built-in mic.
func captureGain(ctx context.Context, d deps.Dependencies, es *spec.EagerSpec) float64 {
	if d.RunOutput == nil || d.LookPath == nil {
		return 1
	}
	if _, err := d.LookPath("pw-dump"); err != nil {
		return 1
	}
	pctx, cancel := context.WithTimeout(ctx, es.MicCheckTimeout())
	defer cancel()
	st, err := mic.NewDoctor(d, es.MicSettle()).Probe(pctx)
	if err != nil || !st.DefaultIsBluetooth() {
		return 1
	}
	return es.BluetoothMic.GainFactor
}
