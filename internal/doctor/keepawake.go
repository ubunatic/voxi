package doctor

import (
	"os"

	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/mic"
	"ubunatic.com/voxi/spec"
)

// keepAwakeStatus is the read-only Bluetooth keep-awake line for the mic check.
// It only reads the WirePlumber drop-in; `voxi mic headset` is the one that
// changes it (issue 210).
func keepAwakeStatus(st mic.State) string {
	return keepAwakeLine(st, deps.Dependencies{Getenv: os.Getenv, ReadFile: os.ReadFile})
}

// keepAwakeLine is the status line for a probed state, or "" when the default
// mic is not a Bluetooth input matched by the spec's node_matches.
func keepAwakeLine(st mic.State, d deps.Dependencies) string {
	es, err := spec.LoadEager()
	if err != nil {
		return "Bluetooth keep-awake: unknown (" + err.Error() + ")"
	}
	if !st.DefaultIsBluetooth(es.BluetoothMic.NodeMatches) {
		return ""
	}
	return mic.NewHeadset(d, es.BluetoothMic).StatusLine()
}
