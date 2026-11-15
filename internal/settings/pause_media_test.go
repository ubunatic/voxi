package settings

import (
	"strings"
	"testing"

	"ubunatic.com/voxi/internal/config"
)

// pause_media (issue 125) is on unless the user turns it off, survives a save
// of an unrelated setting, and shows in the summary.
func TestSetPauseMedia(t *testing.T) {
	home := t.TempDir()
	load := func() *config.UserSettings {
		t.Helper()
		s, err := config.LoadUserSettings(home)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	if !load().PauseMedia {
		t.Fatal("pause_media default = false, want true")
	}
	steps := []struct {
		key, value string
		want       bool
	}{
		{"pause_media", "off", false},
		{"history", "on", false}, // an unrelated save must not switch it back on
		{"pause-media", "on", true},
		{"pause_media", "false", false},
		{"pause_media", "true", true},
	}
	for _, step := range steps {
		if _, err := runSettings(t, home, "set", step.key, step.value); err != nil {
			t.Fatalf("set %s %s: %v", step.key, step.value, err)
		}
		if got := load().PauseMedia; got != step.want {
			t.Errorf("after set %s %s: pause_media = %v, want %v", step.key, step.value, got, step.want)
		}
	}
	if summary := RenderSummary(load(), home); !strings.Contains(summary, "Pause Media:           [✓] Enabled") {
		t.Errorf("summary missing enabled Pause Media:\n%s", summary)
	}
	off := load()
	off.PauseMedia = false
	if summary := RenderSummary(off, home); !strings.Contains(summary, "Pause Media:           [ ] Disabled") {
		t.Errorf("summary missing disabled Pause Media:\n%s", summary)
	}
}
