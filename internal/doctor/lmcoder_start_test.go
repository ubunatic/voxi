package doctor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"ubunatic.com/voxi/spec"
)

// lmcoderService is the unit that serves r2t2-confucius4 and its start costs,
// read from the embedded spec so these tests follow spec/models.yaml.
func lmcoderService(t *testing.T) *spec.LocalService {
	t.Helper()
	models, err := spec.LoadModels()
	if err != nil {
		t.Fatal(err)
	}
	m, ok := models.Models["r2t2-confucius4"]
	if !ok || m.LocalService == nil {
		t.Fatal("r2t2-confucius4 has no local_service in spec/models.yaml")
	}
	return m.LocalService
}

// lmcoderStartCommand is the asr-engine start repair from spec/doctor.yaml.
func lmcoderStartCommand(t *testing.T) string {
	t.Helper()
	cmd := checkFor(t, loadDoctorSpec(t), "asr-engine").Repair.Templates["start_asr"]
	if cmd == "" {
		t.Fatal("spec/doctor.yaml has no start_asr template")
	}
	return cmd
}

// lmcoderUnitHost reports the lmcoder unit as enabled at boot.
func lmcoderUnitHost(t *testing.T, load, active string, cli bool) *fixtureHost {
	t.Helper()
	return lmcoderUnitHostFile(t, load, active, "enabled", cli)
}

// lmcoderUnitHostFile reports the lmcoder unit with the given load, active and
// unit file states, and whether the lmcoder command is on PATH. The eager
// daemon runs on the saved model, so a probe that transcribes is accepted as
// serving.
func lmcoderUnitHostFile(t *testing.T, load, active, file string, cli bool) *fixtureHost {
	t.Helper()
	unit := lmcoderService(t).Unit
	base := unitsHost(eagerDaemonPS, testAgentPID, "0")
	h := &fixtureHost{}
	h.run = func(name string, args ...string) commandResult {
		switch {
		case name == "systemctl" && slices.Contains(args, "show") && slices.Contains(args, unit):
			return commandResult{output: fmt.Sprintf("LoadState=%s\nActiveState=%s\nUnitFileState=%s\n", load, active, file)}
		case name == "which" && slices.Equal(args, []string{"lmcoder"}):
			if cli {
				return commandResult{output: "/usr/bin/lmcoder\n"}
			}
			return commandResult{err: errors.New("lmcoder: not found")}
		case name == "pw-dump":
			return commandResult{output: "[]"}
		}
		return base.run(name, args...)
	}
	return h
}

// lmcoderStateHost reports the unit inactive until the start is applied, then
// reports states in order, one per query, repeating the last one.
func lmcoderStateHost(t *testing.T, cli bool, states ...string) *fixtureHost {
	t.Helper()
	unit := lmcoderService(t).Unit
	h := lmcoderUnitHost(t, "loaded", "inactive", cli)
	inner := h.run
	queries := 0
	h.run = func(name string, args ...string) commandResult {
		if name == "systemctl" && slices.Contains(args, "show") && slices.Contains(args, unit) && len(h.applied) > 0 {
			st := states[min(queries, len(states)-1)]
			queries++
			return commandResult{output: "LoadState=loaded\nActiveState=" + st + "\nUnitFileState=enabled\n"}
		}
		return inner(name, args...)
	}
	return h
}

// checkResult returns the check with id from a report, failing the test if absent.
func checkResult(t *testing.T, r Report, id string) Result {
	t.Helper()
	for _, c := range r.Checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("report has no %s check: %+v", id, r.Checks)
	return Result{}
}

