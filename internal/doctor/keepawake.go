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
func keepAwakeStatus() string {
	es, err := spec.LoadEager()
	if err != nil {
		return "Bluetooth keep-awake: unknown (" + err.Error() + ")"
	}
	return mic.NewHeadset(deps.Dependencies{Getenv: os.Getenv, ReadFile: os.ReadFile}, es.BluetoothMic).StatusLine()
}
