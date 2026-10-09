// SPDX-FileCopyrightText: 2026 Uwe Jugel
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"ubunatic.com/loom"
	"ubunatic.com/voxi/internal/agent"
	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/frontend"
	"ubunatic.com/voxi/internal/tts"
	"ubunatic.com/voxi/spec"
)

// recAgent records recording actions. Its status calls fail, as with the agent down.
type recAgent struct{ records []agent.RecordAction }

func (a *recAgent) Status(context.Context) (agent.Status, error) {
	return agent.Status{}, agent.ErrUnavailable
}

func (a *recAgent) Record(_ context.Context, action agent.RecordAction) (agent.Status, error) {
	a.records = append(a.records, action)
	return agent.Status{}, nil
}

// recTTS records playback controls.
type recTTS struct{ controls []tts.Action }

func (t *recTTS) Snapshot(context.Context) (tts.Snapshot, error) {
	return tts.Snapshot{}, agent.ErrUnavailable
}

func (t *recTTS) Control(_ context.Context, action tts.Action) error {
	t.controls = append(t.controls, action)
	return nil
}

// testUI builds a ui whose actions run inline and whose commands are recorded.
// lmcoder answers its version check; any other command succeeds with no output.
func testUI(t *testing.T) (*ui, *recAgent, *recTTS, *[]string) {
	t.Helper()
	ag := &recAgent{}
	tt := &recTTS{}
	var calls []string
	deps := frontend.Deps{
		Home:  t.TempDir(),
		Agent: ag,
		TTS:   tt,
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			key := strings.Join(append([]string{name}, args...), " ")
			calls = append(calls, key)
			if key == "lmcoder version" {
				return []byte("lmcoder version 0.1.2\n"), nil
			}
			return nil, nil
		},
		Probe:    func(context.Context, string) (int, error) { return 0, errors.New("offline") },
		Models:   func() (*spec.ModelSpec, error) { return &spec.ModelSpec{}, nil },
		Settings: func(string) (*config.UserSettings, error) { return config.DefaultUserSettings(), nil },
	}
	u := newUI(deps, func() {})
	u.spawn = func(f func()) { f() }
	return u, ag, tt, &calls
}

func key(name string) loom.KeyEvent {
	if len(name) == 1 {
		return loom.KeyEvent{Text: name}
	}
	return loom.KeyEvent{Key: name}
}

func TestTabKeysSelectScreens(t *testing.T) {
	u, _, _, _ := testUI(t)
	for name, want := range map[string]tab{"2": tabServers, "3": tabSettings, "1": tabSpeech} {
		u.ConsumeKey(key(name))
		if u.tab != want {
			t.Errorf("key %q: tab = %d, want %d", name, u.tab, want)
		}
	}
}

func TestQuitKeys(t *testing.T) {
	u, _, _, _ := testUI(t)
	for _, name := range []string{"q", "ctrl-c"} {
		if res := u.ConsumeKey(key(name)); !res.Quit {
			t.Errorf("key %q: Quit = false, want true", name)
		}
	}
	if res := u.ConsumeKey(key("r")); res.Quit {
		t.Errorf("key r on speech must not quit")
	}
}

func TestSpeechKeysDriveAgentAndTTSOnlyOnPress(t *testing.T) {
	u, ag, tt, calls := testUI(t)
	u.ConsumeKey(key("1"))
	if len(ag.records) != 0 || len(tt.controls) != 0 {
		t.Fatalf("selecting a tab must not change state: records=%v controls=%v", ag.records, tt.controls)
	}
	u.ConsumeKey(key("r"))
	u.ConsumeKey(key("x"))
	u.ConsumeKey(key("p"))
	u.ConsumeKey(key("s"))

	if len(ag.records) != 2 || ag.records[0] != agent.RecordStart || ag.records[1] != agent.RecordStop {
		t.Errorf("records = %v, want start then stop", ag.records)
	}
	if len(tt.controls) != 2 || tt.controls[0] != tts.ActionPlayPause || tt.controls[1] != tts.ActionStop {
		t.Errorf("controls = %v, want play-pause then stop", tt.controls)
	}
	for _, c := range *calls {
		if strings.Contains(c, "systemctl") && strings.Contains(c, "lmcoder") {
			t.Errorf("command %q: lmcoder must never go through systemctl", c)
		}
	}
}

func TestServersKeysUseLmcoderAndOwnedUnitOnly(t *testing.T) {
	u, _, _, calls := testUI(t)
	u.ConsumeKey(key("2"))
	u.ConsumeKey(key("s"))
	u.ConsumeKey(key("x"))
	u.ConsumeKey(key("a"))
	u.ConsumeKey(key("d"))

	want := []string{
		"lmcoder service start --asr",
		"lmcoder service stop --asr",
		"systemctl --user start voxi-r2t2.service",
		"systemctl --user stop voxi-r2t2.service",
	}
	for _, w := range want {
		if !contains(*calls, w) {
			t.Errorf("calls = %v, want %q", *calls, w)
		}
	}
	for _, c := range *calls {
		if strings.Contains(c, "systemctl") && strings.Contains(c, "lmcoder") {
			t.Errorf("command %q: lmcoder must never go through systemctl", c)
		}
	}
}

func TestServersStartWithoutLmcoderReportsAndDoesNothing(t *testing.T) {
	u, _, _, calls := testUI(t)
	u.deps.Run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		*calls = append(*calls, strings.Join(append([]string{name}, args...), " "))
		return nil, exec.ErrNotFound
	}
	u.ConsumeKey(key("2"))
	u.ConsumeKey(key("s"))

	if contains(*calls, "lmcoder service start --asr") {
		t.Errorf("calls = %v, lifecycle must not run without lmcoder", *calls)
	}
	u.mu.Lock()
	note := u.note
	u.mu.Unlock()
	if !strings.Contains(note, "lmcoder is not installed") {
		t.Errorf("note = %q, want the missing-lmcoder reason", note)
	}
}

func TestSettingsKeysChangeNothing(t *testing.T) {
	u, ag, tt, calls := testUI(t)
	u.ConsumeKey(key("3"))
	u.ConsumeKey(key("e"))
	u.ConsumeKey(key("r"))

	if len(ag.records) != 0 || len(tt.controls) != 0 || len(*calls) != 0 {
		t.Errorf("settings keys must not run actions: records=%v controls=%v calls=%v", ag.records, tt.controls, *calls)
	}
	u.mu.Lock()
	note := u.note
	u.mu.Unlock()
	if !strings.Contains(note, "not part of this build") {
		t.Errorf("note = %q, want the not-available message", note)
	}
}

func TestNarrowTerminalShowsMessage(t *testing.T) {
	u, _, _, _ := testUI(t)
	rows := loom.Render(u, 40, 10)
	if !strings.Contains(rows[0], "Terminal too small") {
		t.Errorf("row 0 = %q, want the too-small message", rows[0])
	}
	if !strings.Contains(rows[1], "need 72x22, have 40x10") {
		t.Errorf("row 1 = %q, want the required and actual size", rows[1])
	}
}

func TestFullSizeDrawsBoxAndStatusLine(t *testing.T) {
	u, _, _, _ := testUI(t)
	rows := loom.Render(u, 72, screenHeight+noteRows)
	if !strings.Contains(rows[0], "Voxi Speech") {
		t.Errorf("row 0 = %q, want the speech title", rows[0])
	}
	if !strings.Contains(rows[screenHeight-2], "[q] Quit") {
		t.Errorf("footer row = %q, want [q] Quit", rows[screenHeight-2])
	}
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
