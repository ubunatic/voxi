package mic

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func loadState(t *testing.T) State {
	t.Helper()
	data, err := os.ReadFile("testdata/bt-a2dp-stale-default.json")
	if err != nil {
		t.Fatal(err)
	}
	st, err := ParseDump(data)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestParseDump(t *testing.T) {
	st := loadState(t)
	if st.DefaultSource != "bluez_input.00:11:22:33:44:55" || !strings.Contains(st.ConfiguredSource, "Depstech") {
		t.Fatalf("defaults: %q %q", st.DefaultSource, st.ConfiguredSource)
	}
	src, ok := st.Source(st.DefaultSource)
	if !ok || !src.Bluetooth || src.DeviceID != 82 {
		t.Fatalf("bluetooth source: %+v", src)
	}
	dev, _ := st.Device(82)
	if dev.Active.Name != "a2dp-sink" || dev.Active.Sources != 0 || !dev.IsHeadphone() {
		t.Fatalf("headphones: %+v", dev)
	}
	if p, ok := dev.HeadsetProfile(); !ok || p.Index != 196865 {
		t.Fatalf("headset profile: %+v", p)
	}
	alsa, _ := st.Device(46)
	if alsa.Active.Sources != 2 {
		t.Fatalf("alsa sources: %+v", alsa.Active)
	}
}

func kinds(ps []Problem) []Kind {
	var out []Kind
	for _, p := range ps {
		out = append(out, p.Kind)
	}
	return out
}

func TestDiagnoseAndPlan(t *testing.T) {
	st := loadState(t)
	probs := Diagnose(st)
	if got := kinds(probs); !reflect.DeepEqual(got, []Kind{StaleConfigured, BluetoothNoMic}) {
		t.Fatalf("problems: %v", got)
	}
	var cmds []string
	for _, a := range Plan(st, probs) {
		cmds = append(cmds, strings.Join(a.Cmd, " "))
	}
	want := []string{
		"pw-metadata -d 0 default.configured.audio.source",
		"wpctl set-profile 82 196865", // the speaker (id 90) is left alone
	}
	if !reflect.DeepEqual(cmds, want) {
		t.Fatalf("plan:\n got %v\nwant %v", cmds, want)
	}
}

func TestDiagnoseHealthy(t *testing.T) {
	st := loadState(t)
	st.DefaultSource = "alsa_input.pci-0000_07_00.6.HiFi__Mic1__source"
	// Stale saved default alone must not stop a working recording.
	doc := Doctor{Probe: func(context.Context) (State, error) { return st, nil }}
	rep, err := doc.Check(context.Background())
	if err != nil || rep.Found() {
		t.Fatalf("healthy: %+v %v", rep, err)
	}
}

// WirePlumber's autoswitch loopback in A2DP is the normal state: recording
// from it switches the card to headset mode by itself.
func TestDiagnoseAutoswitchLoopbackIsHealthy(t *testing.T) {
	st := loadState(t)
	for i := range st.Sources {
		st.Sources[i].Loopback = st.Sources[i].Bluetooth
	}
	doc := Doctor{Probe: func(context.Context) (State, error) { return st, nil }}
	if _, probs, err := doc.Find(context.Background()); err != nil || len(probs) != 0 {
		t.Fatalf("loopback: %v %v", probs, err)
	}
}

func TestPlanDeadBluetoothFallsBackToBuiltin(t *testing.T) {
	st := loadState(t)
	acts := Plan(st, []Problem{{Kind: DeadSignal}})
	if len(acts) != 1 || strings.Join(acts[0].Cmd, " ") != "wpctl set-default 66" {
		t.Fatalf("plan: %+v", acts)
	}
}

func TestDoctorRepairsAndReports(t *testing.T) {
	st := loadState(t)
	var ran []string
	doc := Doctor{
		Probe: func(context.Context) (State, error) { return st, nil },
		Run: func(_ context.Context, name string, args ...string) error {
			ran = append(ran, name+" "+strings.Join(args, " "))
			switch name {
			case "pw-metadata":
				st.ConfiguredSource = ""
			case "wpctl":
				if args[0] == "set-profile" {
					d := &st.Devices[1]
					d.Active = d.Profiles[3]
				}
			}
			return nil
		},
		Sleep:  func(time.Duration) {},
		Settle: time.Second,
	}
	rep, err := doc.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ran) != 2 || len(rep.Remaining) != 0 || !rep.Changed() {
		t.Fatalf("ran %v report %+v", ran, rep)
	}
	want := "Mic repaired: cleared saved mic alsa_input.usb-Depstech_webcam-02.analog-stereo; Der Kopfhörer → headset mode. Please record again."
	if rep.Summary() != want {
		t.Fatalf("summary: %q", rep.Summary())
	}
}

func TestDoctorFallsBackToBuiltin(t *testing.T) {
	st := loadState(t)
	st.Devices[1].Profiles = st.Devices[1].Profiles[:2] // no headset profile
	var ran []string
	doc := Doctor{
		Probe: func(context.Context) (State, error) { return st, nil },
		Run: func(_ context.Context, name string, args ...string) error {
			ran = append(ran, name+" "+strings.Join(args, " "))
			if name == "wpctl" && args[0] == "set-default" {
				st.DefaultSource = "alsa_input.pci-0000_07_00.6.HiFi__Mic1__source"
			}
			if name == "pw-metadata" {
				st.ConfiguredSource = ""
			}
			return nil
		},
		Sleep:  func(time.Duration) {},
		Settle: 200 * time.Millisecond,
	}
	rep, err := doc.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ran[len(ran)-1] != "wpctl set-default 66" || len(rep.Remaining) != 0 {
		t.Fatalf("ran %v report %+v", ran, rep)
	}
}

func TestSignalMonitor(t *testing.T) {
	const bps = 32000
	frame := func(v byte) []byte { b := make([]byte, 640); b[0] = v; return b }
	run := func(m *SignalMonitor, secs float64, v byte, every int) time.Time {
		t0 := time.Unix(1000, 0)
		m.Start(t0)
		now := t0
		for i := 0; float64(i)*0.02 < secs; i++ {
			now = t0.Add(time.Duration(i) * 20 * time.Millisecond)
			if i%every == 0 {
				m.Feed(frame(v), now)
			}
		}
		return now
	}
	cases := []struct {
		name  string
		zeros bool
		v     byte
		every int
		dead  bool
	}{
		{"built-in with noise", true, 1, 1, false},
		{"built-in all zero", true, 0, 1, true},
		{"bluetooth all zero is not dead", false, 0, 1, false},
		{"stall", false, 1, 10, true},
	}
	for _, c := range cases {
		m := &SignalMonitor{Window: 3 * time.Second, BytesPerSec: bps, ZerosMeanDead: c.zeros}
		now := run(m, 4, c.v, c.every)
		if got := m.Check(now); got != c.dead {
			t.Errorf("%s: dead=%v, want %v", c.name, got, c.dead)
		}
	}
	m := &SignalMonitor{Window: 3 * time.Second, BytesPerSec: bps, ZerosMeanDead: true}
	now := run(m, 2, 0, 1)
	if m.Check(now) {
		t.Error("must not judge before one window has passed")
	}
}