func TestLmcoderStoppedUnitOffersStart(t *testing.T) {
	s := loadDoctorSpec(t)
	svc := lmcoderService(t)
	isolateASRHome(t)
	h := lmcoderUnitHostFile(t, "loaded", "inactive", "disabled", true)
	probe := &fakeASRProbe{}
	res := HostDoctor{Host: h, Spec: s, ASRProbe: probe.probe}.checkAsrEngine(context.Background(), checkFor(t, s, "asr-engine"))
	if res.Status != FAULT {
		t.Fatalf("status = %s, want FAULT: %+v", res.Status, res)
	}
	if !slices.Equal(res.Repairs, []string{lmcoderStartCommand(t)}) {
		t.Fatalf("repairs = %v, want only %q (the proxy restart cannot start a stopped service)", res.Repairs, lmcoderStartCommand(t))
	}
	notice := strings.Join(res.Notice, "\n")
	for _, want := range []string{
		"starting " + svc.Unit + " costs:",
		fmt.Sprintf("GPU: about %.2f GiB of VRAM", svc.VRAMGiB),
		fmt.Sprintf("RAM: about %.1f GiB", svc.RAMGiB),
		fmt.Sprintf("first start, if the model files are not cached, downloads about %.1f GB", svc.DownloadGB),
		fmt.Sprintf("first start can take up to %d min", svc.FirstStartSeconds/60),
		"systemctl --user enable " + svc.Unit + " (--fix does not)",
	} {
		if !strings.Contains(notice, want) {
			t.Errorf("notice missing %q:\n%s", want, notice)
		}
	}
	if want := time.Duration(svc.FirstStartSeconds) * time.Second; res.StartBudget != want {
		t.Errorf("start budget = %v, want %v from spec", res.StartBudget, want)
	}
	if res.StartUnit != svc.Unit {
		t.Errorf("start unit = %q, want %q", res.StartUnit, svc.Unit)
	}
	if !strings.Contains(res.Detail, svc.Unit+" is inactive") {
		t.Errorf("detail does not name the stopped unit: %q", res.Detail)
	}
}

// TestLmcoderEnabledUnitOmitsBootLine checks the printed cost warning: a unit
// that already starts at boot gets no enable hint.
func TestLmcoderEnabledUnitOmitsBootLine(t *testing.T) {
	isolateASRHome(t)
	h := lmcoderUnitHostFile(t, "loaded", "inactive", "enabled", true)
	probe, _ := startingProbe(1)
	out, applied, _ := fixAfterStart(t, h, probe, "n\n", false)

	if len(applied) != 0 {
		t.Fatalf("applied = %v, want nothing after a declined prompt", applied)
	}
	if !strings.Contains(out, "starting "+lmcoderService(t).Unit+" costs:") {
		t.Fatalf("cost warning missing:\n%s", out)
	}
	if strings.Contains(out, "boot:") || strings.Contains(out, "systemctl --user enable") {
		t.Fatalf("enabled unit must not print an enable hint:\n%s", out)
	}
}

func TestLmcoderUnitStatesThatDoNotStart(t *testing.T) {
	cases := []struct {
		name         string
		load, active string
		cli          bool
		wantRepairs  []string
		wantSee      string
	}{
		{"service already running keeps the proxy restart", "loaded", "active", true, []string{"systemctl --user restart lmcoder-proxy.service"}, ""},
		{"unit missing gives guidance only", "not-found", "inactive", true, nil, "not loaded (LoadState=not-found)"},
		{"lmcoder missing gives guidance only", "loaded", "inactive", false, nil, "not on PATH"},
		{"activating start is waited for, never restarted", "loaded", "activating", true, nil, "start is in progress"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := loadDoctorSpec(t)
			isolateASRHome(t)
			h := lmcoderUnitHost(t, tc.load, tc.active, tc.cli)
			probe := &fakeASRProbe{}
			res := HostDoctor{Host: h, Spec: s, ASRProbe: probe.probe}.checkAsrEngine(context.Background(), checkFor(t, s, "asr-engine"))
			if res.Status != FAULT {
				t.Fatalf("status = %s, want FAULT: %+v", res.Status, res)
			}
			if !slices.Equal(res.Repairs, tc.wantRepairs) {
				t.Fatalf("repairs = %v, want %v", res.Repairs, tc.wantRepairs)
			}
			if res.Notice != nil || res.StartBudget != 0 {
				t.Fatalf("no start means no cost notice or wait, got notice=%v budget=%v", res.Notice, res.StartBudget)
			}
			if tc.wantSee != "" && !strings.Contains(strings.Join(res.See, "\n"), tc.wantSee) {
				t.Fatalf("see = %v, want a line with %q", res.See, tc.wantSee)
			}
		})
	}
}

