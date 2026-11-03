package spec

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"
)

// validBluetoothMic is a complete, valid bluetooth_mic section (issue 210).
const validBluetoothMic = "\nbluetooth_mic: {device_api: bluez5, node_prefix: bluez_, gain_factor: 2.75, gain_max: 16, node_matches: [\"~bluez_input.*\"], " +
	"suspend_timeout_seconds: 0, autoswitch_setting: bluetooth.autoswitch-to-headset-profile, " +
	"dropin_name: 51-voxi-bt-mic-no-suspend.conf, dropin_marker: \"# managed-by: voxi mic headset\", " +
	"a2dp_profile: a2dp-sink, headset_profile: headset-head-unit, restore_record_prefix: \"# voxi-restore-autoswitch: \"}\n"

// validEagerBase is a document with every other required section valid, so a
// rejection is attributable to the bluetooth_mic field under test. It omits
// the bluetooth_mic section; callers append one.
const validEagerBase = "energy_gate: {threshold_rms: 150, min_mean_rms: 120, min_voiced_frames: 8, min_voiced_run_frames: 7}\n" +
	"modifier_gate: {timeout_ms: 1000, start_grace_ms: 750, notify_delay_ms: 500}\n" +
	"\ndrain: {delivery_deadline_ms: 60000, injection_timeout_ms: 15000}\n" + validMicCheck

