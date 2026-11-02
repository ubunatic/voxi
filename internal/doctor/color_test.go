package doctor

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// runFaultCommand runs `voxi doctor --fix --no-probe` on the dangling-sink
// fixture, which reports FAULT lines without applying anything.
func runFaultCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	h := healthyHost(t)
	var out bytes.Buffer
	cmd := NewCommand(&out, h)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs(append([]string{"--fix", "--no-probe", "--pw-dump", fixture(t, "pw-dump-dangling-bt-sink.json"), "--since", "2099-01-01"}, args...))
	err := cmd.Execute()
	return out.String(), err
}

func TestDoctorColorAlwaysPaintsStatus(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	out, err := runFaultCommand(t, "--color=always")
	var exit ExitError
	if !errors.As(err, &exit) || exit.Code != 1 {
		t.Fatalf("Execute() error=%v, want fault exit code 1", err)
	}
	if !strings.Contains(out, "\x1b[31mFAULT\x1b[0m") {
		t.Fatalf("--color=always should paint FAULT red:\n%q", out)
	}
}

func TestDoctorNoColorEnvWinsOverAlways(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	out, _ := runFaultCommand(t, "--color=always")
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("NO_COLOR must disable color even with --color=always:\n%q", out)
	}
	if !strings.Contains(out, "FAULT ") {
		t.Fatalf("plain output must still name the verdict:\n%s", out)
	}
}

func TestDoctorAutoIsPlainWhenStdoutIsNotTerminal(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	out, _ := runFaultCommand(t)
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("auto color on a buffer (not a terminal) must be plain:\n%q", out)
	}
}

func TestDoctorRejectsUnknownColorMode(t *testing.T) {
	out, err := runFaultCommand(t, "--color=sometimes")
	var exit ExitError
	if !errors.As(err, &exit) || exit.Code != 2 {
		t.Fatalf("Execute() error=%v, want exit code 2", err)
	}
	if !strings.Contains(out, `invalid --color value "sometimes"`) {
		t.Fatalf("missing validation message:\n%s", out)
	}
}

func TestStartNoticeIsYellowWhenColorOn(t *testing.T) {
	s := loadDoctorSpec(t)
	isolateASRHome(t)
	h := lmcoderUnitHost(t, "loaded", "inactive", true)
	probe, _ := startingProbe(1)
	d := HostDoctor{Host: h, Spec: s, ASRProbe: probe, Color: true}
	first := d.checkAsrEngine(context.Background(), checkFor(t, s, "asr-engine"))
	var out bytes.Buffer
	if _, err := d.Repair(context.Background(), Report{Checks: []Result{first}}, Options{Fix: true, StateDir: t.TempDir()}, strings.NewReader("y\n"), &out, false, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "\x1b[33mstarting lmcoder-asr.service costs:\x1b[0m") {
		t.Fatalf("cost notice should be yellow:\n%q", out.String())
	}
	if !strings.Contains(out.String(), "\x1b[31mFAULT\x1b[0m asr-engine") {
		t.Fatalf("the verdict line should be red:\n%q", out.String())
	}
}

func TestStyleIsPlainWhenOff(t *testing.T) {
	off := style{}
	for _, st := range []Status{OK, WARN, FAULT, ERROR, SKIP} {
		if got := off.status(st); got != string(st) {
			t.Errorf("status(%s) = %q, want plain", st, got)
		}
	}
	if got := off.notice("costs"); got != "costs" {
		t.Errorf("notice = %q, want plain", got)
	}
}
