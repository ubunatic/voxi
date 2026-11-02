package doctor

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"ubunatic.com/voxi/spec"
)

// lmcoderASRUnit is the unit the lmcoder endpoint is served by (spec/models.yaml).
const lmcoderASRUnit = "lmcoder-asr.service"

const lmcoderStartCommand = "lmcoder service start --asr"

// lmcoderUnitHost reports lmcoder-asr.service with the given load and active
// state, and whether the lmcoder command is on PATH. The eager daemon runs on
// the saved model, so a probe that transcribes is accepted as serving.
func lmcoderUnitHost(load, active string, cli bool) *fixtureHost {
	base := unitsHost(eagerDaemonPS, testAgentPID, "0")
	h := &fixtureHost{}
	h.run = func(name string, args ...string) commandResult {
		switch {
		case name == "systemctl" && slices.Contains(args, "show") && slices.Contains(args, lmcoderASRUnit):
			return commandResult{output: "LoadState=" + load + "\nActiveState=" + active + "\n"}
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
	isolateASRHome(t)
	h := lmcoderUnitHost("loaded", "inactive", true)
	probe := &fakeASRProbe{}
	res := HostDoctor{Host: h, Spec: s, ASRProbe: probe.probe}.checkAsrEngine(context.Background(), checkFor(t, s, "asr-engine"))
	if res.Status != FAULT {
		t.Fatalf("status = %s, want FAULT: %+v", res.Status, res)
	}
	if !slices.Equal(res.Repairs, []string{lmcoderStartCommand}) {
		t.Fatalf("repairs = %v, want only %q (the proxy restart cannot start a stopped service)", res.Repairs, lmcoderStartCommand)
	}
	notice := strings.Join(res.Notice, "\n")
	for _, want := range []string{
		"starting lmcoder-asr.service costs:",
		"GPU: about 2.78 GiB of VRAM",
		"RAM: about 2.0 GiB",
		"first start downloads about 1.4 GB",
		"can take up to 5 min",
		"systemctl --user enable lmcoder-asr.service (--fix does not)",
	} {
		if !strings.Contains(notice, want) {
			t.Errorf("notice missing %q:\n%s", want, notice)
		}
	}
	if res.StartBudget != 5*time.Minute {
		t.Errorf("start budget = %v, want 5m", res.StartBudget)
	}
	if !strings.Contains(res.Detail, "lmcoder-asr.service is inactive") {
		t.Errorf("detail does not name the stopped unit: %q", res.Detail)
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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := loadDoctorSpec(t)
			isolateASRHome(t)
			h := lmcoderUnitHost(tc.load, tc.active, tc.cli)
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
	h := lmcoderUnitHost("loaded", "inactive", true)
	probe, _ := startingProbe(2)
	out, applied, got := fixAfterStart(t, h, probe, "y\n", false)

	if !slices.Equal(applied, []string{lmcoderStartCommand}) {
		t.Fatalf("applied = %v, want only %q", applied, lmcoderStartCommand)
	}
	notice := strings.Index(out, "starting lmcoder-asr.service costs:")
	prompt := strings.Index(out, "Apply repair for asr-engine? [y/N]")
	if notice < 0 || prompt < 0 || notice > prompt {
		t.Fatalf("cost notice must precede the prompt:\n%s", out)
	}
	if len(h.waits) != 1 || h.waits[0] != startPollInterval {
		t.Fatalf("waits = %v, want one poll of %v while the first start loads", h.waits, startPollInterval)
	}
	if status := checkResult(t, got, "asr-engine").Status; status != OK {
		t.Fatalf("asr-engine after start = %s, want OK:\n%s", status, out)
	}
}

func TestStartRepairGivesUpAfterBudget(t *testing.T) {
	h := lmcoderUnitHost("loaded", "inactive", true)
	probe, _ := startingProbe(1 << 20)
	out, applied, got := fixAfterStart(t, h, probe, "y\n", false)

	if len(applied) != 1 {
		t.Fatalf("applied = %v, want the start once", applied)
	}
	if want := int((5 * time.Minute) / startPollInterval); len(h.waits) != want {
		t.Fatalf("waits = %d, want %d polls across the 5m budget", len(h.waits), want)
	}
	if !strings.Contains(out, "started service did not transcribe within 5m0s") {
		t.Fatalf("missing budget message:\n%s", out)
	}
	if status := checkResult(t, got, "asr-engine").Status; status != FAULT {
		t.Fatalf("asr-engine after failed start = %s, want FAULT", status)
	}
}

func TestStartRepairDeclinedAppliesNothing(t *testing.T) {
	h := lmcoderUnitHost("loaded", "inactive", true)
	probe, _ := startingProbe(1)
	out, applied, _ := fixAfterStart(t, h, probe, "n\n", false)

	if len(applied) != 0 {
		t.Fatalf("applied = %v, want nothing after a declined prompt", applied)
	}
	if !strings.Contains(out, "declined") {
		t.Fatalf("missing declined outcome:\n%s", out)
	}
	if n := strings.Count(out, "starting lmcoder-asr.service costs:"); n != 1 {
		t.Fatalf("cost notice printed %d times, want once:\n%s", n, out)
	}
}

func TestStartRepairYesPrintsCostAndApplies(t *testing.T) {
	h := lmcoderUnitHost("loaded", "inactive", true)
	probe, _ := startingProbe(1)
	out, applied, _ := fixAfterStart(t, h, probe, "", true)

	if !slices.Equal(applied, []string{lmcoderStartCommand}) {
		t.Fatalf("applied = %v, want the start under --yes", applied)
	}
	if !strings.Contains(out, "starting lmcoder-asr.service costs:") {
		t.Fatalf("--yes must still print the cost:\n%s", out)
	}
}
