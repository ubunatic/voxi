package mic

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/spec"
)

// headsetSpec returns the real bluetooth_mic spec (issue 210).
func headsetSpec(t *testing.T) spec.BluetoothMicSpec {
	t.Helper()
	es, err := spec.LoadEager()
	if err != nil {
		t.Fatal(err)
	}
	return es.BluetoothMic
}

// fakeHost records every command and answers wpctl/systemctl from its fields.
// File operations use the real filesystem under t.TempDir.
type fakeHost struct {
	home     string
	noWpctl  bool
	wpActive bool
	runErr   error // returned by Run (wpctl settings writes)
	calls    []string
}

func (f *fakeHost) deps() deps.Dependencies {
	return deps.Dependencies{
		Getenv: func(k string) string {
			switch k {
			case "HOME":
				return f.home
			}
			return ""
		},
		LookPath: func(name string) (string, error) {
			if name == "wpctl" && f.noWpctl {
				return "", errors.New("not found")
			}
			return "/usr/bin/" + name, nil
		},
		RunOutput: func(_ context.Context, name string, args ...string) (string, error) {
			f.calls = append(f.calls, name+" "+strings.Join(args, " "))
			if name == "systemctl" && f.wpActive {
				return "active\n", nil
			}
			return "inactive\n", errors.New("exit status 3")
		},
		Run: func(_ context.Context, name string, args ...string) error {
			f.calls = append(f.calls, name+" "+strings.Join(args, " "))
			return f.runErr
		},
		ReadFile:  os.ReadFile,
		WriteFile: os.WriteFile,
		MkdirAll:  os.MkdirAll,
		Remove:    os.Remove,
		Stdout:    os.Stdout,
	}
}

func (f *fakeHost) autoswitchCalls() []string {
	var out []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, "wpctl settings --save bluetooth.autoswitch-to-headset-profile") {
			out = append(out, c)
		}
	}
	return out
}

func newFakeHeadset(t *testing.T, wpActive bool) (*fakeHost, Headset) {
	t.Helper()
	f := &fakeHost{home: t.TempDir(), wpActive: wpActive}
	return f, NewHeadset(f.deps(), headsetSpec(t))
}

func TestHeadsetDropinPathUsesXDGThenHome(t *testing.T) {
	bm := headsetSpec(t)
	d := deps.Dependencies{Getenv: func(k string) string {
		if k == "XDG_CONFIG_HOME" {
			return "/x/cfg"
		}
		return "/home/u"
	}}
	want := filepath.Join("/x/cfg", "wireplumber", "wireplumber.conf.d", bm.DropinName)
	if got := NewHeadset(d, bm).DropinPath(); got != want {
		t.Fatalf("DropinPath() = %q, want %q", got, want)
	}
	d.Getenv = func(k string) string {
		if k == "HOME" {
			return "/home/u"
		}
		return ""
	}
	want = filepath.Join("/home/u", ".config", "wireplumber", "wireplumber.conf.d", bm.DropinName)
	if got := NewHeadset(d, bm).DropinPath(); got != want {
		t.Fatalf("DropinPath() without XDG = %q, want %q", got, want)
	}
}

func TestHeadsetRenderFollowsSpec(t *testing.T) {
	bm := headsetSpec(t)
	bm.SuspendTimeoutSeconds = 7
	out := (Headset{bm: bm}).Render()
	if !strings.HasPrefix(out, bm.DropinMarker+"\n") {
		t.Fatalf("render does not start with the marker: %q", out)
	}
	for _, want := range []string{
		`"~bluez_input.*"`, `"~bluez_capture_internal.*"`, "session.suspend-timeout-seconds = 7",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %s:\n%s", want, out)
		}
	}
}

func TestHeadsetEnableWritesDropinAndTurnsAutoswitchOff(t *testing.T) {
	f, h := newFakeHeadset(t, true)
	res, err := h.Enable(context.Background())
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if !res.DropinChanged {
		t.Fatal("DropinChanged = false on a fresh host")
	}
	data, err := os.ReadFile(h.DropinPath())
	if err != nil {
		t.Fatalf("drop-in not written: %v", err)
	}
	if string(data) != h.Render() {
		t.Fatalf("drop-in content differs from Render()")
	}
	if got := f.autoswitchCalls(); len(got) != 1 || !strings.HasSuffix(got[0], " false") {
		t.Fatalf("autoswitch calls = %v, want one ending in false", got)
	}
}

func TestHeadsetEnableIsIdempotent(t *testing.T) {
	_, h := newFakeHeadset(t, true)
	if _, err := h.Enable(context.Background()); err != nil {
		t.Fatal(err)
	}
	res, err := h.Enable(context.Background())
	if err != nil {
		t.Fatalf("second Enable() error = %v", err)
	}
	if res.DropinChanged {
		t.Fatal("second Enable() rewrote an identical drop-in")
	}
}

func TestHeadsetEnableRewritesOwnedDropinFromManualFix(t *testing.T) {
	// The host's manual fix carries the marker, so voxi owns it and may
	// replace it with the rendered text.
	f, h := newFakeHeadset(t, true)
	manual := "# voxi: keep the Bluetooth headset mic link awake so the first ~1 s is kept (manual)\nmonitor.bluez.rules = []\n"
	mustWrite(t, h.DropinPath(), manual)
	res, err := h.Enable(context.Background())
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if !res.DropinChanged {
		t.Fatal("manual drop-in with the marker was not replaced")
	}
	if got := readFile(t, h.DropinPath()); got != h.Render() {
		t.Fatal("owned drop-in not rewritten to the rendered text")
	}
	_ = f
}

