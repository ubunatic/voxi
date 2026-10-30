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
	"time"

	"ubunatic.com/voxi/internal/config"
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
	waits    []time.Duration
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

func (h *fixtureHost) Wait(_ context.Context, d time.Duration) error {
	h.waits = append(h.waits, d)
	h.calls = append(h.calls, "WAIT")
	return nil
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("testdata", name)
}

func dumpRun(t *testing.T, name string) func(string, ...string) commandResult {
	t.Helper()
	return func(command string, _ ...string) commandResult {
		if command != "pw-dump" {
			return commandResult{err: fmt.Errorf("unexpected command %s", command)}
		}
		data, err := os.ReadFile(fixture(t, name))
		return commandResult{output: string(data), err: err}
	}
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
				if !strings.Contains(out.String(), "  repair: "+command+"\n") || !strings.Contains(out.String(), "  applied\n") {
					t.Fatalf("repair output missing command/outcome:\n%s", out.String())
				}
				if strings.Index(out.String(), "Snapshot:") > strings.Index(out.String(), "  applied\n") {
					t.Fatalf("snapshot was not printed before apply:\n%s", out.String())
				}
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

func TestRoutingRejectsDummySinks(t *testing.T) {
	s := loadDoctorSpec(t)
	for _, tt := range []struct {
		name string
		link string
		dump string
	}{
		{"auto_null name", "pw-link-auto-null.txt", "pw-dump-healthy.json"},
		{"null sink factory", "pw-link-dummy-sink.txt", "pw-dump-dummy-sink.json"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := (HostDoctor{Host: &fixtureHost{linkFile: fixture(t, tt.link)}, Spec: s}).checkRouting(context.Background(), checkFor(t, s, "stream-routing"), false, fixture(t, tt.dump))
			if got.Status != FAULT || !strings.Contains(got.Detail, "dummy/null sink") {
				t.Fatalf("checkRouting() = %+v, want dummy sink fault", got)
			}
		})
	}
}

func TestRepairSettlesGraphUsingInjectedClock(t *testing.T) {
	s := loadDoctorSpec(t)
	good, err := os.ReadFile(fixture(t, "pw-dump-healthy.json"))
	if err != nil {
		t.Fatal(err)
	}
	dumpCalls := 0
	h := &fixtureHost{}
	h.run = func(name string, args ...string) commandResult {
		switch name {
		case "pw-dump":
			dumpCalls++
			return commandResult{output: string(good)}
		case "systemctl":
			if strings.Contains(strings.Join(args, " "), "MainPID") {
				if len(h.waits) > 0 {
					return commandResult{output: "202"}
				}
				return commandResult{output: "101"}
			}
			if len(args) > 1 && args[len(args)-2] == "is-active" {
				return commandResult{output: "active"}
			}
			return commandResult{output: "0"}
		case "journalctl":
			return commandResult{}
		case "coredumpctl":
			return commandResult{output: "No coredumps found."}
		case "curl":
			return commandResult{output: "ok"}
		default:
			return commandResult{err: fmt.Errorf("unexpected command %s", name)}
		}
	}
	report := Report{Checks: []Result{{ID: "services", Status: FAULT, Detail: "wireplumber is inactive", Repairs: []string{"systemctl --user restart wireplumber.service"}}}}
	got, err := (HostDoctor{Host: h, Spec: s}).Repair(context.Background(), report, Options{Fix: true, PWDump: fixture(t, "pw-dump-dangling-bt-sink.json"), NoProbe: true, Since: "2099-01-01", StateDir: t.TempDir()}, strings.NewReader(""), &bytes.Buffer{}, true, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"services", "default-devices", "bt-card-profile-off", "mic"} {
		idx := got.index(id)
		if idx < 0 || got.Checks[idx].Status != OK {
			t.Fatalf("post-repair %s result = %+v, want fresh OK", id, got.Checks)
		}
	}
	if len(h.waits) != 1 || h.waits[0] != 100*time.Millisecond || dumpCalls < 4 {
		t.Fatalf("waits=%v, want one 100ms interval before fresh diagnosis", h.waits)
	}
	applyAt, waitAt, freshDumpCalls := -1, -1, 0
	for i, call := range h.calls {
		if strings.HasPrefix(call, "APPLY ") {
			applyAt = i
		}
		if applyAt >= 0 && call == "WAIT" {
			waitAt = i
		}
		if waitAt >= 0 && i > waitAt && call == "pw-dump " {
			freshDumpCalls++
		}
	}
	if freshDumpCalls < 2 {
		t.Fatalf("expected a settle dump and fresh full recheck after settle, calls=%v", h.calls)
	}
}

