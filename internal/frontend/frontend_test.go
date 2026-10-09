// SPDX-FileCopyrightText: 2026 Uwe Jugel
// SPDX-License-Identifier: AGPL-3.0-or-later

package frontend

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"ubunatic.com/voxi/internal/agent"
	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/tts"
	"ubunatic.com/voxi/spec"
)

const (
	micName   = "alsa_input.pci-0000_07_00.6.HiFi__Mic1__source"
	micDesc   = "Ryzen HD Audio Controller Digital Microphone"
	micSinkMo = "alsa_output.pci-0000_07_00.6.HiFi__Speaker__sink.monitor"
)

// fakeAgent records recording actions and returns canned state.
type fakeAgent struct {
	status   agent.Status
	err      error
	recorded []agent.RecordAction
}

func (f *fakeAgent) Status(context.Context) (agent.Status, error) { return f.status, f.err }

func (f *fakeAgent) Record(_ context.Context, action agent.RecordAction) (agent.Status, error) {
	f.recorded = append(f.recorded, action)
	return f.status, f.err
}

// fakeTTS records playback controls and returns a canned snapshot.
type fakeTTS struct {
	snap     tts.Snapshot
	err      error
	controls []tts.Action
}

func (f *fakeTTS) Snapshot(context.Context) (tts.Snapshot, error) { return f.snap, f.err }

func (f *fakeTTS) Control(_ context.Context, action tts.Action) error {
	f.controls = append(f.controls, action)
	return f.err
}

// fakeRunner answers known command lines and fails on anything unplanned, so a
// test that reaches a real or unexpected command fails loudly.
type fakeRunner struct {
	out   map[string]string
	errs  map[string]error
	calls []string
}

func (f *fakeRunner) run(_ context.Context, name string, args ...string) ([]byte, error) {
	key := strings.Join(append([]string{name}, args...), " ")
	f.calls = append(f.calls, key)
	if err, ok := f.errs[key]; ok {
		return []byte(f.out[key]), err
	}
	if out, ok := f.out[key]; ok {
		return []byte(out), nil
	}
	return nil, fmt.Errorf("unexpected command: %s", key)
}

func (f *fakeRunner) calledWith(prefix string) []string {
	var hits []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			hits = append(hits, c)
		}
	}
	return hits
}

// healthyDeps returns Deps for a machine that matches the committed mockups:
// agent up, mic found, lmcoder 0.1.2, ASR endpoint ready, voxi-r2t2 stopped.
func healthyDeps() (Deps, *fakeAgent, *fakeTTS, *fakeRunner) {
	ag := &fakeAgent{status: agent.Status{Mode: agent.ModeEager, Recording: agent.RecordingIdle}}
	tt := &fakeTTS{snap: tts.Snapshot{Status: "idle"}}
	runner := &fakeRunner{
		out: map[string]string{
			"pactl get-default-source":                           micName + "\n",
			"pactl -f json list sources":                         sourcesJSON(),
			"lmcoder version":                                    "lmcoder version 0.1.2\n",
			"systemctl --user is-active voxi-agent.service":      "active\n",
			"systemctl --user is-enabled voxi-agent.service":     "enabled\n",
			"systemctl --user is-active voxi-r2t2.service":       "inactive\n",
			"systemctl --user is-enabled voxi-r2t2.service":      "disabled\n",
			"systemctl --user is-active whisper-server.service":  "active\n",
			"systemctl --user is-enabled whisper-server.service": "enabled\n",
			"lmcoder service start --asr":                        "started\n",
			"lmcoder service stop --asr":                         "stopped\n",
			"systemctl --user start voxi-r2t2.service":           "",
		},
		errs: map[string]error{
			"systemctl --user is-active voxi-r2t2.service":  errors.New("exit status 3"),
			"systemctl --user is-enabled voxi-r2t2.service": errors.New("exit status 1"),
		},
	}
	d := Deps{
		Home:  t0Home,
		Agent: ag,
		TTS:   tt,
		Run:   runner.run,
		Probe: func(context.Context, string) (int, error) { return 200, nil },
		Models: func() (*spec.ModelSpec, error) {
			return &spec.ModelSpec{Models: map[string]spec.Model{
				"r2t2-confucius4": {BaseURL: "http://127.0.0.1:8735/v1", Manager: "lmcoder"},
			}}, nil
		},
		Settings: func(string) (*config.UserSettings, error) {
			return &config.UserSettings{ASRModel: "r2t2-confucius4", TTSEnabled: true, TTSBackend: "voxcpm"}, nil
		},
		Media: func(context.Context) MediaStatus {
			return MediaStatus{Status: "unavailable", Detail: "not on the session bus"}
		},
	}
	return d, ag, tt, runner
}

