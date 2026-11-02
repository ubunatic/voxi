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

func TestHeadsetCommandEnableThenOff(t *testing.T) {
	f := &fakeHost{home: t.TempDir(), wpActive: true}
	out, err := runHeadset(t, f)
	if err != nil {
		t.Fatalf("voxi mic headset: %v\n%s", err, out)
	}
	if !strings.Contains(out, "turned off") || !strings.Contains(out, "--off") {
		t.Fatalf("enable output lacks the autoswitch note or the undo hint:\n%s", out)
	}
	if _, statErr := os.Stat(NewHeadset(f.deps(), headsetSpec(t)).DropinPath()); statErr != nil {
		t.Fatalf("drop-in not written by voxi mic headset: %v", statErr)
	}
	out, err = runHeadset(t, f, "--off")
	if err != nil {
		t.Fatalf("voxi mic headset --off: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Removed") {
		t.Fatalf("off output does not report removal:\n%s", out)
	}
}

func TestHeadsetCommandStatusChangesNothing(t *testing.T) {
	f := &fakeHost{home: t.TempDir(), wpActive: true}
	out, err := runHeadset(t, f, "--status")
	if err != nil {
		t.Fatalf("voxi mic headset --status: %v", err)
	}
	if !strings.Contains(out, "off") {
		t.Fatalf("status on a clean host = %q, want off", out)
	}
	if len(f.autoswitchCalls()) != 0 {
		t.Fatal("--status changed autoswitch")
	}
	if _, statErr := os.Stat(NewHeadset(f.deps(), headsetSpec(t)).DropinPath()); statErr == nil {
		t.Fatal("--status wrote a drop-in")
	}
}

func TestHeadsetCommandRejectsOffAndStatusTogether(t *testing.T) {
	f := &fakeHost{home: t.TempDir(), wpActive: true}
	if _, err := runHeadset(t, f, "--off", "--status"); err == nil {
		t.Fatal("--off --status was accepted")
	}
	if len(f.autoswitchCalls()) != 0 {
		t.Fatal("rejected flag combination still changed autoswitch")
	}
}

// Negative: an enable refused for WirePlumber reasons surfaces as a command error.
func TestHeadsetCommandSurfacesWirePlumberError(t *testing.T) {
	f := &fakeHost{home: t.TempDir(), wpActive: false}
	_, err := runHeadset(t, f)
	if err == nil {
		t.Fatal("voxi mic headset succeeded with WirePlumber stopped")
	}
}