func TestRepairSettleTimeoutIsCheckError(t *testing.T) {
	s := loadDoctorSpec(t)
	dump, err := os.ReadFile(fixture(t, "pw-dump-dangling-bt-sink.json"))
	if err != nil {
		t.Fatal(err)
	}
	h := &fixtureHost{run: func(name string, args ...string) commandResult {
		if name == "pw-dump" {
			return commandResult{output: string(dump)}
		}
		if name == "systemctl" {
			if len(args) > 1 && args[len(args)-2] == "is-active" {
				return commandResult{output: "active"}
			}
			return commandResult{output: "0"}
		}
		return commandResult{err: fmt.Errorf("unexpected command %s", name)}
	}}
	report := Report{Checks: []Result{{ID: "services", Status: FAULT, Detail: "inactive", Repairs: []string{"systemctl --user restart wireplumber.service"}}}}
	got, err := (HostDoctor{Host: h, Spec: s}).Repair(context.Background(), report, Options{Fix: true, StateDir: t.TempDir()}, strings.NewReader(""), &bytes.Buffer{}, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Checks[0].Status != ERROR || got.ExitCode() != 2 || len(h.waits) != 50 {
		t.Fatalf("report=%+v waits=%d, want ERROR, exit 2, 50 intervals", got.Checks[0], len(h.waits))
	}
}

func TestRepairDeduplicatesRestartEffects(t *testing.T) {
	s := loadDoctorSpec(t)
	h := healthyHost(t)
	h.routing = "alsa_output.pci-0000_07_00.6.HiFi__Speaker__sink"
	h.run = func(name string, args ...string) commandResult {
		if name == "pw-dump" {
			data, err := os.ReadFile(fixture(t, "pw-dump-healthy.json"))
			return commandResult{output: string(data), err: err}
		}
		if name == "systemctl" && strings.Contains(strings.Join(args, " "), "MainPID") {
			if len(h.applied) > 0 {
				return commandResult{output: "456"}
			}
			return commandResult{output: "123"}
		}
		return healthyHost(t).run(name, args...)
	}
	report := Report{Checks: []Result{
		{ID: "services", Status: FAULT, Detail: "wireplumber inactive", Repairs: []string{"systemctl --user restart wireplumber.service"}},
		{ID: "stream-routing", Status: FAULT, Detail: "unlinked", Repairs: []string{"systemctl --user restart wireplumber"}},
	}}
	var out bytes.Buffer
	got, err := (HostDoctor{Host: h, Spec: s}).Repair(context.Background(), report, Options{Fix: true, PWDump: fixture(t, "pw-dump-healthy.json"), StateDir: t.TempDir()}, strings.NewReader(""), &out, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.applied) != 1 || got.Checks[1].Status != OK || !strings.Contains(out.String(), "skipped: covered by the earlier restart") {
		t.Fatalf("applied=%v routing=%+v output:\n%s", h.applied, got.Checks[1], out.String())
	}
}

func TestCommandNonTTYDoesNotReadOrApply(t *testing.T) {
	h := healthyHost(t)
	cmd := NewCommand(&bytes.Buffer{}, h)
	var out bytes.Buffer
	cmd = NewCommand(&out, h)
	cmd.SetIn(strings.NewReader("y\n"))
	cmd.SetArgs([]string{"--fix", "--no-probe", "--pw-dump", fixture(t, "pw-dump-dangling-bt-sink.json"), "--since", "2099-01-01"})
	err := cmd.Execute()
	var exit ExitError
	if !errors.As(err, &exit) || exit.Code != 1 {
		t.Fatalf("Execute() error=%v, want fault exit code 1", err)
	}
	if len(h.applied) != 0 || strings.Contains(out.String(), "Apply repair") || !strings.Contains(out.String(), "stdin is not a terminal; rerun with --yes to apply") {
		t.Fatalf("applied=%v output:\n%s", h.applied, out.String())
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
			return doctor(&fixtureHost{routeErr: &exec.Error{Name: "pw-play", Err: exec.ErrNotFound}}).checkRouting(ctx, checkFor(t, s, "stream-routing"), false, "")
		}},
		{"06 no probe skips", resultFor(t, "stream-routing", SKIP, "--no-probe given, the stream probe was not run"), func() Result {
			return doctor(&fixtureHost{}).checkRouting(ctx, checkFor(t, s, "stream-routing"), true, "")
		}},
		{"06 linked probe is OK", resultFor(t, "stream-routing", OK, "probe stream linked to sink 'alsa_output.pci-0000_07_00.6.HiFi__Speaker__sink'"), func() Result {
			return doctor(&fixtureHost{run: dumpRun(t, "pw-dump-healthy.json"), linkFile: fixture(t, "pw-link-linked.txt")}).checkRouting(ctx, checkFor(t, s, "stream-routing"), false, fixture(t, "pw-dump-healthy.json"))
		}},
		{"06 unlinked probe faults", resultFor(t, "stream-routing", FAULT, "a new playback stream was not linked to any sink (WirePlumber is not routing streams)", "systemctl --user restart wireplumber"), func() Result {
			return doctor(&fixtureHost{linkFile: fixture(t, "pw-link-unlinked.txt")}).checkRouting(ctx, checkFor(t, s, "stream-routing"), false, "")
		}},
		{"06 repair restarts WirePlumber", resultFor(t, "stream-routing", FAULT, "a new playback stream was not linked to any sink (WirePlumber is not routing streams)", "systemctl --user restart wireplumber"), func() Result {
			return doctor(&fixtureHost{}).checkRouting(ctx, checkFor(t, s, "stream-routing"), false, "")
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
		case "curl":
			return commandResult{output: "ok"}
		case "ps":
			return commandResult{}
		case "which":
			// The isolated HOME selects the default model, a local engine.
			return commandResult{output: "/usr/bin/" + args[0]}
		default:
			return commandResult{err: fmt.Errorf("unexpected command %s", name)}
		}
	}}
}

