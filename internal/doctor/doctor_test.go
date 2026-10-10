package doctor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"ubunatic.com/voxi/spec"
)

type commandResult struct {
	output string
	err    error
}

type fixtureHost struct {
	run      func(string, ...string) commandResult
	routing  string
	linkFile string
	routeErr error
	calls    []string
	applied  []string
}

func (h *fixtureHost) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	h.calls = append(h.calls, name+" "+strings.Join(args, " "))
	if h.run == nil {
		return nil, fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
	}
	r := h.run(name, args...)
	return []byte(r.output), r.err
}

func (h *fixtureHost) ProbeRouting(_ context.Context, cfg spec.DoctorRouting) (string, error) {
	h.calls = append(h.calls, "ProbeRouting")
	if h.linkFile != "" {
		data, err := os.ReadFile(h.linkFile)
		if err != nil {
			return "", err
		}
		return parseProbeTarget(string(data), cfg.ProbeNode), nil
	}
	return h.routing, h.routeErr
}

func (h *fixtureHost) Apply(_ context.Context, command string) error {
	h.applied = append(h.applied, command)
	h.calls = append(h.calls, "APPLY "+command)
	return nil
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("testdata", name)
}

func loadDoctorSpec(t *testing.T) *spec.DoctorSpec {
	t.Helper()
	s, err := spec.LoadDoctor()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func resultFor(t *testing.T, id string, status Status, detail string, repairs ...string) Result {
	t.Helper()
	return Result{ID: id, Status: status, Detail: detail, Repairs: repairs}
}

func TestCheckIDsMatchSpec(t *testing.T) {
	s := loadDoctorSpec(t)
	want := CheckIDs()
	got := make([]string, 0, len(s.Checks))
	for _, check := range s.Checks {
		got = append(got, check.ID)
	}
	sort.Strings(want)
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("spec check IDs = %v, Go implementations = %v", got, want)
	}
}

func checkFor(t *testing.T, s *spec.DoctorSpec, id string) spec.DoctorCheck {
	t.Helper()
	for _, check := range s.Checks {
		if check.ID == id {
			return check
		}
	}
	t.Fatalf("missing check %q", id)
	return spec.DoctorCheck{}
}

func TestRepairWizardDeclineAndSnapshot(t *testing.T) {
	s := loadDoctorSpec(t)
	command := "pactl set-card-profile bluez_card.00_11_22_33_44_55 a2dp-sink"
	h := &fixtureHost{run: func(name string, _ ...string) commandResult {
		if name == "pw-dump" {
			b, err := os.ReadFile(fixture(t, "pw-dump-dangling-bt-sink.json"))
			if err != nil {
				return commandResult{err: err}
			}
			return commandResult{output: string(b)}
		}
		return commandResult{err: errors.New("unexpected command")}
	}}
	base := Report{Checks: []Result{{ID: "bt-card-profile-off", Status: FAULT, Detail: "fault", Repairs: []string{command}}}}
	for _, tt := range []struct {
		name        string
		answer      string
		interactive bool
		yes         bool
		wantApply   bool
	}{
		{"declined", "n\n", true, false, false},
		{"non-terminal cannot apply", "y\n", false, false, false},
		{"accepted", "y\n", true, false, true},
		{"yes applies without prompt", "", false, true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h.applied = nil
			stateDir := t.TempDir()
			out := &bytes.Buffer{}
			report, err := (HostDoctor{Host: h, Spec: s}).Repair(context.Background(), base, Options{Fix: true, PWDump: fixture(t, "pw-dump-dangling-bt-sink.json"), StateDir: stateDir}, strings.NewReader(tt.answer), out, tt.yes, tt.interactive)
			if err != nil {
				t.Fatal(err)
			}
			if (len(h.applied) > 0) != tt.wantApply {
				t.Fatalf("applied %v, want apply=%v", h.applied, tt.wantApply)
			}
			if tt.wantApply {
				if report.Snapshot == "" || !strings.HasPrefix(report.Snapshot, stateDir) {
					t.Fatalf("snapshot path %q not in %q", report.Snapshot, stateDir)
				}
				if b, err := os.ReadFile(report.Snapshot); err != nil || len(b) == 0 {
					t.Fatalf("snapshot read error=%v bytes=%d", err, len(b))
				}
			}
		})
	}
}

