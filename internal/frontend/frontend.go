// SPDX-FileCopyrightText: 2026 Uwe Jugel
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package frontend is the feature and service layer shared by voxi's front
// ends (the loom terminal UI in examples/voxi-tui and the planned desktop UI).
// It reads the state of the voxi agent, TTS, lmcoder, model servers, settings
// and host media, and runs the few lifecycle actions the front ends offer.
// It opens no terminal and no audio device. Every external boundary is a field
// of Deps, so tests replace it and never touch live services.
package frontend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"ubunatic.com/voxi/internal/agent"
	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/tts"
	"ubunatic.com/voxi/spec"
)

// probeTimeout bounds one status read so a hung socket or tool cannot stall a refresh.
const probeTimeout = 2 * time.Second

// Agent is the part of agent.Client the front ends use.
type Agent interface {
	Status(ctx context.Context) (agent.Status, error)
	Record(ctx context.Context, action agent.RecordAction) (agent.Status, error)
}

// TTS is the part of tts.Client the front ends use.
type TTS interface {
	Snapshot(ctx context.Context) (tts.Snapshot, error)
	Control(ctx context.Context, action tts.Action) error
}

// Runner runs an external command and returns its standard output. A non-zero
// exit returns the output together with the error, as systemctl is-active does.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Prober returns the HTTP status code of a GET request to url.
type Prober func(ctx context.Context, url string) (int, error)

// Deps are the injectable boundaries. NewDeps wires the real ones.
type Deps struct {
	Home     string
	Agent    Agent
	TTS      TTS
	Run      Runner
	Probe    Prober
	Models   func() (*spec.ModelSpec, error)
	Settings func(home string) (*config.UserSettings, error)
	Media    func(ctx context.Context) MediaStatus
}

// Snapshot is one read of everything the front ends display.
type Snapshot struct {
	Agent    AgentStatus
	Mic      MicStatus
	TTS      TTSStatus
	Lmcoder  LmcoderStatus
	ASR      ASRStatus
	Units    []UnitStatus // voxi-owned units, in display order
	Whisper  UnitStatus   // not managed by voxi: status only
	Settings SettingsStatus
	Media    MediaStatus
}

// AgentStatus is the voxi agent's control socket state.
type AgentStatus struct {
	Reachable bool
	Mode      string
	Recording bool
	Err       string
}

// MicStatus is the system default input. Monitor sources are never a default input.
type MicStatus struct {
	Found bool
	Name  string // description shown to the user, e.g. "Ryzen HD Audio Controller Digital Microphone"
	Err   string
}

// TTSStatus is the voxi TTS server state.
type TTSStatus struct {
	Reachable bool
	Status    string
	Current   string
	Err       string
}

// LmcoderStatus reports whether the lmcoder CLI is installed.
type LmcoderStatus struct {
	Found   bool
	Version string
	Err     string
}

// ASRStatus is the selected speech model and whether its endpoint answers.
type ASRStatus struct {
	Model    string
	Endpoint string
	Owner    string // lifecycle tool from spec/models.yaml ("manager"), empty if none
	Ready    bool
	Err      string
}

// UnitStatus is one systemd user unit. Owner is "voxi" for units the front ends
// may start or stop, and "external" for units they only report.
type UnitStatus struct {
	Name    string
	Owner   string
	Active  string // systemctl is-active output: active, inactive, failed, ...
	Enabled string // systemctl is-enabled output, or "not-found"
	Err     string
}

// SettingsStatus is the part of the voxi user settings the front ends show.
type SettingsStatus struct {
	ASRModel   string
	TTSEnabled bool
	TTSBackend string
	Err        string
}

// MediaStatus is the host OS media-control integration for voxi's player.
type MediaStatus struct {
	Available bool
	Status    string // short label, e.g. "unavailable" or "not built for macOS"
	Detail    string // one line of why, for the footer or a message
}

// Collect reads every status once. Each probe has its own timeout and records
// its failure in its own Err field, so one missing tool never hides the rest.
func Collect(ctx context.Context, d Deps) Snapshot {
	var s Snapshot
	s.Agent = d.agentStatus(ctx)
	s.Mic = d.micStatus(ctx)
	s.TTS = d.ttsStatus(ctx)
	s.Lmcoder = d.lmcoderStatus(ctx)
	s.Settings = d.settingsStatus()
	s.ASR = d.asrStatus(ctx, s.Settings)
	s.Units = []UnitStatus{
		d.unitStatus(ctx, "voxi-agent.service", "voxi"),
		d.unitStatus(ctx, "voxi-r2t2.service", "voxi"),
	}
	s.Whisper = d.unitStatus(ctx, "whisper-server.service", "external")
	if d.Media != nil {
		s.Media = d.Media(ctx)
	}
	return s
}

