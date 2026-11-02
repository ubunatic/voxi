package config

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ubunatic.com/voxi/spec"
)

// testGainSpec is the spec's bluetooth_mic bounds, read from the real YAML so
// the tests follow spec changes.
func testGainSpec(t *testing.T) spec.BluetoothMicSpec {
	t.Helper()
	es, err := spec.LoadEager()
	if err != nil {
		t.Fatal(err)
	}
	return es.BluetoothMic
}

// writeTestConfig writes config.yaml under a temp home and returns the home.
func writeTestConfig(t *testing.T, body string) string {
	t.Helper()
	home := t.TempDir()
	path := VoxiConfigYAMLPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestBluetoothMicGainUnsetUsesSpecDefault(t *testing.T) {
	home := t.TempDir() // no config.yaml at all
	s, err := LoadUserSettings(home)
	if err != nil {
		t.Fatal(err)
	}
	if s.BluetoothMicGain != 0 {
		t.Fatalf("BluetoothMicGain = %v, want 0 (unset)", s.BluetoothMicGain)
	}
	if got := s.EffectiveBluetoothMicGain(2.75); got != 2.75 {
		t.Fatalf("EffectiveBluetoothMicGain(2.75) = %v, want the spec default", got)
	}
}

func TestBluetoothMicGainOverrideLoadsFromConfig(t *testing.T) {
	home := writeTestConfig(t, "bluetooth_mic_gain: 1.5\n")
	s, err := LoadUserSettings(home)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.EffectiveBluetoothMicGain(2.75); got != 1.5 {
		t.Fatalf("EffectiveBluetoothMicGain = %v, want the override 1.5", got)
	}
}

// 1.0 is a real override (gain off), not "unset".
func TestBluetoothMicGainOneDisablesGain(t *testing.T) {
	home := writeTestConfig(t, "bluetooth_mic_gain: 1.0\n")
	s, err := LoadUserSettings(home)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.EffectiveBluetoothMicGain(2.75); got != 1 {
		t.Fatalf("EffectiveBluetoothMicGain = %v, want 1 (gain disabled)", got)
	}
}

// An unset gain is not written, so the spec default keeps flowing to users who
// never set it; a set gain is written and survives a save/load round trip.
func TestBluetoothMicGainSaveOmitsUnsetAndKeepsSet(t *testing.T) {
	home := t.TempDir()
	unset := DefaultUserSettings()
	if err := SaveUserSettings(home, unset); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(VoxiConfigYAMLPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "bluetooth_mic_gain") {
		t.Fatalf("unset gain was written to config.yaml:\n%s", data)
	}

	set := DefaultUserSettings()
	set.BluetoothMicGain = 2.25
	if err := SaveUserSettings(home, set); err != nil {
		t.Fatal(err)
	}
	s, err := LoadUserSettings(home)
	if err != nil {
		t.Fatal(err)
	}
	if s.BluetoothMicGain != 2.25 {
		t.Fatalf("round trip BluetoothMicGain = %v, want 2.25", s.BluetoothMicGain)
	}
}

func TestEffectiveBluetoothMicGainNilSettings(t *testing.T) {
	var s *UserSettings
	if got := s.EffectiveBluetoothMicGain(2.75); got != 2.75 {
		t.Fatalf("nil settings gain = %v, want the spec default", got)
	}
}

// Negatives: out-of-range values are refused by the validator, whatever the
// spec default is.
func TestValidateBluetoothMicGain(t *testing.T) {
	bm := testGainSpec(t)
	ok := []float64{0.5, 1, 2.75, bm.GainMax}
	for _, g := range ok {
		if err := ValidateBluetoothMicGain(g, bm); err != nil {
			t.Errorf("ValidateBluetoothMicGain(%v) = %v, want nil", g, err)
		}
	}
	bad := []float64{0, -1, bm.GainMax + 0.5, math.NaN(), math.Inf(1)}
	for _, g := range bad {
		if err := ValidateBluetoothMicGain(g, bm); err == nil {
			t.Errorf("ValidateBluetoothMicGain(%v) accepted an out-of-range gain", g)
		}
	}
}
