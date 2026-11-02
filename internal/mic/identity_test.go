package mic

import "testing"

// Detection reads the spec's Bluetooth identity (issue 213), so a node is
// Bluetooth by the same device.api and prefix that the spec names.
func TestBluetoothFlagsComeFromSpec(t *testing.T) {
	dump := `[
	{"type":"PipeWire:Interface:Node","id":7,"info":{"props":{"media.class":"Audio/Source","node.name":"bluez_input.AA","device.api":""}}},
	{"type":"PipeWire:Interface:Node","id":8,"info":{"props":{"media.class":"Audio/Source","node.name":"alsa_input.x","device.api":"bluez5"}}},
	{"type":"PipeWire:Interface:Node","id":9,"info":{"props":{"media.class":"Audio/Source","node.name":"alsa_input.y","device.api":"alsa"}}},
	{"type":"PipeWire:Interface:Device","id":10,"info":{"props":{"media.class":"Audio/Device","device.api":"bluez5"}}}
	]`
	st, err := ParseDump([]byte(dump))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"bluez_input.AA": true, "alsa_input.x": true, "alsa_input.y": false}
	for _, src := range st.Sources {
		if src.Bluetooth != want[src.Name] {
			t.Errorf("source %s: Bluetooth = %v, want %v", src.Name, src.Bluetooth, want[src.Name])
		}
	}
	if dev, ok := st.Device(10); !ok || !dev.Bluetooth {
		t.Errorf("device with device.api bluez5 not Bluetooth: %+v", dev)
	}
	if !IsBluetoothDevice("bluez5") || IsBluetoothDevice("alsa") {
		t.Error("IsBluetoothDevice does not follow the spec device.api")
	}
}