func TestPruneSnapshots(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

	// Create 6 snapshots with timestamps from 1 to 6 days ago.
	dates := []time.Time{
		now.Add(-1 * 24 * time.Hour),
		now.Add(-2 * 24 * time.Hour),
		now.Add(-3 * 24 * time.Hour),
		now.Add(-4 * 24 * time.Hour),
		now.Add(-5 * 24 * time.Hour),
		now.Add(-6 * 24 * time.Hour),
	}

	for _, d := range dates {
		filename := fmt.Sprintf("%s%s%s", snapshotPrefix, d.Format(snapshotTimeFormat), snapshotSuffix)
		if err := os.WriteFile(filepath.Join(dir, filename), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// Also create an unrelated file that should not be touched
	unrelated := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(unrelated, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	// 1. Prune with keepCount=3 and maxAge=10 days:
	// Only the newest 3 snapshots should remain.
	if err := pruneSnapshots(dir, 3, 10*24*time.Hour, now); err != nil {
		t.Fatalf("pruneSnapshots error = %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	var remainingSnapshots []string
	foundUnrelated := false
	for _, e := range entries {
		if e.Name() == "notes.txt" {
			foundUnrelated = true
		} else if strings.HasPrefix(e.Name(), snapshotPrefix) {
			remainingSnapshots = append(remainingSnapshots, e.Name())
		}
	}

	if !foundUnrelated {
		t.Error("expected notes.txt to remain untouched")
	}
	if len(remainingSnapshots) != 3 {
		t.Fatalf("remaining snapshots = %d, want 3 (%v)", len(remainingSnapshots), remainingSnapshots)
	}

	// Ensure the remaining ones are the newest (1, 2, 3 days ago)
	for _, expectedDate := range dates[:3] {
		expectedFile := fmt.Sprintf("%s%s%s", snapshotPrefix, expectedDate.Format(snapshotTimeFormat), snapshotSuffix)
		if _, err := os.Stat(filepath.Join(dir, expectedFile)); err != nil {
			t.Errorf("expected snapshot %s to exist: %v", expectedFile, err)
		}
	}

	// 2. Prune with maxAge=1.5 days:
	// Snapshots older than 1.5 days should be deleted, leaving only the 1-day-old one.
	if err := pruneSnapshots(dir, 5, 36*time.Hour, now); err != nil {
		t.Fatalf("pruneSnapshots error = %v", err)
	}

	entriesAfterAge, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var snapsAfterAge []string
	for _, e := range entriesAfterAge {
		if strings.HasPrefix(e.Name(), snapshotPrefix) {
			snapsAfterAge = append(snapsAfterAge, e.Name())
		}
	}
	if len(snapsAfterAge) != 1 {
		t.Fatalf("remaining snapshots after age prune = %d, want 1 (%v)", len(snapsAfterAge), snapsAfterAge)
	}
}

func TestSnapshotPrunesOldFiles(t *testing.T) {
	s := loadDoctorSpec(t)
	keepCount := s.SnapshotKeepCount()
	if keepCount < 1 {
		t.Fatalf("keepCount = %d, want >= 1", keepCount)
	}

	stateDir := t.TempDir()
	now := time.Now()

	// Pre-create keepCount + 2 older snapshots
	for i := 1; i <= keepCount+2; i++ {
		ts := now.Add(-time.Duration(i) * time.Hour)
		name := fmt.Sprintf("%s%s%s", snapshotPrefix, ts.Format(snapshotTimeFormat), snapshotSuffix)
		if err := os.WriteFile(filepath.Join(stateDir, name), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	h := &fixtureHost{run: func(name string, _ ...string) commandResult {
		if name == "pw-dump" {
			return commandResult{output: `[{"id": 0}]`}
		}
		return commandResult{err: errors.New("unexpected command")}
	}}

	doctor := HostDoctor{Host: h, Spec: s}
	path, dump, err := doctor.snapshot(context.Background(), stateDir)
	if err != nil {
		t.Fatalf("doctor.snapshot error = %v", err)
	}
	if len(dump) == 0 {
		t.Fatal("empty dump returned")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("new snapshot file not found: %v", err)
	}

	entries, err := os.ReadDir(stateDir)
	if err != nil {
		t.Fatal(err)
	}

	var snapshots []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), snapshotPrefix) {
			snapshots = append(snapshots, e.Name())
		}
	}

	if len(snapshots) != keepCount {
		t.Fatalf("snapshot count after prune = %d, want %d (%v)", len(snapshots), keepCount, snapshots)
	}
}

func TestCheckAsrEngine(t *testing.T) {
	s, err := spec.LoadDoctor()
	if err != nil {
		t.Fatal(err)
	}
	check := checkFor(t, s, "asr-engine")

	t.Run("healthy online openai engine", func(t *testing.T) {
		isolateASRHome(t)
		probe := &fakeASRProbe{answers: map[string]string{pinnedR2T2URL: "One, two."}}
		h := psHost(eagerDaemonPS)
		res := HostDoctor{Host: h, Spec: s, ASRProbe: probe.probe}.checkAsrEngine(context.Background(), check)
		if res.Status != OK {
			t.Fatalf("expected status OK, got %+v", res)
		}
		if !strings.Contains(res.Detail, "online") {
			t.Fatalf("expected detail to mention online, got: %s", res.Detail)
		}
	})

	t.Run("offline proxy endpoint suggests repairs", func(t *testing.T) {
		isolateASRHome(t)
		probe := &fakeASRProbe{errs: map[string]error{pinnedR2T2URL: failoverErr("connection refused")}}
		h := &fixtureHost{run: func(name string, args ...string) commandResult {
			return commandResult{err: fmt.Errorf("unexpected: %s", name)}
		}}
		res := HostDoctor{Host: h, Spec: s, ASRProbe: probe.probe}.checkAsrEngine(context.Background(), check)
		if res.Status != FAULT {
			t.Fatalf("expected status FAULT, got %+v", res)
		}
		joined := strings.Join(res.Repairs, "\n")
		if !strings.Contains(joined, "restart lmcoder-proxy.service") {
			t.Fatalf("expected proxy restart in repairs, got: %v", res.Repairs)
		}
		// Obsolete expectation replaced (issue 206): the fallback was offered with
		// no check that its model files exist. Offering it is now unsafe.
		if strings.Contains(joined, "r2t2-confucius4-voxi") || strings.Contains(joined, "voxi-r2t2.service") {
			t.Fatalf("repairs must not select or start the unverified fallback, got: %v", res.Repairs)
		}
	})

	t.Run("fallback active warns when primary is back online", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		if err := config.SaveUserSettings(home, &config.UserSettings{ASRModel: "r2t2-confucius4-voxi"}); err != nil {
			t.Fatal(err)
		}
		probe := &fakeASRProbe{answers: map[string]string{
			fallbackR2T2URL: "One, two.",
			pinnedR2T2URL:   "One, two.",
		}}
		h := &fixtureHost{run: func(name string, args ...string) commandResult {
			return commandResult{err: fmt.Errorf("unexpected: %s", name)}
		}}
		res := HostDoctor{Host: h, Spec: s, ASRProbe: probe.probe}.checkAsrEngine(context.Background(), check)
		if res.Status != WARN {
			t.Fatalf("expected status WARN, got %+v", res)
		}
		if len(res.See) == 0 || !strings.Contains(strings.Join(res.See, " "), "voxi-r2t2.service") {
			t.Fatalf("expected see hints to revert and stop fallback service, got: %v", res.See)
		}
		// Guidance only: switching back or stopping the fallback is the user's call,
		// and the daemon keeps the fallback until it restarts.
		if len(res.Repairs) != 0 {
			t.Fatalf("primary-online guidance must not carry repairs, got: %v", res.Repairs)
		}
		modelSpec, err := spec.LoadModels()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.Join(res.See, " "), modelSpec.DefaultModel) {
			t.Fatalf("guidance must not switch to the default model %q, got: %v", modelSpec.DefaultModel, res.See)
		}
		if !strings.Contains(strings.Join(res.See, " "), "voxi settings set model r2t2-confucius4") {
			t.Fatalf("expected guidance to return to the primary, got: %v", res.See)
		}
	})
}