const t0Home = "/home/test"

func sourcesJSON() string {
	return fmt.Sprintf(`[
  {"name": %q, "description": %q, "monitor_of_sink": null},
  {"name": %q, "description": "Monitor of Ryzen HD Audio Controller Speaker", "monitor_of_sink": %q}
]`, micName, micDesc, micSinkMo, "alsa_output.pci-0000_07_00.6.HiFi__Speaker__sink")
}

func TestCollectHealthy(t *testing.T) {
	d, _, _, _ := healthyDeps()
	var probed []string
	d.Probe = func(_ context.Context, url string) (int, error) {
		probed = append(probed, url)
		return 200, nil
	}
	s := Collect(context.Background(), d)

	if !s.Agent.Reachable || s.Agent.Mode != "eager" || s.Agent.Recording {
		t.Errorf("agent = %+v, want reachable eager idle", s.Agent)
	}
	if !s.Mic.Found || s.Mic.Name != micDesc {
		t.Errorf("mic = %+v, want %q", s.Mic, micDesc)
	}
	if !s.Lmcoder.Found || s.Lmcoder.Version != "0.1.2" {
		t.Errorf("lmcoder = %+v, want found 0.1.2", s.Lmcoder)
	}
	if !s.ASR.Ready || s.ASR.Owner != "lmcoder" || s.ASR.Err != "" {
		t.Errorf("asr = %+v, want ready and owned by lmcoder", s.ASR)
	}
	if len(probed) != 1 || probed[0] != "http://127.0.0.1:8735/v1/models" {
		t.Errorf("probed = %v, want one GET of the model endpoint /models", probed)
	}
	if got := s.Units[0]; got.Name != "voxi-agent.service" || got.Active != "active" {
		t.Errorf("voxi-agent = %+v, want active", got)
	}
	if got := s.Units[1]; got.Name != "voxi-r2t2.service" || got.Active != "inactive" || got.Err != "" {
		t.Errorf("voxi-r2t2 = %+v, want inactive without error (exit 3 is a normal answer)", got)
	}
	if s.Whisper.Owner != "external" || s.Whisper.Active != "active" {
		t.Errorf("whisper = %+v, want external and active", s.Whisper)
	}
	if !s.Settings.TTSEnabled || s.Settings.TTSBackend != "voxcpm" {
		t.Errorf("settings = %+v", s.Settings)
	}
}

func TestCollectLmcoderMissingKeepsOtherStatus(t *testing.T) {
	d, _, _, runner := healthyDeps()
	runner.errs["lmcoder version"] = exec.ErrNotFound
	s := Collect(context.Background(), d)

	if s.Lmcoder.Found || s.Lmcoder.Err != "not installed (not on PATH)" {
		t.Errorf("lmcoder = %+v, want not installed", s.Lmcoder)
	}
	if !s.ASR.Ready || !s.Agent.Reachable {
		t.Errorf("other status must survive a missing lmcoder: asr=%+v agent=%+v", s.ASR, s.Agent)
	}
}

func TestCollectAgentDown(t *testing.T) {
	d, ag, _, _ := healthyDeps()
	ag.err = agent.ErrUnavailable
	s := Collect(context.Background(), d)

	if s.Agent.Reachable || !strings.Contains(s.Agent.Err, "unavailable") {
		t.Errorf("agent = %+v, want unreachable with the unavailable error", s.Agent)
	}
}

