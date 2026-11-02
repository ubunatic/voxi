package settings

import (
	"fmt"
	"strconv"
	"strings"

	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/spec"
)

// bluetoothGainDefault is the spec's Bluetooth gain_factor, or 0 when the spec
// cannot be loaded (the labels then say so).
func bluetoothGainDefault() (float64, bool) {
	es, err := spec.LoadEager()
	if err != nil {
		return 0, false
	}
	return es.BluetoothMic.GainFactor, true
}

// bluetoothGainSpec is the spec's bounds for the bluetooth_mic_gain override.
func bluetoothGainSpec() (spec.BluetoothMicSpec, error) {
	es, err := spec.LoadEager()
	if err != nil {
		return spec.BluetoothMicSpec{}, err
	}
	return es.BluetoothMic, nil
}

// parseBluetoothMicGain reads a `voxi settings set bluetooth_mic_gain` value.
// "default" clears the override (stored as unset); any other value must be a
// number in the spec's range, and 1 turns the gain off.
func parseBluetoothMicGain(val string, bm spec.BluetoothMicSpec) (float64, error) {
	v := strings.TrimSpace(val)
	if strings.EqualFold(v, "default") {
		return 0, nil
	}
	g, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid bluetooth_mic_gain %q: use a number or 'default'", val)
	}
	if err := config.ValidateBluetoothMicGain(g, bm); err != nil {
		return 0, err
	}
	return g, nil
}

// formatGain prints a gain without trailing zeros: 2.75, 1, 16.
func formatGain(g float64) string {
	return strconv.FormatFloat(g, 'g', -1, 64)
}

// bluetoothMicGainLabel is the summary text: the effective gain, marked when it
// is the spec default or off.
func bluetoothMicGainLabel(s *config.UserSettings) string {
	def, ok := bluetoothGainDefault()
	if !ok {
		return "unavailable (spec did not load)"
	}
	g := s.EffectiveBluetoothMicGain(def)
	switch {
	case s.BluetoothMicGain == 0:
		return formatGain(g) + "x (spec default)"
	case g == 1:
		return "1x (off)"
	default:
		return formatGain(g) + "x"
	}
}

// bluetoothGainChoices is the menu's choice list and the index of the current
// value. The first choice is the spec default; a set value not in the list is
// appended so it can be shown and kept.
func bluetoothGainChoices(set float64) ([]string, int) {
	def, _ := bluetoothGainDefault()
	choices := []string{"default (" + formatGain(def) + ")"}
	for _, g := range []float64{1, 1.5, 2, 2.75, 4} {
		choices = append(choices, formatGain(g))
	}
	if set == 0 {
		return choices, 0
	}
	label := formatGain(set)
	if idx := indexOf(choices, label); idx >= 0 {
		return choices, idx
	}
	return append(choices, label), len(choices)
}