// Negatives: a drop-in voxi does not own is never touched.
func TestHeadsetEnableRefusesForeignDropin(t *testing.T) {
	f, h := newFakeHeadset(t, true)
	foreign := "monitor.bluez.rules = []\n"
	mustWrite(t, h.DropinPath(), foreign)
	if _, err := h.Enable(context.Background()); err == nil {
		t.Fatal("Enable() accepted a foreign drop-in")
	}
	if got := readFile(t, h.DropinPath()); got != foreign {
		t.Fatalf("foreign drop-in was modified: %q", got)
	}
	if len(f.autoswitchCalls()) != 0 {
		t.Fatal("autoswitch changed although the drop-in was refused")
	}
}

func TestHeadsetEnableNeedsWirePlumberInstalled(t *testing.T) {
	f, h := newFakeHeadset(t, true)
	f.noWpctl = true
	if _, err := h.Enable(context.Background()); err == nil {
		t.Fatal("Enable() succeeded without wpctl")
	}
	if _, err := os.Stat(h.DropinPath()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("drop-in written without WirePlumber: %v", err)
	}
}

func TestHeadsetEnableNeedsWirePlumberRunning(t *testing.T) {
	f, h := newFakeHeadset(t, false)
	if _, err := h.Enable(context.Background()); err == nil {
		t.Fatal("Enable() succeeded while WirePlumber is inactive")
	}
	if _, err := os.Stat(h.DropinPath()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("drop-in written while WirePlumber is inactive: %v", err)
	}
	if len(f.autoswitchCalls()) != 0 {
		t.Fatal("autoswitch changed while WirePlumber is inactive")
	}
}

func TestHeadsetEnableReportsPartialWhenAutoswitchFails(t *testing.T) {
	f, h := newFakeHeadset(t, true)
	f.runErr = errors.New("wpctl: settings failed")
	_, err := h.Enable(context.Background())
	if err == nil || !strings.Contains(err.Error(), "drop-in written") {
		t.Fatalf("Enable() error = %v, want a partial-state error naming the drop-in", err)
	}
	if _, statErr := os.Stat(h.DropinPath()); statErr != nil {
		t.Fatalf("drop-in missing after partial enable: %v", statErr)
	}
}

func TestHeadsetDisableRemovesOwnedDropinAndRestoresAutoswitch(t *testing.T) {
	f, h := newFakeHeadset(t, true)
	if _, err := h.Enable(context.Background()); err != nil {
		t.Fatal(err)
	}
	res, err := h.Disable(context.Background())
	if err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if !res.Removed {
		t.Fatal("Disable() did not report removing the drop-in")
	}
	if _, err := os.Stat(h.DropinPath()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("drop-in still present after Disable: %v", err)
	}
	calls := f.autoswitchCalls()
	if len(calls) != 2 || !strings.HasSuffix(calls[1], " true") {
		t.Fatalf("autoswitch calls = %v, want the last one restoring true", calls)
	}
}

func TestHeadsetDisableWithoutDropinIsNoOp(t *testing.T) {
	_, h := newFakeHeadset(t, true)
	res, err := h.Disable(context.Background())
	if err != nil {
		t.Fatalf("Disable() on a clean host error = %v", err)
	}
	if res.Removed {
		t.Fatal("Disable() reported removing a drop-in that was never there")
	}
}

// Negatives: undo never deletes a file voxi does not own.
func TestHeadsetDisableRefusesForeignDropin(t *testing.T) {
	f, h := newFakeHeadset(t, true)
	foreign := "monitor.bluez.rules = []\n"
	mustWrite(t, h.DropinPath(), foreign)
	if _, err := h.Disable(context.Background()); err == nil {
		t.Fatal("Disable() removed a foreign drop-in")
	}
	if got := readFile(t, h.DropinPath()); got != foreign {
		t.Fatalf("foreign drop-in was modified: %q", got)
	}
	if len(f.autoswitchCalls()) != 0 {
		t.Fatal("autoswitch restored although the drop-in was refused")
	}
}

func TestHeadsetDisableWorksWhenWirePlumberStopped(t *testing.T) {
	// Undo must still work after WirePlumber has stopped (e.g. a crash).
	f, h := newFakeHeadset(t, true)
	if _, err := h.Enable(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.wpActive = false
	if _, err := h.Disable(context.Background()); err != nil {
		t.Fatalf("Disable() with WirePlumber stopped error = %v", err)
	}
}

func TestHeadsetDisableNeedsWpctl(t *testing.T) {
	f, h := newFakeHeadset(t, true)
	if _, err := h.Enable(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.noWpctl = true
	if _, err := h.Disable(context.Background()); err == nil {
		t.Fatal("Disable() succeeded without wpctl")
	}
	if _, err := os.Stat(h.DropinPath()); err != nil {
		t.Fatalf("drop-in removed although autoswitch could not be restored: %v", err)
	}
}

func TestHeadsetStatusLine(t *testing.T) {
	_, h := newFakeHeadset(t, true)
	if got := h.StatusLine(); !strings.Contains(got, "off") {
		t.Fatalf("StatusLine() on clean host = %q, want off", got)
	}
	if _, err := h.Enable(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := h.StatusLine(); !strings.Contains(got, "on") {
		t.Fatalf("StatusLine() after enable = %q, want on", got)
	}
	mustWrite(t, h.DropinPath(), "monitor.bluez.rules = []\n")
	if got := h.StatusLine(); !strings.Contains(got, "not voxi") {
		t.Fatalf("StatusLine() with foreign drop-in = %q", got)
	}
}

func mustWrite(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