// TestFaultSeeHintIsPrinted checks the printed report: guidance on a FAULT
// (unit not loaded, unit activating, lmcoder missing) must appear as a see line.
func TestFaultSeeHintIsPrinted(t *testing.T) {
	svc := lmcoderService(t)
	cases := []struct {
		name, load, active string
		cli                bool
		want               string
	}{
		{"unit not loaded", "not-found", "inactive", true, "  see: " + svc.Unit + " is not loaded (LoadState=not-found)"},
		{"unit activating", "loaded", "activating", true, "  see: " + svc.Unit + " is activating"},
		{"lmcoder missing", "loaded", "inactive", false, "  see: " + svc.Unit + " is inactive, but lmcoder is not on PATH"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := loadDoctorSpec(t)
			isolateASRHome(t)
			h := lmcoderUnitHost(t, tc.load, tc.active, tc.cli)
			probe := &fakeASRProbe{}
			res := HostDoctor{Host: h, Spec: s, ASRProbe: probe.probe}.checkAsrEngine(context.Background(), checkFor(t, s, "asr-engine"))
			var out bytes.Buffer
			Report{Checks: []Result{res}}.Print(&out)
			if res.Status != FAULT || !strings.Contains(out.String(), tc.want) {
				t.Fatalf("printed report lacks %q:\n%s", tc.want, out.String())
			}
		})
	}
}

// TestStartNoticeRoundsMinutes checks the printed cost lines for first-start
// durations that are not whole minutes, and that the download line is
// conditional on the model files not being cached.
func TestStartNoticeRoundsMinutes(t *testing.T) {
	cases := []struct {
		seconds int
		want    string
	}{
		{300, "can take up to 5 min"},
		{330, "can take up to 5.5 min"},
		{90, "can take up to 1.5 min"},
	}
	for _, tc := range cases {
		svc := *lmcoderService(t)
		svc.FirstStartSeconds = tc.seconds
		out := strings.Join(startNotice(&svc, ""), "\n")
		if !strings.Contains(out, tc.want) {
			t.Errorf("%ds: notice lacks %q:\n%s", tc.seconds, tc.want, out)
		}
		if !strings.Contains(out, "first start, if the model files are not cached, downloads") {
			t.Errorf("%ds: download line is not conditional on the cache:\n%s", tc.seconds, out)
		}
		if strings.Contains(out, "boot:") {
			t.Errorf("%ds: empty enable command must omit the boot line:\n%s", tc.seconds, out)
		}
	}
}

// fixAfterStart runs Repair on the asr-engine fault and returns the output,
// the applied commands and the report after repair.
func fixAfterStart(t *testing.T, h *fixtureHost, probe ASRProbeFunc, answer string, yes bool) (string, []string, Report) {
	t.Helper()
	s := loadDoctorSpec(t)
	isolateASRHome(t)
	d := HostDoctor{Host: h, Spec: s, ASRProbe: probe}
	first := d.checkAsrEngine(context.Background(), checkFor(t, s, "asr-engine"))
	report := Report{Checks: []Result{first}}
	var out bytes.Buffer
	got, err := d.Repair(context.Background(), report, Options{Fix: true, StateDir: t.TempDir()}, strings.NewReader(answer), &out, yes, !yes)
	if err != nil {
		t.Fatal(err)
	}
	return out.String(), h.applied, got
}

// startingProbe refuses the pinned proxy for the first failures probes, then
// transcribes, as a first start does while its model downloads.
func startingProbe(failures int) (ASRProbeFunc, *int) {
	calls := 0
	return func(_ context.Context, baseURL string, _ spec.Model) (string, error) {
		if baseURL != pinnedR2T2URL {
			return "", failoverErr("connection refused")
		}
		calls++
		if calls <= failures {
			return "", failoverErr("returned 503 Service Unavailable")
		}
		return "One, two.", nil
	}, &calls
}