func TestLoadEagerBluetoothMic(t *testing.T) {
	s, err := LoadEager()
	if err != nil {
		t.Fatalf("LoadEager() error = %v", err)
	}
	bm := s.BluetoothMic
	if bm.GainMax != 16 {
		t.Errorf("GainMax = %v, want 16 (the user override ceiling, equal to the schema maximum)", bm.GainMax)
	}
	if bm.GainFactor != 2.75 {
		t.Errorf("GainFactor = %v, want 2.75 (1.4 wpctl volume on the cubic scale, issue 210 M4)", bm.GainFactor)
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
	if bm.DropinMarker != "# managed-by: voxi mic headset" {
		t.Errorf("DropinMarker = %q, want the exact ownership token", bm.DropinMarker)
	}
	if bm.A2DPProfile != "a2dp-sink" || bm.HeadsetProfile != "headset-head-unit" {
		t.Errorf("profiles = %q / %q", bm.A2DPProfile, bm.HeadsetProfile)
	}
	if bm.RestoreRecordPrefix != "# voxi-restore-autoswitch: " {
		t.Errorf("RestoreRecordPrefix = %q", bm.RestoreRecordPrefix)
	}
}

func TestParseBluetoothMicAcceptsValid(t *testing.T) {
	if _, err := parseEagerSpec([]byte(validEagerBase + validBluetoothMic)); err != nil {
		t.Fatalf("parseEagerSpec(valid bluetooth_mic) error = %v", err)
	}
}

// Negatives. The gain ceiling is enforced by the schema, not Go (issue 210 M4);
// TestEagerSchemaBoundsBluetoothMic covers it.
func TestParseBluetoothMicRejectsBadValues(t *testing.T) {
	const tail = `, suspend_timeout_seconds: 0, autoswitch_setting: a.b, dropin_name: x.conf, dropin_marker: "# managed-by: voxi mic headset", a2dp_profile: a2dp-sink, headset_profile: headset-head-unit, restore_record_prefix: "# r: "}`
	cases := map[string]string{
		"zero gain":        `bluetooth_mic: {device_api: bluez5, node_prefix: bluez_, gain_factor: 0, gain_max: 16, node_matches: ["~bluez_input.*"]` + tail,
		"zero gain max":    `bluetooth_mic: {device_api: bluez5, node_prefix: bluez_, gain_factor: 2.75, gain_max: 0, node_matches: ["~bluez_input.*"]` + tail,
		"gain above max":   `bluetooth_mic: {device_api: bluez5, node_prefix: bluez_, gain_factor: 20, gain_max: 16, node_matches: ["~bluez_input.*"]` + tail,
		"negative gain":    `bluetooth_mic: {device_api: bluez5, node_prefix: bluez_, gain_factor: -2, gain_max: 16, node_matches: ["~bluez_input.*"]` + tail,
		"no node matches":  `bluetooth_mic: {device_api: bluez5, node_prefix: bluez_, gain_factor: 2.75, gain_max: 16, node_matches: []` + tail,
		"negative suspend": `bluetooth_mic: {device_api: bluez5, node_prefix: bluez_, gain_factor: 2.75, node_matches: ["~bluez_input.*"], suspend_timeout_seconds: -1, autoswitch_setting: a.b, dropin_name: x.conf, dropin_marker: "# m", a2dp_profile: a, headset_profile: h, restore_record_prefix: "# r: "}`,
		"empty autoswitch": `bluetooth_mic: {device_api: bluez5, node_prefix: bluez_, gain_factor: 2.75, node_matches: ["~bluez_input.*"], suspend_timeout_seconds: 0, autoswitch_setting: "", dropin_name: x.conf, dropin_marker: "# m", a2dp_profile: a, headset_profile: h, restore_record_prefix: "# r: "}`,
		"dropin path":      `bluetooth_mic: {device_api: bluez5, node_prefix: bluez_, gain_factor: 2.75, node_matches: ["~bluez_input.*"], suspend_timeout_seconds: 0, autoswitch_setting: a.b, dropin_name: ../x.conf, dropin_marker: "# m", a2dp_profile: a, headset_profile: h, restore_record_prefix: "# r: "}`,
		"empty marker":     `bluetooth_mic: {device_api: bluez5, node_prefix: bluez_, gain_factor: 2.75, node_matches: ["~bluez_input.*"], suspend_timeout_seconds: 0, autoswitch_setting: a.b, dropin_name: x.conf, dropin_marker: "", a2dp_profile: a, headset_profile: h, restore_record_prefix: "# r: "}`,
		"empty a2dp":       `bluetooth_mic: {device_api: bluez5, node_prefix: bluez_, gain_factor: 2.75, node_matches: ["~bluez_input.*"], suspend_timeout_seconds: 0, autoswitch_setting: a.b, dropin_name: x.conf, dropin_marker: "# m", a2dp_profile: "", headset_profile: h, restore_record_prefix: "# r: "}`,
		"empty headset":    `bluetooth_mic: {device_api: bluez5, node_prefix: bluez_, gain_factor: 2.75, node_matches: ["~bluez_input.*"], suspend_timeout_seconds: 0, autoswitch_setting: a.b, dropin_name: x.conf, dropin_marker: "# m", a2dp_profile: a, headset_profile: "", restore_record_prefix: "# r: "}`,
		"empty record":     `bluetooth_mic: {device_api: bluez5, node_prefix: bluez_, gain_factor: 2.75, node_matches: ["~bluez_input.*"], suspend_timeout_seconds: 0, autoswitch_setting: a.b, dropin_name: x.conf, dropin_marker: "# m", a2dp_profile: a, headset_profile: h, restore_record_prefix: ""}`,
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

// TestEagerSchemaBoundsBluetoothMic checks the gain ceiling and other bounds in
// spec/schemas/eager.schema.json against mutated copies of the real spec.
func TestEagerSchemaBoundsBluetoothMic(t *testing.T) {
	schemaPath := "schemas/eager.schema.json"
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft7
	compiler.AssertFormat = true
	if err := compiler.AddResource(schemaPath, bytes.NewReader(schemaBytes)); err != nil {
		t.Fatal(err)
	}
	sch, err := compiler.Compile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	yamlBytes, err := os.ReadFile("eager.yaml")
	if err != nil {
		t.Fatal(err)
	}
	validate := func(mutate func(bm map[string]any)) error {
		var doc map[string]any
		if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
			t.Fatal(err)
		}
		mutate(doc["bluetooth_mic"].(map[string]any))
		data, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		var obj any
		if err := json.Unmarshal(data, &obj); err != nil {
			t.Fatal(err)
		}
		return sch.Validate(obj)
	}
	if err := validate(func(bm map[string]any) {}); err != nil {
		t.Fatalf("real spec fails its schema: %v", err)
	}
	cases := map[string]func(bm map[string]any){
		"gain above maximum": func(bm map[string]any) { bm["gain_factor"] = 100.0 },
		"gain zero":          func(bm map[string]any) { bm["gain_factor"] = 0.0 },
		"negative suspend":   func(bm map[string]any) { bm["suspend_timeout_seconds"] = -1 },
	}
	for name, mutate := range cases {
		if err := validate(mutate); err == nil {
			t.Errorf("schema accepted %s", name)
		}
	}
}

// TestBluetoothMicGainAllowed checks the user override range against the
// spec's gain_max (issue 210 M5): (0, gain_max], NaN never allowed.
func TestBluetoothMicGainAllowed(t *testing.T) {
	bm := BluetoothMicSpec{GainFactor: 2.75, GainMax: 16}
	cases := map[float64]bool{
		0:           false,
		-1:          false,
		math.NaN():  false,
		math.Inf(1): false,
		0.5:         true,
		1:           true,
		2.75:        true,
		16:          true,
		16.0001:     false,
	}
	for g, want := range cases {
		if got := bm.GainAllowed(g); got != want {
			t.Errorf("GainAllowed(%v) = %v, want %v", g, got, want)
		}
	}
}

// TestSchemaGainMaximumIsSpecGainMax keeps the schema's gain_factor maximum and
// the spec's gain_max (the user override ceiling) from drifting apart.
func TestSchemaGainMaximumIsSpecGainMax(t *testing.T) {
	s, err := LoadEager()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("schemas/eager.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties map[string]struct {
			Properties map[string]struct {
				Maximum float64 `json:"maximum"`
			} `json:"properties"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	max := schema.Properties["bluetooth_mic"].Properties["gain_factor"].Maximum
	if max != s.BluetoothMic.GainMax {
		t.Fatalf("schema gain_factor maximum = %v, spec gain_max = %v; keep them equal", max, s.BluetoothMic.GainMax)
	}
}
