package spec

import (
	"strings"
	"testing"
)

// validBluetoothMic is a complete, valid bluetooth_mic section (issue 210).
const validBluetoothMic = "\nbluetooth_mic: {gain_factor: 6, node_matches: [\"~bluez_input.*\"], " +
	"suspend_timeout_seconds: 0, autoswitch_setting: bluetooth.autoswitch-to-headset-profile, " +
	"dropin_name: 51-voxi-bt-mic-no-suspend.conf, dropin_marker: \"# voxi: keep the Bluetooth headset mic link awake\"}\n"

// validEagerBase is a document with every other required section valid, so a
// rejection is attributable to the bluetooth_mic field under test. It omits
// the bluetooth_mic section; callers append one.
const validEagerBase = "modifier_gate: {timeout_ms: 1000, start_grace_ms: 750, notify_delay_ms: 500}\n" +
	"\ndrain: {delivery_deadline_ms: 60000, injection_timeout_ms: 15000}\n" + validMicCheck

func TestLoadEagerBluetoothMic(t *testing.T) {
	s, err := LoadEager()
	if err != nil {
		t.Fatalf("LoadEager() error = %v", err)
	}
	bm := s.BluetoothMic
	if bm.GainFactor <= 1 || bm.GainFactor > 16 {
		t.Errorf("GainFactor = %v, want a boost in (1, 16]", bm.GainFactor)
	}
	if !contains(bm.NodeMatches, "~bluez_input.*") {
		t.Errorf("NodeMatches = %v, want ~bluez_input.*", bm.NodeMatches)
	}
	if bm.SuspendTimeoutSeconds != 0 {
		t.Errorf("SuspendTimeoutSeconds = %d, want 0 (keep-awake)", bm.SuspendTimeoutSeconds)
	}
	if bm.AutoswitchSetting != "bluetooth.autoswitch-to-headset-profile" {
		t.Errorf("AutoswitchSetting = %q", bm.AutoswitchSetting)
	}
	if bm.DropinName != "51-voxi-bt-mic-no-suspend.conf" {
		t.Errorf("DropinName = %q", bm.DropinName)
	}
	if !strings.HasPrefix(bm.DropinMarker, "# voxi:") {
		t.Errorf("DropinMarker = %q, want a '# voxi:' comment", bm.DropinMarker)
	}
}

func TestParseBluetoothMicAcceptsValid(t *testing.T) {
	if _, err := parseEagerSpec([]byte(validEagerBase + validBluetoothMic)); err != nil {
		t.Fatalf("parseEagerSpec(valid bluetooth_mic) error = %v", err)
	}
}

func TestParseBluetoothMicRejectsBadValues(t *testing.T) {
	cases := map[string]string{
		"zero gain":        `bluetooth_mic: {gain_factor: 0, node_matches: ["~bluez_input.*"], suspend_timeout_seconds: 0, autoswitch_setting: a.b, dropin_name: x.conf, dropin_marker: "# voxi: m"}`,
		"negative gain":    `bluetooth_mic: {gain_factor: -2, node_matches: ["~bluez_input.*"], suspend_timeout_seconds: 0, autoswitch_setting: a.b, dropin_name: x.conf, dropin_marker: "# voxi: m"}`,
		"absurd gain":      `bluetooth_mic: {gain_factor: 100, node_matches: ["~bluez_input.*"], suspend_timeout_seconds: 0, autoswitch_setting: a.b, dropin_name: x.conf, dropin_marker: "# voxi: m"}`,
		"no node matches":  `bluetooth_mic: {gain_factor: 6, node_matches: [], suspend_timeout_seconds: 0, autoswitch_setting: a.b, dropin_name: x.conf, dropin_marker: "# voxi: m"}`,
		"negative suspend": `bluetooth_mic: {gain_factor: 6, node_matches: ["~bluez_input.*"], suspend_timeout_seconds: -1, autoswitch_setting: a.b, dropin_name: x.conf, dropin_marker: "# voxi: m"}`,
		"empty autoswitch": `bluetooth_mic: {gain_factor: 6, node_matches: ["~bluez_input.*"], suspend_timeout_seconds: 0, autoswitch_setting: "", dropin_name: x.conf, dropin_marker: "# voxi: m"}`,
		"dropin path":      `bluetooth_mic: {gain_factor: 6, node_matches: ["~bluez_input.*"], suspend_timeout_seconds: 0, autoswitch_setting: a.b, dropin_name: ../x.conf, dropin_marker: "# voxi: m"}`,
		"empty marker":     `bluetooth_mic: {gain_factor: 6, node_matches: ["~bluez_input.*"], suspend_timeout_seconds: 0, autoswitch_setting: a.b, dropin_name: x.conf, dropin_marker: ""}`,
		"section missing":  "",
	}
	for name, section := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseEagerSpec([]byte(validEagerBase + section)); err == nil {
				t.Fatalf("parseEagerSpec accepted %s", name)
			}
		})
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
