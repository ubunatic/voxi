package doctor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/mic"
	"ubunatic.com/voxi/spec"
)

// btLoopbackDefaultDump is a healthy Bluetooth default: WirePlumber's loopback
// input, which the mic check never flags (issue 210).
const btLoopbackDefaultDump = `[
{"type":"PipeWire:Interface:Metadata","id":0,"props":{"metadata.name":"default"},
 "metadata":[{"key":"default.audio.source","value":{"name":"bluez_input.AA"}}]},
{"type":"PipeWire:Interface:Node","id":42,"info":{"props":{"media.class":"Audio/Source","node.name":"bluez_input.AA","device.api":"bluez5","bluez5.loopback":true}}}
]`

// builtinDefaultDump is a healthy built-in default.
const builtinDefaultDump = `[
{"type":"PipeWire:Interface:Metadata","id":0,"props":{"metadata.name":"default"},
 "metadata":[{"key":"default.audio.source","value":{"name":"alsa_input.pci"}}]},
{"type":"PipeWire:Interface:Node","id":42,"info":{"props":{"media.class":"Audio/Source","node.name":"alsa_input.pci","device.api":"alsa"}}}
]`

func writeDump(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pw-dump.json")
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMicCheckReportsKeepAwakeOnBluetoothDefault(t *testing.T) {
	s, err := spec.LoadDoctor()
	if err != nil {
		t.Fatal(err)
	}
	check := checkFor(t, s, "mic")
	hd := HostDoctor{Host: psHost(""), Spec: s, KeepAwake: func(mic.State) string { return "Bluetooth keep-awake: off (enable with voxi mic headset)" }}
	res := hd.checkMic(context.Background(), check, writeDump(t, btLoopbackDefaultDump))
	if res.Status != OK {
		t.Fatalf("status = %v, want OK for a healthy loopback default: %+v", res.Status, res)
	}
	if want := "no microphone source problem; Bluetooth keep-awake: off (enable with voxi mic headset)"; res.Detail != want {
		t.Fatalf("detail = %q, want %q", res.Detail, want)
	}
}

// Negative: a built-in default gets no keep-awake suffix, and no status func
// means no suffix.
func TestMicCheckKeepAwakeLineOnlyForBluetoothDefault(t *testing.T) {
	s, err := spec.LoadDoctor()
	if err != nil {
		t.Fatal(err)
	}
	check := checkFor(t, s, "mic")
	hd := HostDoctor{Host: psHost(""), Spec: s, KeepAwake: keepAwakeStatus}
	res := hd.checkMic(context.Background(), check, writeDump(t, builtinDefaultDump))
	if res.Detail != "no microphone source problem" {
		t.Fatalf("built-in default detail = %q", res.Detail)
	}

	bare := HostDoctor{Host: psHost(""), Spec: s}
	res = bare.checkMic(context.Background(), check, writeDump(t, btLoopbackDefaultDump))
	if res.Detail != "no microphone source problem" {
		t.Fatalf("nil KeepAwake detail = %q", res.Detail)
	}
}

// keepAwakeTestDeps uses a fake HOME so the test never reads the host's drop-in.
func keepAwakeTestDeps(t *testing.T) deps.Dependencies {
	t.Helper()
	home := t.TempDir()
	return deps.Dependencies{
		Getenv:   func(k string) string { return map[string]string{"HOME": home}[k] },
		ReadFile: os.ReadFile,
	}
}

func parsedState(t *testing.T, dump string) mic.State {
	t.Helper()
	st, err := mic.ParseDump([]byte(dump))
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestKeepAwakeLineFollowsDropinStatus(t *testing.T) {
	d := keepAwakeTestDeps(t)
	es, err := spec.LoadEager()
	if err != nil {
		t.Fatal(err)
	}
	st := parsedState(t, btLoopbackDefaultDump)
	const off = "Bluetooth keep-awake: off (enable with voxi mic headset)"
	if got := keepAwakeLine(st, d); got != off {
		t.Fatalf("keepAwakeLine() with no drop-in = %q, want %q", got, off)
	}
	path := mic.NewHeadset(d, es.BluetoothMic).DropinPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(es.BluetoothMic.DropinMarker+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := keepAwakeLine(st, d); got != "Bluetooth keep-awake: on" {
		t.Fatalf("keepAwakeLine() with voxi's drop-in = %q", got)
	}
}

// Negatives: the line is empty for a built-in default, and a foreign drop-in
// is reported as left alone.
func TestKeepAwakeLineNegatives(t *testing.T) {
	d := keepAwakeTestDeps(t)
	es, err := spec.LoadEager()
	if err != nil {
		t.Fatal(err)
	}
	if got := keepAwakeLine(parsedState(t, builtinDefaultDump), d); got != "" {
		t.Fatalf("keepAwakeLine() for a built-in default = %q, want empty", got)
	}
	path := mic.NewHeadset(d, es.BluetoothMic).DropinPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("monitor.bluez.rules = []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := keepAwakeLine(parsedState(t, btLoopbackDefaultDump), d)
	if !strings.Contains(got, "not voxi's drop-in") {
		t.Fatalf("keepAwakeLine() with a foreign drop-in = %q", got)
	}
}
