package mic

import (
	"os"
	"testing"
)

// btMatches is the spec's bluetooth_mic.node_matches (spec/eager.yaml).
var btMatches = []string{"~bluez_input.*", "~bluez_capture_internal.*"}

// TestDefaultIsBluetooth covers the predicate that gates digital gain and
// keep-awake (issue 210): only a Bluetooth default source whose node name the
// spec matches qualifies.
func TestDefaultIsBluetooth(t *testing.T) {
	cases := []struct {
		name string
		st   State
		want bool
	}{
		{"bluetooth input", State{DefaultSource: "bluez_input.A", Sources: []Source{{Name: "bluez_input.A", Bluetooth: true}}}, true},
		{"loopback input", State{DefaultSource: "bluez_input.A", Sources: []Source{{Name: "bluez_input.A", Bluetooth: true, Loopback: true}}}, true},
		{"capture-internal input", State{DefaultSource: "bluez_capture_internal.A", Sources: []Source{{Name: "bluez_capture_internal.A", Bluetooth: true}}}, true},
		{"built-in default", State{DefaultSource: "alsa_input.pci", Sources: []Source{{Name: "alsa_input.pci"}}}, false},
		{"no default", State{Sources: []Source{{Name: "bluez_input.A", Bluetooth: true}}}, false},
		{"default names an absent node", State{DefaultSource: "bluez_input.GONE"}, false},
		{"bluetooth source not the default", State{DefaultSource: "alsa_input.pci", Sources: []Source{{Name: "alsa_input.pci"}, {Name: "bluez_input.A", Bluetooth: true}}}, false},
		// Negative: the name must match the spec, not just a bluez device.
		{"bluetooth node the spec does not match", State{DefaultSource: "bluez_output.A", Sources: []Source{{Name: "bluez_output.A", Bluetooth: true}}}, false},
		{"spec match on a built-in device", State{DefaultSource: "bluez_input.A", Sources: []Source{{Name: "bluez_input.A"}}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.st.DefaultIsBluetooth(btMatches); got != c.want {
				t.Fatalf("DefaultIsBluetooth() = %v, want %v", got, c.want)
			}
		})
	}
}

// TestNodeMatchesFollowsWirePlumberRules: "~" is a regex over the whole name,
// anything else is an exact name.
func TestNodeMatchesFollowsWirePlumberRules(t *testing.T) {
	cases := []struct {
		name     string
		patterns []string
		want     bool
	}{
		{"bluez_input.AA", []string{"~bluez_input.*"}, true},
		{"bluez_input.AA", []string{"bluez_input.AA"}, true},
		{"bluez_input.AA", []string{"bluez_input"}, false}, // exact, not a prefix
		{"bluez_inputX.AA", []string{"~bluez_input.*"}, true},
		{"alsa_input.pci", []string{"~bluez_input.*"}, false},
		{"bluez_input.AA", []string{"~bluez_input["}, false}, // invalid regex matches nothing
		{"bluez_input.AA", nil, false},
	}
	for _, c := range cases {
		if got := NodeMatches(c.name, c.patterns); got != c.want {
			t.Errorf("NodeMatches(%q, %v) = %v, want %v", c.name, c.patterns, got, c.want)
		}
	}
}

// TestDefaultIsBluetoothFromSavedDump checks the predicate on a real saved
// pw-dump with a Bluetooth default (the A2DP-stale fixture).
func TestDefaultIsBluetoothFromSavedDump(t *testing.T) {
	data, err := os.ReadFile("testdata/bt-a2dp-stale-default.json")
	if err != nil {
		t.Fatal(err)
	}
	st, err := ParseDump(data)
	if err != nil {
		t.Fatal(err)
	}
	if !st.DefaultIsBluetooth(btMatches) {
		t.Fatalf("fixture default %q is not detected as Bluetooth", st.DefaultSource)
	}
}
