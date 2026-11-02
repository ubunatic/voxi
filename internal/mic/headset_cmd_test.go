package mic

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// runHeadset executes `voxi mic headset <args>` against a fake host and returns stdout.
func runHeadset(t *testing.T, f *fakeHost, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	d := f.deps()
	d.Stdout = &out
	mic := NewCommand(d, time.Millisecond)
	mic.SetArgs(append([]string{"headset"}, args...))
	mic.SetOut(&out)
	mic.SetErr(&out)
	mic.SetContext(context.Background())
	err := mic.Execute()
	return out.String(), err
}

// newCmdHost is a fake host with WirePlumber running and autoswitch on.
func newCmdHost(t *testing.T) *fakeHost {
	return &fakeHost{home: t.TempDir(), wpActive: true, autoswitch: "true", dump: "[]"}
}

func TestHeadsetCommandEnableThenOff(t *testing.T) {
	f := newCmdHost(t)
	out, err := runHeadset(t, f)
	if err != nil {
		t.Fatalf("voxi mic headset: %v\n%s", err, out)
	}
	if !strings.Contains(out, "turned off") || !strings.Contains(out, "--off") {
		t.Fatalf("enable output lacks the autoswitch note or the undo hint:\n%s", out)
	}
	if !strings.Contains(out, "--off will set it back to true") {
		t.Fatalf("enable output does not say what undo restores:\n%s", out)
	}
	if _, statErr := os.Stat(NewHeadset(f.deps(), headsetSpec(t)).DropinPath()); statErr != nil {
		t.Fatalf("drop-in not written by voxi mic headset: %v", statErr)
	}
	out, err = runHeadset(t, f, "--off")
	if err != nil {
		t.Fatalf("voxi mic headset --off: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Removed") || !strings.Contains(out, "restored") {
		t.Fatalf("off output does not report removal and restore:\n%s", out)
	}
}

func TestHeadsetCommandStatusChangesNothing(t *testing.T) {
	f := newCmdHost(t)
	out, err := runHeadset(t, f, "--status")
	if err != nil {
		t.Fatalf("voxi mic headset --status: %v", err)
	}
	if want := "Bluetooth keep-awake: off (enable with voxi mic headset)\n"; out != want {
		t.Fatalf("status on a clean host = %q, want %q", out, want)
	}
	if len(f.autoswitchWrites()) != 0 || len(f.pactlCalls()) != 0 {
		t.Fatalf("--status changed the host: %v", f.calls)
	}
	if _, statErr := os.Stat(NewHeadset(f.deps(), headsetSpec(t)).DropinPath()); statErr == nil {
		t.Fatal("--status wrote a drop-in")
	}
}

func TestHeadsetCommandRejectsOffAndStatusTogether(t *testing.T) {
	f := newCmdHost(t)
	if _, err := runHeadset(t, f, "--off", "--status"); err == nil {
		t.Fatal("--off --status was accepted")
	}
	if len(f.autoswitchWrites()) != 0 {
		t.Fatal("rejected flag combination still changed autoswitch")
	}
}

// Negative: --adopt only applies to enabling.
func TestHeadsetCommandRejectsAdoptWithOff(t *testing.T) {
	f := newCmdHost(t)
	if _, err := runHeadset(t, f, "--off", "--adopt"); err == nil {
		t.Fatal("--off --adopt was accepted")
	}
}

// Negative: an enable refused for WirePlumber reasons surfaces as a command error.
func TestHeadsetCommandSurfacesWirePlumberError(t *testing.T) {
	f := newCmdHost(t)
	f.wpActive = false
	_, err := runHeadset(t, f)
	if err == nil {
		t.Fatal("voxi mic headset succeeded with WirePlumber stopped")
	}
}
