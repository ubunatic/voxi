package mic

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
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

// fakeHost records every command and answers wpctl, systemctl and pw-dump from
// its fields. File operations use the real filesystem under t.TempDir.
type fakeHost struct {
	home       string
	noWpctl    bool
	noPwDump   bool
	wpActive   bool
	autoswitch string // the WirePlumber setting; `wpctl settings --save` updates it
	readErr    error  // returned by `wpctl settings KEY` (a read)
	readOut    string // overrides the read output when set
	runErr     error  // returned by `wpctl settings --save`
	pactlErr   error  // returned by `pactl set-card-profile`
	dump       string // pw-dump output
	calls      []string
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
			if (name == "wpctl" && f.noWpctl) || (name == "pw-dump" && f.noPwDump) {
				return "", errors.New("not found")
			}
			return "/usr/bin/" + name, nil
		},
		RunOutput: func(_ context.Context, name string, args ...string) (string, error) {
			f.calls = append(f.calls, name+" "+strings.Join(args, " "))
			switch {
			case name == "systemctl" && f.wpActive:
				return "active\n", nil
			case name == "wpctl" && len(args) == 2 && args[0] == "settings":
				if f.readErr != nil {
					return "", f.readErr
				}
				if f.readOut != "" {
					return f.readOut, nil
				}
				return "Value: " + f.autoswitch + " (Saved: " + f.autoswitch + ")\n", nil
			case name == "pw-dump":
				return f.dump, nil
			}
			return "inactive\n", errors.New("exit status 3")
		},
		Run: func(_ context.Context, name string, args ...string) error {
			f.calls = append(f.calls, name+" "+strings.Join(args, " "))
			if name == "pactl" {
				return f.pactlErr
			}
			if name == "wpctl" && len(args) == 4 && args[0] == "settings" && args[1] == "--save" {
				if f.runErr != nil {
					return f.runErr
				}
				f.autoswitch = args[3]
			}
			return nil
		},
		ReadFile:  os.ReadFile,
		WriteFile: os.WriteFile,
		MkdirAll:  os.MkdirAll,
		Remove:    os.Remove,
		Stdout:    os.Stdout,
	}
}

// autoswitchWrites returns the `wpctl settings --save` calls for the setting.
func (f *fakeHost) autoswitchWrites() []string {
	var out []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, "wpctl settings --save bluetooth.autoswitch-to-headset-profile") {
			out = append(out, c)
		}
	}
	return out
}

// pactlCalls returns the card profile commands.
func (f *fakeHost) pactlCalls() []string {
	var out []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, "pactl ") {
			out = append(out, c)
		}
	}
	return out
}

func newFakeHeadset(t *testing.T, wpActive bool) (*fakeHost, Headset) {
	t.Helper()
	f := &fakeHost{home: t.TempDir(), wpActive: wpActive, autoswitch: "true", dump: "[]"}
	return f, NewHeadset(f.deps(), headsetSpec(t))
}

// pwDumpBluetoothCard is a pw-dump with one bluez card whose active profile is
// active and whose profiles are listed in profiles (name -> has a mic).
func pwDumpBluetoothCard(active string, withHeadset bool) string {
	headset := ""
	if withHeadset {
		headset = `,{"index":2,"name":"headset-head-unit","available":"yes","classes":[["Audio/Source",1]]}`
	}
	index := 1 // a2dp-sink
	if active == "headset-head-unit" {
		index = 2
	}
	return `[{"type":"PipeWire:Interface:Device","id":7,"info":{"props":{"media.class":"Audio/Device","device.name":"bluez_card.AA","device.api":"bluez5"},` +
		`"params":{"EnumProfile":[{"index":1,"name":"a2dp-sink","available":"yes"}` + headset + `],` +
		`"Profile":[{"index":` + strconv.Itoa(index) + `,"name":"` + active + `"}]}}}]`
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
	out := (Headset{bm: bm}).Render("")
	if !strings.HasPrefix(out, bm.DropinMarker+"\n") {
		t.Fatalf("render does not start with the marker line: %q", out)
	}
	for _, want := range []string{
		`"~bluez_input.*"`, `"~bluez_capture_internal.*"`, "session.suspend-timeout-seconds = 7",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %s:\n%s", want, out)
		}
	}
	if strings.Contains(out, bm.RestoreRecordPrefix) {
		t.Fatalf("render without a restore value wrote a record line:\n%s", out)
	}
	if rec := (Headset{bm: bm}).Render("true"); !strings.Contains(rec, bm.RestoreRecordPrefix+"true\n") {
		t.Fatalf("render with restore true has no record line:\n%s", rec)
	}
}