func TestPrototypeCases(t *testing.T) {
	s := loadDoctorSpec(t)
	ctx := context.Background()
	doctor := func(h Host) HostDoctor { return HostDoctor{Host: h, Spec: s} }
	load := func(id, file string, h Host) Result {
		path := ""
		if file == "garbage.json" {
			path = filepath.Join(t.TempDir(), file)
			if err := os.WriteFile(path, []byte("not json\n"), 0600); err != nil {
				t.Fatal(err)
			}
		} else if file != "" {
			path = fixture(t, file)
		}
		for _, c := range s.Checks {
			if c.ID != id {
				continue
			}
			if id == "default-devices" {
				return doctor(h).checkDefaults(ctx, c, path)
			}
			return doctor(h).checkBluetooth(ctx, c, path)
		}
		t.Fatalf("missing spec check %q", id)
		return Result{}
	}
	noTool := &exec.Error{Name: "pw-dump", Err: exec.ErrNotFound}

	tests := []struct {
		name string
		want Result
		run  func() Result
	}{
		{"01 missing systemctl", resultFor(t, "services", SKIP, "systemctl is not installed, cannot check"), func() Result {
			return doctor(&fixtureHost{run: func(string, ...string) commandResult {
				return commandResult{err: &exec.Error{Name: "systemctl", Err: exec.ErrNotFound}}
			}}).checkServices(ctx, s.Checks[0])
		}},
		{"02 missing pw-dump binary", resultFor(t, "default-devices", SKIP, "pw-dump failed, PipeWire is not reachable"), func() Result {
			return load("default-devices", "", &fixtureHost{run: func(string, ...string) commandResult { return commandResult{err: noTool} }})
		}},
		{"02 Go JSON parser needs no jq", resultFor(t, "default-devices", OK, "default sink 'bluez_output.00_11_22_33_44_55.1' and source 'alsa_input.pci-0000_07_00.6.HiFi__Mic1__source' both exist"), func() Result {
			return load("default-devices", "pw-dump-healthy.json", &fixtureHost{})
		}},
		{"03 Go JSON parser needs no jq", resultFor(t, "bt-card-profile-off", OK, "bluez_card.00_11_22_33_44_55 on profile a2dp-sink"), func() Result {
			return load("bt-card-profile-off", "pw-dump-healthy.json", &fixtureHost{})
		}},
		{"02 invalid pw-dump JSON", resultFor(t, "default-devices", SKIP, "no pw-dump JSON array"), func() Result {
			return load("default-devices", "garbage.json", &fixtureHost{})
		}},
		{"02 dangling default sink faults", resultFor(t, "default-devices", FAULT, "default.audio.sink names 'bluez_output.00_11_22_33_44_55.1', which no node has", "pw-metadata 0 default.audio.sink '{\"name\":\"alsa_output.pci-0000_07_00.6.HiFi__Speaker__sink\"}' Spa:String:JSON"), func() Result {
			return load("default-devices", "pw-dump-dangling-bt-sink.json", &fixtureHost{})
		}},
		{"02 active repair uses configured sink", resultFor(t, "default-devices", FAULT, "default.audio.sink names 'bluez_output.00_11_22_33_44_55.1', which no node has", "pw-metadata 0 default.audio.sink '{\"name\":\"alsa_output.pci-0000_07_00.6.HiFi__Speaker__sink\"}' Spa:String:JSON"), func() Result {
			return load("default-devices", "pw-dump-dangling-bt-sink.json", &fixtureHost{})
		}},
		{"02 dangling active repair avoids wpctl", resultFor(t, "default-devices", FAULT, "default.audio.sink names 'bluez_output.00_11_22_33_44_55.1', which no node has", "pw-metadata 0 default.audio.sink '{\"name\":\"alsa_output.pci-0000_07_00.6.HiFi__Speaker__sink\"}' Spa:String:JSON"), func() Result {
			return load("default-devices", "pw-dump-dangling-bt-sink.json", &fixtureHost{})
		}},
		{"03 Bluetooth off with a2dp faults", resultFor(t, "bt-card-profile-off", FAULT, "bluez_card.00_11_22_33_44_55 is off, but profiles are available: a2dp-sink,a2dp-sink-sbc_xq,headset-head-unit", "pactl set-card-profile bluez_card.00_11_22_33_44_55 a2dp-sink"), func() Result {
			return load("bt-card-profile-off", "pw-dump-dangling-bt-sink.json", &fixtureHost{})
		}},
		{"03 repair selects first usable profile", resultFor(t, "bt-card-profile-off", FAULT, "bluez_card.00_11_22_33_44_55 is off, but profiles are available: a2dp-sink,a2dp-sink-sbc_xq,headset-head-unit", "pactl set-card-profile bluez_card.00_11_22_33_44_55 a2dp-sink"), func() Result {
			return load("bt-card-profile-off", "pw-dump-dangling-bt-sink.json", &fixtureHost{})
		}},
		{"02 healthy defaults", resultFor(t, "default-devices", OK, "default sink 'bluez_output.00_11_22_33_44_55.1' and source 'alsa_input.pci-0000_07_00.6.HiFi__Mic1__source' both exist"), func() Result {
			return load("default-devices", "pw-dump-healthy.json", &fixtureHost{})
		}},
		{"03 healthy Bluetooth profile", resultFor(t, "bt-card-profile-off", OK, "bluez_card.00_11_22_33_44_55 on profile a2dp-sink"), func() Result {
			return load("bt-card-profile-off", "pw-dump-healthy.json", &fixtureHost{})
		}},
		{"03 off without alternative is OK", resultFor(t, "bt-card-profile-off", OK, "bluez_card.00_11_22_33_44_55 is off with no usable profile, not a fault"), func() Result {
			return load("bt-card-profile-off", "pw-dump-bt-off-no-alternative.json", &fixtureHost{})
		}},
		{"02 empty active default faults", resultFor(t, "default-devices", FAULT, "default.audio.sink is empty (no default recorded)", "wpctl status"), func() Result {
			return load("default-devices", "pw-dump-empty-default-sink.json", &fixtureHost{})
		}},
		{"02 missing configured source faults", resultFor(t, "default-devices", FAULT, "default.configured.audio.source names 'alsa_input.usb-SunplusIT_Inc_Depstech_webcam_TESTSERIAL-02.analog-stereo', which is gone", "wpctl set-default 66"), func() Result {
			r := load("default-devices", "pw-dump-configured-source-gone.json", &fixtureHost{})
			return r
		}},
		{"02 missing configured sink falls back to first sink", Result{ID: "default-devices", Status: FAULT, Detail: "", Repairs: []string{"pw-metadata 0 default.audio.sink '{\"name\":\"alsa_output.pci-0000_07_00.1.HiFi__HDMI1__sink\"}' Spa:String:JSON", "wpctl set-default 77"}}, func() Result {
			return load("default-devices", "pw-dump-configured-sink-gone-hdmi.json", &fixtureHost{})
		}},
		{"02 configured sink repair uses fallback id", Result{ID: "default-devices", Status: FAULT, Detail: "", Repairs: []string{"pw-metadata 0 default.audio.sink '{\"name\":\"alsa_output.pci-0000_07_00.1.HiFi__HDMI1__sink\"}' Spa:String:JSON", "wpctl set-default 77"}}, func() Result {
			return load("default-devices", "pw-dump-configured-sink-gone-hdmi.json", &fixtureHost{})
		}},
		{"04 since controls the journal window", resultFor(t, "journal-signals", OK, "no audio failure signature in the log since 2099-01-01; sleep: none"), func() Result {
			return doctor(&fixtureHost{run: func(name string, args ...string) commandResult {
				if name == "journalctl" {
					return commandResult{}
				}
				return commandResult{err: errors.New("unexpected command")}
			}}).checkJournal(ctx, checkFor(t, s, "journal-signals"), "2099-01-01")
		}},
		{"06 missing probe tools skips", resultFor(t, "stream-routing", SKIP, "pw-play/pw-link is not installed, cannot check"), func() Result {
			return doctor(&fixtureHost{routeErr: &exec.Error{Name: "pw-play", Err: exec.ErrNotFound}}).checkRouting(ctx, checkFor(t, s, "stream-routing"), false)
		}},
		{"06 no probe skips", resultFor(t, "stream-routing", SKIP, "--no-probe given, the stream probe was not run"), func() Result {
			return doctor(&fixtureHost{}).checkRouting(ctx, checkFor(t, s, "stream-routing"), true)
		}},
		{"06 linked probe is OK", resultFor(t, "stream-routing", OK, "probe stream linked to sink 'alsa_output.pci-0000_07_00.6.HiFi__Speaker__sink'"), func() Result {
			return doctor(&fixtureHost{linkFile: fixture(t, "pw-link-linked.txt")}).checkRouting(ctx, checkFor(t, s, "stream-routing"), false)
		}},
		{"06 unlinked probe faults", resultFor(t, "stream-routing", FAULT, "a new playback stream was not linked to any sink (WirePlumber is not routing streams)", "systemctl --user restart wireplumber"), func() Result {
			return doctor(&fixtureHost{linkFile: fixture(t, "pw-link-unlinked.txt")}).checkRouting(ctx, checkFor(t, s, "stream-routing"), false)
		}},
		{"06 repair restarts WirePlumber", resultFor(t, "stream-routing", FAULT, "a new playback stream was not linked to any sink (WirePlumber is not routing streams)", "systemctl --user restart wireplumber"), func() Result {
			return doctor(&fixtureHost{}).checkRouting(ctx, checkFor(t, s, "stream-routing"), false)
		}},
		{"aggregate report faults on dangling state", Result{ID: "aggregate", Status: FAULT, Detail: "faults=2"}, func() Result {
			h := healthyHost(t)
			dump := fixture(t, "pw-dump-dangling-bt-sink.json")
			r, err := doctor(h).Run(ctx, Options{PWDump: dump, NoProbe: true})
			if err != nil || r.Fault != 2 {
				t.Fatalf("Run() = (%+v, %v), want two faults", r, err)
			}
			return Result{ID: "aggregate", Status: FAULT, Detail: fmt.Sprintf("faults=%d", r.Fault)}
		}},
		{"report summary shape", Result{ID: "summary", Detail: "Summary: 0 OK, 2 FAULT, 4 SKIP, 0 WARN, 0 ERROR"}, func() Result {
			var b bytes.Buffer
			r := Report{Fault: 2, Skip: 4}
			r.Print(&b)
			return Result{ID: "summary", Detail: strings.Split(strings.TrimSpace(b.String()), "\n")[0]}
		}},
		{"no-probe is included in aggregate output", Result{ID: "summary", Detail: "SKIP stream-routing: --no-probe given, the stream probe was not run"}, func() Result {
			h := healthyHost(t)
			r, err := doctor(h).Run(ctx, Options{PWDump: fixture(t, "pw-dump-healthy.json"), NoProbe: true, Since: "2099-01-01"})
			if err != nil {
				t.Fatal(err)
			}
			var b bytes.Buffer
			r.Print(&b)
			for _, line := range strings.Split(b.String(), "\n") {
				if strings.HasPrefix(line, "SKIP stream-routing:") {
					return Result{ID: "summary", Detail: line}
				}
			}
			return Result{ID: "summary", Detail: "missing stream-routing skip"}
		}},
		{"repairs are printed and never executed", Result{ID: "read-only", Detail: "no repair command invoked"}, func() Result {
			h := healthyHost(t)
			_, err := doctor(h).Run(ctx, Options{PWDump: fixture(t, "pw-dump-dangling-bt-sink.json"), NoProbe: true, Since: "2099-01-01"})
			if err != nil {
				t.Fatal(err)
			}
			for _, call := range h.calls {
				if strings.Contains(call, "pw-metadata") || strings.Contains(call, "wpctl set-") || strings.Contains(call, "pactl set-") || strings.Contains(call, "restart") {
					return Result{ID: "read-only", Detail: "repair command was executed: " + call}
				}
			}
			return Result{ID: "read-only", Detail: "no repair command invoked"}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.run()
			if got.ID != tt.want.ID || got.Status != tt.want.Status || !strings.Contains(got.Detail, tt.want.Detail) {
				t.Errorf("got %#v\nwant ID/status/detail containing %#v", got, tt.want)
			}
			if tt.want.Repairs != nil && !reflect.DeepEqual(got.Repairs, tt.want.Repairs) {
				t.Errorf("repairs = %#v, want %#v", got.Repairs, tt.want.Repairs)
			}
		})
	}
}

func healthyHost(t *testing.T) *fixtureHost {
	t.Helper()
	return &fixtureHost{run: func(name string, args ...string) commandResult {
		switch name {
		case "systemctl":
			if len(args) > 1 && args[len(args)-2] == "is-active" {
				return commandResult{output: "active"}
			}
			return commandResult{output: "0"}
		case "journalctl":
			return commandResult{}
		case "coredumpctl":
			return commandResult{output: "No coredumps found."}
		default:
			return commandResult{err: fmt.Errorf("unexpected command %s", name)}
		}
	}}
}
