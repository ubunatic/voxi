package settings

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/deps"
)

// gainCmdDeps runs the settings command against a temp HOME only.
func gainCmdDeps(t *testing.T, home string, stdout *bytes.Buffer) deps.Dependencies {
	t.Helper()
	d := deps.DefaultDependencies(strings.NewReader(""), stdout)
	d.Getenv = func(k string) string {
		if k == "HOME" {
			return home
		}
		return ""
	}
	return d
}

func runSettings(t *testing.T, home string, args ...string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	cmd := NewCommand(gainCmdDeps(t, home, &stdout), nil)
	cmd.SetArgs(args)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	err := cmd.Execute()
	return stdout.String(), err
}

func loadGain(t *testing.T, home string) float64 {
	t.Helper()
	s, err := config.LoadUserSettings(home)
	if err != nil {
		t.Fatal(err)
	}
	return s.BluetoothMicGain
}

func TestSettingsSetBluetoothMicGain(t *testing.T) {
	home := t.TempDir()
	if _, err := runSettings(t, home, "set", "bluetooth_mic_gain", "1.5"); err != nil {
		t.Fatalf("set 1.5: %v", err)
	}
	if got := loadGain(t, home); got != 1.5 {
		t.Fatalf("after set 1.5, gain = %v", got)
	}
	if _, err := runSettings(t, home, "set", "bluetooth_mic_gain", "default"); err != nil {
		t.Fatalf("set default: %v", err)
	}
	if got := loadGain(t, home); got != 0 {
		t.Fatalf("after set default, gain = %v, want unset (0)", got)
	}
}

// Negatives: a bad value is refused and config.yaml keeps the last good gain.
func TestSettingsSetBluetoothMicGainRejectsBadValues(t *testing.T) {
	home := t.TempDir()
	if _, err := runSettings(t, home, "set", "bluetooth_mic_gain", "2"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"17", "0", "-1", "abc", "NaN", "Inf", ""} {
		if _, err := runSettings(t, home, "set", "bluetooth_mic_gain", bad); err == nil {
			t.Errorf("set bluetooth_mic_gain %q was accepted", bad)
		}
		if got := loadGain(t, home); got != 2 {
			t.Fatalf("after rejected %q, gain = %v, want the previous 2", bad, got)
		}
	}
}

func TestSettingsDumpShowsBluetoothMicGain(t *testing.T) {
	home := t.TempDir()
	out, err := runSettings(t, home, "--dump")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Bluetooth Mic Gain:") || !strings.Contains(out, "2.75x (spec default)") {
		t.Fatalf("unset dump lacks the default gain:\n%s", out)
	}

	if _, err := runSettings(t, home, "set", "bluetooth_mic_gain", "1"); err != nil {
		t.Fatal(err)
	}
	out, err = runSettings(t, home, "--dump")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "1x (off)") {
		t.Fatalf("gain 1 dump lacks the off label:\n%s", out)
	}
}

func TestSettingsJSONShowsBluetoothMicGain(t *testing.T) {
	home := t.TempDir()
	out, err := runSettings(t, home, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Gain float64 `json:"bluetooth_mic_gain"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if got.Gain != 2.75 {
		t.Fatalf("unset json bluetooth_mic_gain = %v, want the spec default 2.75", got.Gain)
	}

	if _, err := runSettings(t, home, "set", "bluetooth_mic_gain", "1.5"); err != nil {
		t.Fatal(err)
	}
	out, err = runSettings(t, home, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.Gain != 1.5 {
		t.Fatalf("set json bluetooth_mic_gain = %v, want 1.5", got.Gain)
	}
}

// The menu shows the effective gain and writes a picked choice back; the
// "default" choice clears the override.
func TestMenuBluetoothMicGainRoundTrip(t *testing.T) {
	m := NewMenuModel(&config.UserSettings{BluetoothMicGain: 1.5}, nil)
	item := menuItemByID(t, m, "bluetooth_mic_gain")
	if item.Choices[item.ChoiceIndex] != "1.5" {
		t.Fatalf("menu shows %q, want the override 1.5", item.Choices[item.ChoiceIndex])
	}
	if got := m.ToUserSettings(&config.UserSettings{}).BluetoothMicGain; got != 1.5 {
		t.Fatalf("ToUserSettings gain = %v, want 1.5", got)
	}

	m = NewMenuModel(&config.UserSettings{}, nil)
	item = menuItemByID(t, m, "bluetooth_mic_gain")
	if got := m.ToUserSettings(&config.UserSettings{BluetoothMicGain: 3}).BluetoothMicGain; got != 0 {
		t.Fatalf("default choice gain = %v, want unset (0)", got)
	}
	if !strings.HasPrefix(item.Choices[item.ChoiceIndex], "default") {
		t.Fatalf("unset menu choice = %q, want the default label", item.Choices[item.ChoiceIndex])
	}
}

func menuItemByID(t *testing.T, m *MenuModel, id string) *MenuItem {
	t.Helper()
	for _, it := range m.Items {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("menu has no %q item", id)
	return nil
}
