// SPDX-FileCopyrightText: 2026 Uwe Jugel
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	"ubunatic.com/voxi/internal/frontend"
)

// healthySnapshot is the state the committed mockups show.
func healthySnapshot() frontend.Snapshot {
	return frontend.Snapshot{
		Agent: frontend.AgentStatus{Reachable: true, Mode: "eager"},
		Mic:   frontend.MicStatus{Found: true, Name: "Ryzen HD Audio Controller Digital Microphone"},
		TTS:   frontend.TTSStatus{Reachable: true, Status: "idle"},
		Lmcoder: frontend.LmcoderStatus{
			Found:   true,
			Version: "0.1.2",
		},
		ASR: frontend.ASRStatus{
			Model:    "r2t2-confucius4",
			Endpoint: "http://127.0.0.1:8735/v1",
			Owner:    "lmcoder",
			Ready:    true,
		},
		Units: []frontend.UnitStatus{
			{Name: "voxi-agent.service", Owner: "voxi", Active: "active", Enabled: "enabled"},
			{Name: "voxi-r2t2.service", Owner: "voxi", Active: "inactive", Enabled: "disabled"},
		},
		Whisper:  frontend.UnitStatus{Name: "whisper-server.service", Owner: "external", Active: "active"},
		Settings: frontend.SettingsStatus{ASRModel: "r2t2-confucius4", TTSEnabled: true, TTSBackend: "voxcpm"},
		Media:    frontend.MediaStatus{Status: "unavailable"},
	}
}

func TestScreenShapeMatchesMockups(t *testing.T) {
	for _, tb := range []tab{tabSpeech, tabServers, tabSettings} {
		lines := screen(tb, healthySnapshot())
		if len(lines) != screenHeight {
			t.Errorf("tab %d: %d lines, want %d", tb, len(lines), screenHeight)
		}
		for i, line := range lines {
			if got := utf8.RuneCountInString(line); got != tb.width() {
				t.Errorf("tab %d line %d: %d runes, want %d: %q", tb, i, got, tb.width(), line)
			}
		}
	}
}

func TestScreenStaticLabelsMatchMockups(t *testing.T) {
	cases := []struct {
		tab  tab
		want []string
	}{
		{tabSpeech, []string{
			"VOXI  /  SPEECH",
			"MICROPHONE",
			"SPEECH INPUT",
			"SPEECH OUTPUT",
			"[ r ] Start listening              [ x ] Stop",
			"[ p ] Play / pause                 [ s ] Stop",
			"[1] Speech   [2] Servers   [3] Settings",
			"[q] Quit",
		}},
		{tabServers, []string{
			"VOXI  /  MODEL SERVERS",
			"REQUIRED TOOL",
			"[ i ] Install guidance                 [ r ] Refresh status",
			"[ s ] Start via lmcoder               [ x ] Stop via lmcoder",
			"VOXI SERVICES",
			"[ a ] Start selected                  [ d ] Stop selected",
			"no direct systemctl for lmcoder.",
		}},
		{tabSettings, []string{
			"VOXI  /  SETTINGS",
			"FEATURES",
			"AUDIO",
			"INSTALLATION",
			"[ e ] Edit selected setting             [ r ] Restore defaults",
		}},
	}
	for _, c := range cases {
		text := strings.Join(screen(c.tab, healthySnapshot()), "\n")
		for _, want := range c.want {
			if !strings.Contains(text, want) {
				t.Errorf("tab %d: missing mockup label %q in:\n%s", c.tab, want, text)
			}
		}
	}
}

func TestScreenLiveValues(t *testing.T) {
	text := strings.Join(screen(tabSpeech, healthySnapshot()), "\n")
	for _, want := range []string{
		"● Ryzen HD Audio Controller Digital Microphone",
		"Listening off",
		"r2t2-confucius4  ·  via lmcoder",
		"voxcpm  ·  host status shown by voxi",
		"● Found  ·  model service ready",
		"OS integration status: unavailable",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("speech: missing %q in:\n%s", want, text)
		}
	}

	text = strings.Join(screen(tabServers, healthySnapshot()), "\n")
	for _, want := range []string{
		"● Found  ·  version 0.1.2",
		"● Ready  ·  owned by lmcoder",
		"● Running",
		"○ Stopped  ·  voxi-owned",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("servers: missing %q in:\n%s", want, text)
		}
	}
	if !strings.Contains(text, "● Running  ·  not managed by voxi  ·  status only") {
		t.Errorf("servers: whisper-server row must say it is status only:\n%s", text)
	}
}

func TestScreenDegradedStates(t *testing.T) {
	s := healthySnapshot()
	s.Mic = frontend.MicStatus{Err: "no default input device"}
	s.Agent = frontend.AgentStatus{Err: "voxi agent unavailable"}
	s.Lmcoder = frontend.LmcoderStatus{Err: "not installed (not on PATH)"}
	s.ASR = frontend.ASRStatus{Model: "r2t2-confucius4", Err: "endpoint offline: connection refused"}

	speech := strings.Join(screen(tabSpeech, s), "\n")
	for _, want := range []string{
		"○ No default input: no default input device",
		"Agent offline",
		"○ not installed (not on PATH)",
	} {
		if !strings.Contains(speech, want) {
			t.Errorf("speech degraded: missing %q in:\n%s", want, speech)
		}
	}

	servers := strings.Join(screen(tabServers, s), "\n")
	for _, want := range []string{
		"○ Missing  ·  not installed (not on PATH)",
		"○ Offline  ·  no lifecycle tool listed",
		"lmcoder missing; server controls off",
	} {
		if !strings.Contains(servers, want) {
			t.Errorf("servers degraded: missing %q in:\n%s", want, servers)
		}
	}
}

func TestSettingsBoxesReflectState(t *testing.T) {
	s := healthySnapshot()
	s.Settings.TTSEnabled = false
	text := strings.Join(screen(tabSettings, s), "\n")
	if !strings.Contains(text, "Speech output     [ ] Off          TTS backend") {
		t.Errorf("settings: TTS off must show Off with the value column in place:\n%s", text)
	}
	if !strings.Contains(text, "Speech input      [x] Enabled") {
		t.Errorf("settings: agent up must show speech input enabled:\n%s", text)
	}
}

func TestLongLabelKeepsValueColumn(t *testing.T) {
	row := kv("r2t2-confucius4-voxi", "○ Offline")
	idx := strings.Index(row, "○")
	if idx < 0 {
		t.Fatalf("value missing from row %q", row)
	}
	if got := utf8.RuneCountInString(row[:idx]); got != labelWidth {
		t.Errorf("value starts at column %d, want %d: %q", got, labelWidth, row)
	}
}

func TestFooterKeepsQuitAtRightEdge(t *testing.T) {
	for _, tb := range []tab{tabSpeech, tabServers, tabSettings} {
		lines := screen(tb, healthySnapshot())
		foot := lines[len(lines)-2]
		if !strings.HasSuffix(strings.TrimSuffix(foot, "│"), "[q] Quit") {
			t.Errorf("tab %d footer %q: [q] Quit not at right edge", tb, foot)
		}
	}
}
