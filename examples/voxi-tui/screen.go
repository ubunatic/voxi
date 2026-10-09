// SPDX-FileCopyrightText: 2026 Uwe Jugel
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"ubunatic.com/voxi/internal/frontend"
)

// screenHeight is the box height of every tab, borders included. The three
// committed mockups in docs/data/voxi-tui-*-001.ansi are this tall.
const screenHeight = 21

// noteRows is the status line drawn below the box for the last action's result.
const noteRows = 1

// tab is one of the three screens; the number keys 1, 2 and 3 select them.
type tab int

const (
	tabSpeech tab = iota
	tabServers
	tabSettings
)

// width is the box width of the tab, matching its committed mockup.
func (t tab) width() int {
	if t == tabServers {
		return 78
	}
	return 72
}

func (t tab) title() string {
	switch t {
	case tabServers:
		return "Voxi Model Servers"
	case tabSettings:
		return "Voxi Settings"
	default:
		return "Voxi Speech"
	}
}

// labelWidth is the column where values start in key/value rows, as in the mockups.
const labelWidth = 18

// navLine is the tab bar. The footer puts [q] Quit at the right edge.
const navLine = "[1] Speech   [2] Servers   [3] Settings"

// screen returns the box for tab t filled with the snapshot s. Every line is
// exactly t.width() runes wide and there are exactly screenHeight lines.
func screen(t tab, s frontend.Snapshot) []string {
	var body []string
	switch t {
	case tabServers:
		body = serversBody(s)
	case tabSettings:
		body = settingsBody(s)
	default:
		body = speechBody(s)
	}
	return frame(t.title(), t.width(), body, footer(t.width()))
}

// frame draws the box: a titled top border, the body rows, the footer row, and
// the bottom border. Body rows beyond the box are cut; short bodies are padded.
func frame(title string, width int, body []string, foot string) []string {
	inner := width - 2
	rows := screenHeight - 3 // top border, footer row and bottom border
	lines := make([]string, 0, screenHeight)
	lines = append(lines, topBorder(title, inner))
	for i := 0; i < rows; i++ {
		text := ""
		if i < len(body) {
			text = body[i]
		}
		lines = append(lines, "│"+pad(text, inner)+"│")
	}
	lines = append(lines, "│"+pad(foot, inner)+"│")
	lines = append(lines, "└"+strings.Repeat("─", inner)+"┘")
	return lines
}

func topBorder(title string, inner int) string {
	label := " " + title + " "
	left := (inner - utf8.RuneCountInString(label)) / 2
	right := inner - utf8.RuneCountInString(label) - left
	return "┌" + strings.Repeat("─", left) + label + strings.Repeat("─", right) + "┐"
}

func footer(width int) string {
	right := "[q] Quit"
	gap := width - 2 - utf8.RuneCountInString(navLine) - utf8.RuneCountInString(right)
	if gap < 1 {
		gap = 1
	}
	return navLine + strings.Repeat(" ", gap) + right
}

// pad fits text to exactly width runes, truncating long text with an ellipsis.
func pad(text string, width int) string {
	n := utf8.RuneCountInString(text)
	if n > width {
		runes := []rune(text)
		return string(runes[:width-1]) + "…"
	}
	return text + strings.Repeat(" ", width-n)
}

// kv formats a label and value on one row, with the value starting at labelWidth.
// A label longer than the column is cut with an ellipsis so the value stays aligned.
func kv(label, value string) string {
	return pad(label, labelWidth) + value
}

func speechBody(s frontend.Snapshot) []string {
	return []string{
		"VOXI  /  SPEECH",
		"Voice input and speech output",
		"",
		"MICROPHONE",
		kv("Default input", micLine(s.Mic)),
		kv("Input level", "["+strings.Repeat("░", 12)+"]  "+listeningLine(s.Agent)),
		"[ r ] Start listening              [ x ] Stop",
		"",
		"SPEECH INPUT",
		kv("ASR model", asrLine(s.ASR)),
		kv("Last transcript", "not exposed by the voxi agent yet"),
		"",
		"SPEECH OUTPUT",
		kv("TTS backend", ttsLine(s)),
		"[ p ] Play / pause                 [ s ] Stop",
		kv("Host media", "OS integration status: "+s.Media.Status),
		"",
		kv("lmcoder", lmcoderSpeechLine(s)),
	}
}

func serversBody(s frontend.Snapshot) []string {
	return []string{
		"VOXI  /  MODEL SERVERS",
		"Lifecycle actions go through each owning tool",
		"",
		"REQUIRED TOOL",
		kv("lmcoder", lmcoderServersLine(s.Lmcoder)),
		"[ i ] Install guidance                 [ r ] Refresh status",
		"",
		"ASR",
		kv(s.ASR.Model, asrServersLine(s.ASR)),
		"[ s ] Start via lmcoder               [ x ] Stop via lmcoder",
		kv("whisper-server", unitLine(s.Whisper)),
		"",
		"VOXI SERVICES",
		kv("voxi-agent", agentLine(s)),
		kv("voxi-r2t2", unitLine(unitAt(s, 1))),
		"[ a ] Start selected                  [ d ] Stop selected",
		"",
		serversProblems(s),
	}
}