func TestCollectNoDefaultMic(t *testing.T) {
	d, _, _, runner := healthyDeps()
	runner.out["pactl get-default-source"] = micSinkMo + "\n"
	s := Collect(context.Background(), d)

	if s.Mic.Found {
		t.Errorf("mic = %+v, a monitor source must not be a default input", s.Mic)
	}
	if s.Mic.Err == "" {
		t.Errorf("mic error empty, want a reason")
	}
}

func TestCollectModelOffline(t *testing.T) {
	d, _, _, _ := healthyDeps()
	d.Probe = func(context.Context, string) (int, error) {
		return 0, errors.New("connect: connection refused")
	}
	s := Collect(context.Background(), d)

	if s.ASR.Ready || !strings.HasPrefix(s.ASR.Err, "endpoint offline") {
		t.Errorf("asr = %+v, want offline with reason", s.ASR)
	}
}

func TestCollectModelNotHTTPOK(t *testing.T) {
	d, _, _, _ := healthyDeps()
	d.Probe = func(context.Context, string) (int, error) { return 503, nil }
	s := Collect(context.Background(), d)

	if s.ASR.Ready || s.ASR.Err != "endpoint answered HTTP 503" {
		t.Errorf("asr = %+v, want HTTP 503 reported", s.ASR)
	}
}

func TestSetASRServiceUsesLmcoderOnly(t *testing.T) {
	d, _, _, runner := healthyDeps()
	if err := d.SetASRService(context.Background(), false); err != nil {
		t.Fatalf("SetASRService(stop) error: %v", err)
	}
	if got := runner.calledWith("lmcoder service stop --asr"); len(got) != 1 {
		t.Errorf("calls = %v, want one lmcoder service stop --asr", runner.calls)
	}
	if got := runner.calledWith("systemctl"); len(got) != 0 {
		t.Errorf("systemctl calls = %v, want none for lmcoder-owned units", got)
	}
}

func TestSetASRServiceMissingLmcoder(t *testing.T) {
	d, _, _, runner := healthyDeps()
	runner.errs["lmcoder version"] = exec.ErrNotFound
	err := d.SetASRService(context.Background(), true)

	if !errors.Is(err, ErrLmcoderMissing) {
		t.Fatalf("err = %v, want ErrLmcoderMissing", err)
	}
	if got := runner.calledWith("lmcoder service"); len(got) != 0 {
		t.Errorf("service calls = %v, want none when lmcoder is missing", got)
	}
}

func TestSetVoxiUnitOnlyStartsOwnedUnits(t *testing.T) {
	d, _, _, runner := healthyDeps()
	if err := d.SetVoxiUnit(context.Background(), "voxi-agent.service", false); !errors.Is(err, ErrNotVoxiOwned) {
		t.Fatalf("voxi-agent err = %v, want ErrNotVoxiOwned", err)
	}
	if got := runner.calledWith("systemctl --user stop"); len(got) != 0 {
		t.Errorf("calls = %v, want no stop of a refused unit", got)
	}
	if err := d.SetVoxiUnit(context.Background(), "voxi-r2t2.service", true); err != nil {
		t.Fatalf("voxi-r2t2 start error: %v", err)
	}
	if got := runner.calledWith("systemctl --user start voxi-r2t2.service"); len(got) != 1 {
		t.Errorf("calls = %v, want one start of voxi-r2t2", runner.calls)
	}
}

func TestSetListeningAndSpeechControl(t *testing.T) {
	d, ag, tt, _ := healthyDeps()
	if _, err := d.SetListening(context.Background(), true); err != nil {
		t.Fatalf("SetListening(true) error: %v", err)
	}
	if _, err := d.SetListening(context.Background(), false); err != nil {
		t.Fatalf("SetListening(false) error: %v", err)
	}
	if len(ag.recorded) != 2 || ag.recorded[0] != agent.RecordStart || ag.recorded[1] != agent.RecordStop {
		t.Errorf("recorded = %v, want start then stop", ag.recorded)
	}
	if err := d.SpeechControl(context.Background(), tts.ActionStop); err != nil {
		t.Fatalf("SpeechControl error: %v", err)
	}
	if len(tt.controls) != 1 || tt.controls[0] != tts.ActionStop {
		t.Errorf("controls = %v, want stop", tt.controls)
	}
}