func TestHeadsetEnableWritesDropinAndTurnsAutoswitchOff(t *testing.T) {
	f, h := newFakeHeadset(t, true)
	res, err := h.Enable(context.Background(), false)
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if !res.DropinChanged || !res.AutoswitchWasOn {
		t.Fatalf("Enable() = %+v, want a new drop-in and autoswitch recorded as on", res)
	}
	if got := readFile(t, h.DropinPath()); got != h.Render("true") {
		t.Fatalf("drop-in content differs from Render(\"true\"):\n%s", got)
	}
	if got := f.autoswitchWrites(); len(got) != 1 || !strings.HasSuffix(got[0], " false") {
		t.Fatalf("autoswitch writes = %v, want one ending in false", got)
	}
}

func TestHeadsetEnableIsIdempotent(t *testing.T) {
	_, h := newFakeHeadset(t, true)
	if _, err := h.Enable(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	res, err := h.Enable(context.Background(), false)
	if err != nil {
		t.Fatalf("second Enable() error = %v", err)
	}
	if res.DropinChanged {
		t.Fatal("second Enable() rewrote an identical drop-in")
	}
}

// A re-run must keep the restore record: autoswitch is already off by then,
// so reading it again would wrongly record "nothing to restore".
func TestHeadsetEnableRerunKeepsRestoreRecord(t *testing.T) {
	f, h := newFakeHeadset(t, true)
	if _, err := h.Enable(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Enable(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Disable(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.autoswitch != "true" {
		t.Fatalf("autoswitch after enable+enable+off = %q, want true", f.autoswitch)
	}
}

// Negative: autoswitch that was already off is not voxi's to restore, so
// --off must not turn it on.
func TestHeadsetAutoswitchAlreadyOffIsNotRestored(t *testing.T) {
	f, h := newFakeHeadset(t, true)
	f.autoswitch = "false"
	res, err := h.Enable(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if res.AutoswitchWasOn {
		t.Fatal("Enable() recorded autoswitch as on although it was off")
	}
	if strings.Contains(readFile(t, h.DropinPath()), h.bm.RestoreRecordPrefix) {
		t.Fatal("drop-in carries a restore record although voxi did not turn autoswitch off")
	}
	if _, err := h.Disable(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.autoswitchWrites() {
		if strings.HasSuffix(c, " true") {
			t.Fatalf("Disable() restored autoswitch that voxi never changed: %v", f.autoswitchWrites())
		}
	}
}

// Negative: when autoswitch cannot be read, Enable changes nothing at all,
// because it cannot know what undo should restore.
func TestHeadsetEnableRefusesUnreadableAutoswitch(t *testing.T) {
	f, h := newFakeHeadset(t, true)
	f.readErr = errors.New("wpctl: settings read failed")
	if _, err := h.Enable(context.Background(), false); err == nil {
		t.Fatal("Enable() succeeded although autoswitch could not be read")
	}
	if _, err := os.Stat(h.DropinPath()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("drop-in written although autoswitch was unreadable: %v", err)
	}
	if len(f.autoswitchWrites()) != 0 {
		t.Fatal("autoswitch written although it could not be read")
	}
}

func TestHeadsetEnableRefusesMalformedAutoswitchOutput(t *testing.T) {
	f, h := newFakeHeadset(t, true)
	f.readOut = "something unexpected\n"
	if _, err := h.Enable(context.Background(), false); err == nil {
		t.Fatal("Enable() accepted unparseable wpctl output")
	}
	if _, err := os.Stat(h.DropinPath()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("drop-in written from unparseable output: %v", err)
	}
}

// Negative: a drop-in from the earlier manual fix carries the old marker text,
// which is not the ownership token, so it is someone else's.
func TestHeadsetEnableRefusesManualDropin(t *testing.T) {
	f, h := newFakeHeadset(t, true)
	manual := "# voxi: keep the Bluetooth headset mic link awake so the first ~1 s is kept (manual)\nmonitor.bluez.rules = []\n"
	mustWrite(t, h.DropinPath(), manual)
	_, err := h.Enable(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "--adopt") {
		t.Fatalf("Enable() error = %v, want a refusal naming --adopt", err)
	}
	if got := readFile(t, h.DropinPath()); got != manual {
		t.Fatalf("manual drop-in was modified: %q", got)
	}
	if len(f.autoswitchWrites()) != 0 {
		t.Fatal("autoswitch changed although the drop-in was refused")
	}
}

// Negative: the marker only counts on the first line.
func TestHeadsetMarkerMustBeFirstLine(t *testing.T) {
	_, h := newFakeHeadset(t, true)
	foreign := "monitor.bluez.rules = []\n" + h.bm.DropinMarker + "\n"
	mustWrite(t, h.DropinPath(), foreign)
	if _, err := h.Enable(context.Background(), false); err == nil {
		t.Fatal("Enable() treated a marker on line 2 as ownership")
	}
	if got := readFile(t, h.DropinPath()); got != foreign {
		t.Fatalf("drop-in with the marker on line 2 was modified")
	}
}

func TestHeadsetEnableAdoptReplacesForeignDropin(t *testing.T) {
	f, h := newFakeHeadset(t, true)
	mustWrite(t, h.DropinPath(), "monitor.bluez.rules = []\n")
	res, err := h.Enable(context.Background(), true)
	if err != nil {
		t.Fatalf("Enable(adopt) error = %v", err)
	}
	if !res.DropinChanged {
		t.Fatal("adopt did not replace the foreign drop-in")
	}
	if got := readFile(t, h.DropinPath()); got != h.Render("true") {
		t.Fatalf("adopted drop-in is not the rendered text:\n%s", got)
	}
	if len(f.autoswitchWrites()) != 1 {
		t.Fatal("adopt did not turn autoswitch off")
	}
}

func TestHeadsetEnableNeedsWirePlumberInstalled(t *testing.T) {
	f, h := newFakeHeadset(t, true)
	f.noWpctl = true
	if _, err := h.Enable(context.Background(), false); err == nil {
		t.Fatal("Enable() succeeded without wpctl")
	}
	if _, err := os.Stat(h.DropinPath()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("drop-in written without WirePlumber: %v", err)
	}
}

func TestHeadsetEnableNeedsWirePlumberRunning(t *testing.T) {
	f, h := newFakeHeadset(t, false)
	if _, err := h.Enable(context.Background(), false); err == nil {
		t.Fatal("Enable() succeeded while WirePlumber is inactive")
	}
	if _, err := os.Stat(h.DropinPath()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("drop-in written while WirePlumber is inactive: %v", err)
	}
	if len(f.autoswitchWrites()) != 0 {
		t.Fatal("autoswitch changed while WirePlumber is inactive")
	}
}

func TestHeadsetEnableReportsPartialWhenAutoswitchFails(t *testing.T) {
	f, h := newFakeHeadset(t, true)
	f.runErr = errors.New("wpctl: settings failed")
	_, err := h.Enable(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "drop-in written") {
		t.Fatalf("Enable() error = %v, want a partial-state error naming the drop-in", err)
	}
	if _, statErr := os.Stat(h.DropinPath()); statErr != nil {
		t.Fatalf("drop-in missing after partial enable: %v", statErr)
	}
}

// The mic is only available in the headset profile while autoswitch is off,
// so enable moves a Bluetooth card on A2DP to it.
func TestHeadsetEnablePinsA2DPCardToHeadset(t *testing.T) {
	f, h := newFakeHeadset(t, true)
	f.dump = pwDumpBluetoothCard("a2dp-sink", true)
	res, err := h.Enable(context.Background(), false)
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if len(res.PinnedCards) != 1 || res.PinnedCards[0] != "bluez_card.AA" {
		t.Fatalf("PinnedCards = %v, want [bluez_card.AA]", res.PinnedCards)
	}
	if got := f.pactlCalls(); len(got) != 1 || got[0] != "pactl set-card-profile bluez_card.AA headset-head-unit" {
		t.Fatalf("pactl calls = %v", got)
	}
}

// Negatives: no pin when the card is already on the headset profile, or when
// it has no headset profile to pin to.
func TestHeadsetEnableDoesNotPinWhenNotOnA2DP(t *testing.T) {
	cases := map[string]string{
		"already on headset": pwDumpBluetoothCard("headset-head-unit", true),
		"no headset profile": pwDumpBluetoothCard("a2dp-sink", false),
		"no card at all":     "[]",
	}
	for name, dump := range cases {
		t.Run(name, func(t *testing.T) {
			f, h := newFakeHeadset(t, true)
			f.dump = dump
			res, err := h.Enable(context.Background(), false)
			if err != nil {
				t.Fatalf("Enable() error = %v", err)
			}
			if len(f.pactlCalls()) != 0 || len(res.PinnedCards) != 0 {
				t.Fatalf("card pinned: calls %v, result %v", f.pactlCalls(), res.PinnedCards)
			}
		})
	}
}

func TestHeadsetEnableReportsPartialWhenPinFails(t *testing.T) {
	f, h := newFakeHeadset(t, true)
	f.dump = pwDumpBluetoothCard("a2dp-sink", true)
	f.pactlErr = errors.New("pactl: no such card")
	_, err := h.Enable(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "not pinned") {
		t.Fatalf("Enable() error = %v, want a partial-state error naming the pin", err)
	}
	if _, statErr := os.Stat(h.DropinPath()); statErr != nil {
		t.Fatalf("drop-in missing after partial enable: %v", statErr)
	}
}

func TestHeadsetDisableRemovesOwnedDropinAndRestoresAutoswitch(t *testing.T) {
	f, h := newFakeHeadset(t, true)
	if _, err := h.Enable(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	res, err := h.Disable(context.Background())
	if err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if !res.Removed || !res.AutoswitchRestored {
		t.Fatalf("Disable() = %+v, want drop-in removed and autoswitch restored", res)
	}
	if _, err := os.Stat(h.DropinPath()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("drop-in still present after Disable: %v", err)
	}
	writes := f.autoswitchWrites()
	if len(writes) != 2 || !strings.HasSuffix(writes[1], " true") {
		t.Fatalf("autoswitch writes = %v, want the last one restoring true", writes)
	}
}

// Negatives: a missing drop-in is a no-op that does not even call wpctl.
func TestHeadsetDisableWithoutDropinIsNoOp(t *testing.T) {
	f, h := newFakeHeadset(t, true)
	f.noWpctl = true
	res, err := h.Disable(context.Background())
	if err != nil {
		t.Fatalf("Disable() on a clean host error = %v", err)
	}
	if res.Removed || res.AutoswitchRestored {
		t.Fatalf("Disable() on a clean host = %+v", res)
	}
	if len(f.calls) != 0 {
		t.Fatalf("Disable() on a clean host ran commands: %v", f.calls)
	}
}

// Negative: undo never deletes a file voxi does not own.
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
	if len(f.autoswitchWrites()) != 0 {
		t.Fatal("autoswitch restored although the drop-in was refused")
	}
}

// Undo must still work after WirePlumber has stopped (e.g. a crash).
func TestHeadsetDisableWorksWhenWirePlumberStopped(t *testing.T) {
	f, h := newFakeHeadset(t, true)
	if _, err := h.Enable(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	f.wpActive = false
	if _, err := h.Disable(context.Background()); err != nil {
		t.Fatalf("Disable() with WirePlumber stopped error = %v", err)
	}
}

// Negative: restoring autoswitch needs wpctl, and without it the drop-in stays.
func TestHeadsetDisableNeedsWpctl(t *testing.T) {
	f, h := newFakeHeadset(t, true)
	if _, err := h.Enable(context.Background(), false); err != nil {
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

// Without a restore record, undo needs no wpctl at all.
func TestHeadsetDisableWithoutRecordNeedsNoWpctl(t *testing.T) {
	f, h := newFakeHeadset(t, true)
	f.autoswitch = "false"
	if _, err := h.Enable(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	f.noWpctl = true
	if _, err := h.Disable(context.Background()); err != nil {
		t.Fatalf("Disable() without a record and without wpctl error = %v", err)
	}
	if _, err := os.Stat(h.DropinPath()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("drop-in not removed: %v", err)
	}
}

func TestHeadsetStatusLine(t *testing.T) {
	_, h := newFakeHeadset(t, true)
	const off = "Bluetooth keep-awake: off (enable with voxi mic headset)"
	const on = "Bluetooth keep-awake: on"
	const foreign = "Bluetooth keep-awake: not voxi's drop-in (left alone)"
	if got := h.StatusLine(); got != off {
		t.Fatalf("StatusLine() on clean host = %q, want %q", got, off)
	}
	if _, err := h.Enable(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if got := h.StatusLine(); got != on {
		t.Fatalf("StatusLine() after enable = %q, want %q", got, on)
	}
	mustWrite(t, h.DropinPath(), "monitor.bluez.rules = []\n")
	if got := h.StatusLine(); got != foreign {
		t.Fatalf("StatusLine() with foreign drop-in = %q, want %q", got, foreign)
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