func TestStartRepairShowsCostBeforePromptAndWaits(t *testing.T) {
	svc := lmcoderService(t)
	budget := time.Duration(svc.FirstStartSeconds) * time.Second
	h := lmcoderUnitHost(t, "loaded", "inactive", true)
	probe, _ := startingProbe(2)
	out, applied, got := fixAfterStart(t, h, probe, "y\n", false)

	if !slices.Equal(applied, []string{lmcoderStartCommand(t)}) {
		t.Fatalf("applied = %v, want only the start command", applied)
	}
	notice := strings.Index(out, "starting "+svc.Unit+" costs:")
	prompt := strings.Index(out, "Apply repair for asr-engine? [y/N]")
	starting := strings.Index(out, "  starting "+svc.Unit+" (waiting up to "+budget.String()+" for it to load its models)")
	applied2 := strings.Index(out, "  applied")
	poll := strings.Index(out, "  waiting for "+svc.Unit+": "+startPollInterval.String()+" of "+budget.String())
	if notice < 0 || prompt < 0 || notice > prompt {
		t.Fatalf("cost notice must precede the prompt:\n%s", out)
	}
	if starting < prompt || applied2 < starting || poll < applied2 {
		t.Fatalf("want prompt, then the starting line, then applied, then the poll line; got %d %d %d %d:\n%s", prompt, starting, applied2, poll, out)
	}
	if len(h.waits) != 1 || h.waits[0] != startPollInterval {
		t.Fatalf("waits = %v, want one poll of %v while the first start loads", h.waits, startPollInterval)
	}
	if status := checkResult(t, got, "asr-engine").Status; status != OK {
		t.Fatalf("asr-engine after start = %s, want OK:\n%s", status, out)
	}
}

func TestStartRepairGivesUpAfterBudget(t *testing.T) {
	svc := lmcoderService(t)
	budget := time.Duration(svc.FirstStartSeconds) * time.Second
	h := lmcoderUnitHost(t, "loaded", "inactive", true)
	probe, _ := startingProbe(1 << 20)
	out, applied, got := fixAfterStart(t, h, probe, "y\n", false)

	if len(applied) != 1 {
		t.Fatalf("applied = %v, want the start once", applied)
	}
	if want := int(budget / startPollInterval); len(h.waits) != want {
		t.Fatalf("waits = %d, want %d polls across the %v budget", len(h.waits), want, budget)
	}
	if n := strings.Count(out, "  waiting for "+svc.Unit+":"); n != len(h.waits) {
		t.Fatalf("printed %d progress lines for %d polls:\n%s", n, len(h.waits), out)
	}
	if !strings.Contains(out, "started service did not transcribe within "+budget.String()) {
		t.Fatalf("missing budget message:\n%s", out)
	}
	if status := checkResult(t, got, "asr-engine").Status; status != FAULT {
		t.Fatalf("asr-engine after failed start = %s, want FAULT", status)
	}
}

// TestStartRepairWaitIsWallClock: each probe takes 30s of fake time. The budget
// is a deadline, so probe time counts against it and fewer polls fit than
// budget/interval. A budget made of summed sleeps would allow all of them.
func TestStartRepairWaitIsWallClock(t *testing.T) {
	svc := lmcoderService(t)
	budget := time.Duration(svc.FirstStartSeconds) * time.Second
	h := lmcoderUnitHost(t, "loaded", "inactive", true)
	probe := func(context.Context, string, spec.Model) (string, error) {
		h.clock = h.clock.Add(30 * time.Second)
		return "", failoverErr("returned 503 Service Unavailable")
	}
	out, _, _ := fixAfterStart(t, h, probe, "y\n", false)

	var waited time.Duration
	for _, w := range h.waits {
		waited += w
	}
	if len(h.waits) >= int(budget/startPollInterval) || waited >= budget {
		t.Fatalf("%d polls waited %v: probe time must count against the %v budget:\n%s", len(h.waits), waited, budget, out)
	}
	if !strings.Contains(out, "did not transcribe within "+budget.String()) {
		t.Fatalf("missing budget message:\n%s", out)
	}
}

