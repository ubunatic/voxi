package mic

import (
	"os"
	"testing"
)

// TestDefaultIsBluetooth covers the predicate that gates digital gain and
// keep-awake (issue 210): only a Bluetooth default source qualifies.
func TestDefaultIsBluetooth(t *testing.T) {
	cases := []struct {
		name string
		st   State
		want bool
	}{
		{"bluetooth input", State{DefaultSource: "bluez_input.A", Sources: []Source{{Name: "bluez_input.A", Bluetooth: true}}}, true},
		{"loopback input", State{DefaultSource: "bluez_input.A", Sources: []Source{{Name: "bluez_input.A", Bluetooth: true, Loopback: true}}}, true},
		{"built-in default", State{DefaultSource: "alsa_input.pci", Sources: []Source{{Name: "alsa_input.pci"}}}, false},
		{"no default", State{Sources: []Source{{Name: "bluez_input.A", Bluetooth: true}}}, false},
		{"default names an absent node", State{DefaultSource: "bluez_input.GONE"}, false},
		{"bluetooth source not the default", State{DefaultSource: "alsa_input.pci", Sources: []Source{{Name: "alsa_input.pci"}, {Name: "bluez_input.A", Bluetooth: true}}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.st.DefaultIsBluetooth(); got != c.want {
				t.Fatalf("DefaultIsBluetooth() = %v, want %v", got, c.want)
			}
		})
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
	if !st.DefaultIsBluetooth() {
		t.Fatalf("fixture default %q is not detected as Bluetooth", st.DefaultSource)
	}
}
