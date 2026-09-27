package agent

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/eager"
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
	want := []string{"eager", "--daemon", "--model", "saved-model", "--history=false", "--modifier-gating=false"}
	if got := eagerArgs(home); !reflect.DeepEqual(got, want) {
		t.Fatalf("eagerArgs() = %v, want %v", got, want)
	}

	opts := eager.DefaultEagerOptions()
	cmd := eager.NewCommand(func(cmd *cobra.Command, _ []string) error {
		historyEnabled, err := cmd.Flags().GetBool("history")
		if err != nil {
			return err
		}
		modifierEnabled, err := cmd.Flags().GetBool("modifier-gating")
		if err != nil {
			return err
		}
		if historyEnabled || modifierEnabled || !opts.Daemon {
			t.Fatalf("parsed history=%v modifier-gating=%v daemon=%v, want false, false, true", historyEnabled, modifierEnabled, opts.Daemon)
		}
		return nil
	}, &opts)
	cmd.SetArgs(want[1:])
	if err := cmd.Execute(); err != nil {
		t.Fatalf("parse eager child args with Cobra: %v", err)
	}
}
