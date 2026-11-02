package spec

import (
	"strings"
	"testing"
)

// The Bluetooth identity (issue 213) is spec data: the BlueZ device.api and the
// node-name prefix. The accessors below are the only place Go reads them.
func TestLoadEagerBluetoothIdentity(t *testing.T) {
	s, err := LoadEager()
	if err != nil {
		t.Fatalf("LoadEager() error = %v", err)
	}
	if s.BluetoothMic.DeviceAPI != "bluez5" {
		t.Errorf("DeviceAPI = %q, want bluez5", s.BluetoothMic.DeviceAPI)
	}
	if s.BluetoothMic.NodePrefix != "bluez_" {
		t.Errorf("NodePrefix = %q, want bluez_", s.BluetoothMic.NodePrefix)
	}
}

func TestBluetoothIdentityAccessors(t *testing.T) {
	bm := BluetoothMicSpec{DeviceAPI: "bluez5", NodePrefix: "bluez_"}

	devices := map[string]bool{"bluez5": true, "alsa": false, "": false}
	for api, want := range devices {
		if got := bm.IsBluetoothDevice(api); got != want {
			t.Errorf("IsBluetoothDevice(%q) = %v, want %v", api, got, want)
		}
	}

	nodes := []struct {
		name, api, node string
		want            bool
	}{
		{"bluez api on a plain name", "bluez5", "alsa_input.x", true},
		{"bluez prefix without api", "", "bluez_input.AA", true},
		{"neither api nor prefix", "alsa", "alsa_input.x", false},
		{"prefix needs the underscore", "", "bluezfoo", false},
	}
	for _, c := range nodes {
		if got := bm.IsBluetoothNode(c.api, c.node); got != c.want {
			t.Errorf("%s: IsBluetoothNode(%q, %q) = %v, want %v", c.name, c.api, c.node, got, c.want)
		}
	}
}

// An empty prefix would match every node (strings.HasPrefix(x, "") is true), so
// validation must reject it along with an empty device_api.
func TestParseBluetoothIdentityRejectsEmpty(t *testing.T) {
	const valid = `bluetooth_mic: {device_api: bluez5, node_prefix: bluez_, gain_factor: 2.75, gain_max: 16, node_matches: ["~bluez_input.*"], suspend_timeout_seconds: 0, autoswitch_setting: a.b, dropin_name: x.conf, dropin_marker: "# m", a2dp_profile: a, headset_profile: h, restore_record_prefix: "# r: "}`
	if _, err := parseEagerSpec([]byte(validEagerBase + valid)); err != nil {
		t.Fatalf("valid identity rejected: %v", err)
	}
	cases := map[string]string{
		"empty device_api":  strings.Replace(valid, "device_api: bluez5", `device_api: ""`, 1),
		"empty node_prefix": strings.Replace(valid, "node_prefix: bluez_", `node_prefix: ""`, 1),
	}
	for name, section := range cases {
		if _, err := parseEagerSpec([]byte(validEagerBase + section)); err == nil {
			t.Errorf("%s: parseEagerSpec accepted it, want an error", name)
		}
	}
}