// TestStartRepairStopsWhenUnitFails: a unit that reaches failed during the
// wait ends the wait at once, with a message naming the unit.
func TestStartRepairStopsWhenUnitFails(t *testing.T) {
	svc := lmcoderService(t)
	h := lmcoderStateHost(t, true, "failed")
	probe, _ := startingProbe(1 << 20)
	out, applied, got := fixAfterStart(t, h, probe, "y\n", false)

	if !slices.Equal(applied, []string{lmcoderStartCommand(t)}) {
		t.Fatalf("applied = %v, want only the start", applied)
	}
	if len(h.waits) != 1 {
		t.Fatalf("waits = %d, want 1: the wait stops at the first poll that sees failed", len(h.waits))
	}
	if !strings.Contains(out, svc.Unit+" failed to start") {
		t.Fatalf("missing failure message:\n%s", out)
	}
	if strings.Contains(out, "did not transcribe") {
		t.Fatalf("a failed unit must not print the budget message:\n%s", out)
	}
	if status := checkResult(t, got, "asr-engine").Status; status != FAULT {
		t.Fatalf("asr-engine = %s, want FAULT", status)
	}
}

// TestStartRepairFollowsStateChanges: the unit goes activating, then active,
// and the wait ends when the probe transcribes.
func TestStartRepairFollowsStateChanges(t *testing.T) {
	h := lmcoderStateHost(t, true, "activating", "active")
	probe, _ := startingProbe(3)
	out, applied, got := fixAfterStart(t, h, probe, "y\n", false)

	if len(applied) != 1 {
		t.Fatalf("applied = %v, want the start once", applied)
	}
	if len(h.waits) != 2 {
		t.Fatalf("waits = %d, want 2: activating, then active", len(h.waits))
	}
	if strings.Contains(out, "failed to start") || strings.Contains(out, "did not transcribe") {
		t.Fatalf("a start that becomes active must not report failure:\n%s", out)
	}
	if !strings.Contains(out, "unit active") {
		t.Fatalf("progress line should name the unit state it saw:\n%s", out)
	}
	if status := checkResult(t, got, "asr-engine").Status; status != OK {
		t.Fatalf("asr-engine = %s, want OK:\n%s", status, out)
	}
}

// blockingApplyHost runs Apply until its context ends, as a hung start would.
type blockingApplyHost struct{ *fixtureHost }

func (h blockingApplyHost) Apply(ctx context.Context, _ string) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("start command ran without a deadline")
	}
	<-ctx.Done()
	return ctx.Err()
}

// TestStartCommandIsBoundedByBudget: a start that never returns is stopped at
// the budget, so --fix cannot hang on it.
func TestStartCommandIsBoundedByBudget(t *testing.T) {
	d := HostDoctor{Host: blockingApplyHost{&fixtureHost{}}}
	err := d.applyRepair(context.Background(), Result{StartBudget: time.Nanosecond, StartUnit: "lmcoder-asr.service"}, lmcoderStartCommand(t))
	if err == nil || !strings.Contains(err.Error(), "did not return within 1ns") {
		t.Fatalf("applyRepair error = %v, want the start to be stopped at its budget", err)
	}
}

func TestStartRepairDeclinedAppliesNothing(t *testing.T) {
	h := lmcoderUnitHost(t, "loaded", "inactive", true)
	probe, _ := startingProbe(1)
	out, applied, _ := fixAfterStart(t, h, probe, "n\n", false)

	if len(applied) != 0 {
		t.Fatalf("applied = %v, want nothing after a declined prompt", applied)
	}
	if !strings.Contains(out, "declined") {
		t.Fatalf("missing declined outcome:\n%s", out)
	}
	if n := strings.Count(out, "starting "+lmcoderService(t).Unit+" costs:"); n != 1 {
		t.Fatalf("cost notice printed %d times, want once:\n%s", n, out)
	}
}

func TestStartRepairYesPrintsCostAndApplies(t *testing.T) {
	h := lmcoderUnitHost(t, "loaded", "inactive", true)
	probe, _ := startingProbe(1)
	out, applied, _ := fixAfterStart(t, h, probe, "", true)

	if !slices.Equal(applied, []string{lmcoderStartCommand(t)}) {
		t.Fatalf("applied = %v, want the start under --yes", applied)
	}
	if !strings.Contains(out, "starting "+lmcoderService(t).Unit+" costs:") {
		t.Fatalf("--yes must still print the cost:\n%s", out)
	}
}