func (d Deps) agentStatus(ctx context.Context) AgentStatus {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	st, err := d.Agent.Status(ctx)
	if err != nil {
		return AgentStatus{Err: err.Error()}
	}
	return AgentStatus{
		Reachable: true,
		Mode:      string(st.Mode),
		Recording: st.Recording == agent.RecordingActive,
	}
}

func (d Deps) ttsStatus(ctx context.Context) TTSStatus {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	snap, err := d.TTS.Snapshot(ctx)
	if err != nil {
		return TTSStatus{Err: err.Error()}
	}
	return TTSStatus{Reachable: true, Status: snap.Status, Current: snap.Current}
}

func (d Deps) micStatus(ctx context.Context) MicStatus {
	name, err := d.runText(ctx, "pactl", "get-default-source")
	if err != nil {
		return MicStatus{Err: "pactl get-default-source: " + err.Error()}
	}
	if name == "" || strings.HasSuffix(name, ".monitor") {
		return MicStatus{Err: "no default input device"}
	}
	list, err := d.runText(ctx, "pactl", "-f", "json", "list", "sources")
	if err != nil {
		return MicStatus{Err: "pactl list sources: " + err.Error()}
	}
	var sources []struct {
		Name          string  `json:"name"`
		Description   string  `json:"description"`
		MonitorOfSink *string `json:"monitor_of_sink"`
	}
	if err := json.Unmarshal([]byte(list), &sources); err != nil {
		return MicStatus{Err: "pactl list sources: unexpected output: " + err.Error()}
	}
	for _, src := range sources {
		if src.Name != name {
			continue
		}
		if src.MonitorOfSink != nil {
			return MicStatus{Err: "default input is a monitor source"}
		}
		return MicStatus{Found: true, Name: src.Description}
	}
	return MicStatus{Err: fmt.Sprintf("default input %q not listed by pactl", name)}
}

func (d Deps) lmcoderStatus(ctx context.Context) LmcoderStatus {
	out, err := d.runText(ctx, "lmcoder", "version")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return LmcoderStatus{Err: "not installed (not on PATH)"}
		}
		return LmcoderStatus{Err: err.Error()}
	}
	fields := strings.Fields(out)
	version := ""
	if len(fields) > 0 {
		version = fields[len(fields)-1]
	}
	return LmcoderStatus{Found: true, Version: version}
}

func (d Deps) settingsStatus() SettingsStatus {
	s, err := d.Settings(d.Home)
	if err != nil {
		return SettingsStatus{Err: err.Error()}
	}
	return SettingsStatus{
		ASRModel:   s.ASRModel,
		TTSEnabled: s.TTSEnabled,
		TTSBackend: s.TTSBackend,
	}
}

func (d Deps) asrStatus(ctx context.Context, settings SettingsStatus) ASRStatus {
	st := ASRStatus{Model: settings.ASRModel}
	if settings.Err != "" {
		st.Err = settings.Err
		return st
	}
	models, err := d.Models()
	if err != nil {
		st.Err = "model spec: " + err.Error()
		return st
	}
	model, ok := models.Models[st.Model]
	if !ok {
		st.Err = fmt.Sprintf("model %q is not in spec/models.yaml", st.Model)
		return st
	}
	st.Owner = model.Manager
	st.Endpoint = model.BaseURL
	if model.BaseURL == "" {
		st.Err = "model has no base_url to probe"
		return st
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	code, err := d.Probe(ctx, strings.TrimRight(model.BaseURL, "/")+"/models")
	switch {
	case err != nil:
		st.Err = "endpoint offline: " + err.Error()
	case code/100 != 2:
		st.Err = fmt.Sprintf("endpoint answered HTTP %d", code)
	default:
		st.Ready = true
	}
	return st
}

func (d Deps) unitStatus(ctx context.Context, unit, owner string) UnitStatus {
	st := UnitStatus{Name: unit, Owner: owner}
	active, err := d.runText(ctx, "systemctl", "--user", "is-active", unit)
	if active == "" && err != nil {
		st.Err = err.Error()
		return st
	}
	st.Active = active
	enabled, err := d.runText(ctx, "systemctl", "--user", "is-enabled", unit)
	if enabled == "" && err != nil {
		st.Err = err.Error()
		return st
	}
	st.Enabled = enabled
	return st
}

// runText runs one command under probeTimeout and returns its trimmed output.
// The error is returned with whatever output the command produced.
func (d Deps) runText(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	out, err := d.Run(ctx, name, args...)
	return strings.TrimSpace(string(out)), err
}
