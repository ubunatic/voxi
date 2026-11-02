package eager

import (
	"os"
	"path/filepath"
	"testing"

	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/spec"
)

// sessionGainDeps points HOME at home, so the test never reads the developer's
// ~/.config/voxi.
func sessionGainDeps(home string) deps.Dependencies {
	return deps.Dependencies{Getenv: func(k string) string {
		if k == "HOME" {
			return home
		}
		return ""
	}}
}

func writeGainConfig(t *testing.T, body string) string {
	t.Helper()
	home := t.TempDir()
	path := config.VoxiConfigYAMLPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestBluetoothGainForSession(t *testing.T) {
	es, err := spec.LoadEager()
	if err != nil {
		t.Fatal(err)
	}
	def := es.BluetoothMic.GainFactor
	cases := map[string]struct {
		home string
		want float64
	}{
		"no config uses spec default":        {t.TempDir(), def},
		"override applies":                   {writeGainConfig(t, "bluetooth_mic_gain: 1.5\n"), 1.5},
		"one disables the gain":              {writeGainConfig(t, "bluetooth_mic_gain: 1.0\n"), 1},
		"out of range falls back to default": {writeGainConfig(t, "bluetooth_mic_gain: 17\n"), def},
		"negative falls back to default":     {writeGainConfig(t, "bluetooth_mic_gain: -2\n"), def},
		"no HOME never reads config":         {"", def},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := bluetoothGainFor(sessionGainDeps(c.home), es); got != c.want {
				t.Fatalf("bluetoothGainFor() = %v, want %v", got, c.want)
			}
		})
	}
}