func settingsBody(s frontend.Snapshot) []string {
	return []string{
		"VOXI  /  SETTINGS",
		"Manage installed features and speech preferences",
		"",
		"FEATURES",
		fmt.Sprintf("%-18s%s      %s", "Speech input", enabledBox(s.Agent.Reachable), "ASR model: "+orDash(s.Settings.ASRModel)),
		fmt.Sprintf("%-18s%s      %s", "Speech output", enabledBox(s.Settings.TTSEnabled), "TTS backend: "+orDash(s.Settings.TTSBackend)),
		fmt.Sprintf("%-18s%s      %s", "Model servers", enabledBox(s.Lmcoder.Found), "Manager: "+managerName(s)),
		"",
		"AUDIO",
		"Input device      Default system microphone",
		"Output device     Default system speaker",
		"Host media        OS integration status: " + s.Media.Status,
		"",
		"INSTALLATION",
		kv("Voxi", installedLine(s)),
		kv("lmcoder", lmcoderSettingsLine(s.Lmcoder)),
		"[ e ] Edit selected setting             [ r ] Restore defaults",
	}
}

func micLine(m frontend.MicStatus) string {
	if m.Found {
		return "● " + m.Name
	}
	return "○ No default input: " + m.Err
}

func listeningLine(a frontend.AgentStatus) string {
	switch {
	case !a.Reachable:
		return "Agent offline"
	case a.Recording:
		return "Listening on"
	default:
		return "Listening off"
	}
}

func asrLine(a frontend.ASRStatus) string {
	line := orDash(a.Model)
	if a.Owner != "" {
		line += "  ·  via " + a.Owner
	}
	return line
}

func ttsLine(s frontend.Snapshot) string {
	return orDash(s.Settings.TTSBackend) + "  ·  host status shown by voxi"
}

func lmcoderSpeechLine(s frontend.Snapshot) string {
	if !s.Lmcoder.Found {
		return "○ " + s.Lmcoder.Err
	}
	if s.ASR.Ready {
		return "● Found  ·  model service ready"
	}
	return "● Found  ·  model service not answering"
}

func lmcoderServersLine(l frontend.LmcoderStatus) string {
	if !l.Found {
		return "○ Missing  ·  " + l.Err
	}
	return "● Found  ·  version " + l.Version
}

func lmcoderSettingsLine(l frontend.LmcoderStatus) string {
	if !l.Found {
		return "○ Missing  ·  " + l.Err
	}
	return "● Found  ·  required for managed model servers"
}

// asrServersLine keeps the endpoint error out of the row; serversProblems shows it.
func asrServersLine(a frontend.ASRStatus) string {
	owner := "no lifecycle tool listed"
	if a.Owner != "" {
		owner = "owned by " + a.Owner
	}
	if a.Ready {
		return "● Ready  ·  " + owner
	}
	return "○ Offline  ·  " + owner
}

// unitLine shows a unit's state. Its dot follows the systemd state word.
func unitLine(u frontend.UnitStatus) string {
	if u.Name == "" {
		return "○ Unknown"
	}
	dot, word := unitState(u.Active)
	suffix := ""
	switch u.Owner {
	case "voxi":
		suffix = "  ·  voxi-owned"
	case "external":
		suffix = "  ·  not managed by voxi  ·  status only"
	}
	return dot + " " + word + suffix
}

func unitState(active string) (dot, word string) {
	switch active {
	case "active":
		return "●", "Running"
	case "inactive":
		return "○", "Stopped"
	case "failed":
		return "●", "Failed"
	case "activating":
		return "●", "Starting"
	case "":
		return "○", "Unknown"
	default:
		return "○", active
	}
}

// agentLine shows the voxi-agent unit state. An agent that is not answering on
// its socket is reported in the Servers note row, not here.
func agentLine(s frontend.Snapshot) string {
	dot, word := unitState(unitAt(s, 0).Active)
	return dot + " " + word
}

// unitAt returns the i-th voxi unit (0 voxi-agent, 1 voxi-r2t2), or an empty unit.
func unitAt(s frontend.Snapshot, i int) frontend.UnitStatus {
	if i < len(s.Units) {
		return s.Units[i]
	}
	return frontend.UnitStatus{}
}

// serversProblems is the note row near the bottom of the Servers tab. It names
// missing tools and failed reads; with none it keeps the mockup's default text.
func serversProblems(s frontend.Snapshot) string {
	var problems []string
	if !s.Lmcoder.Found {
		problems = append(problems, "lmcoder missing; server controls off")
	}
	if !s.Agent.Reachable {
		problems = append(problems, "agent: "+s.Agent.Err)
	}
	if s.ASR.Err != "" {
		problems = append(problems, "ASR: "+s.ASR.Err)
	}
	if len(problems) == 0 {
		return "Missing tool or service errors appear here; no direct systemctl for lmcoder."
	}
	return strings.Join(problems, "; ")
}

func installedLine(s frontend.Snapshot) string {
	if len(s.Units) > 0 && s.Units[0].Enabled != "" && s.Units[0].Enabled != "not-found" {
		return "● Installed"
	}
	return "○ Not installed"
}

// enabledBox is 11 runes in both states, so the value column does not move.
func enabledBox(on bool) string {
	if on {
		return "[x] Enabled"
	}
	return fmt.Sprintf("%-11s", "[ ] Off")
}

func managerName(s frontend.Snapshot) string {
	if s.ASR.Owner == "" {
		return "none listed"
	}
	return s.ASR.Owner
}

func orDash(text string) string {
	if text == "" {
		return "not set"
	}
	return text
}

// clip cuts text to at most width runes, for the too-small message.
func clip(text string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	return string(runes[:width])
}
