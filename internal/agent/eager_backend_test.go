package agent

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
	"ubunatic.com/voxi/internal/config"
)

func TestEagerArgsUseSavedSettings(t *testing.T) {
	home := t.TempDir()
	path := config.VoxiConfigYAMLPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	settings := config.DefaultUserSettings()
	settings.ASRModel = "saved-model"
	settings.DictationHistory = false
	settings.ModifierGating = false
	data, err := yaml.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	want := []string{"eager", "--daemon", "--model", "saved-model", "--history", "false", "--modifier-gating", "false"}
	if got := eagerArgs(home); !reflect.DeepEqual(got, want) {
		t.Fatalf("eagerArgs() = %v, want %v", got, want)
	}
}
