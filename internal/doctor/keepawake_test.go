package doctor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
	hd := HostDoctor{Host: psHost(""), Spec: s, KeepAwake: func() string { return "Bluetooth keep-awake: off (enable with voxi mic headset)" }}
	res := hd.checkMic(context.Background(), check, writeDump(t, btLoopbackDefaultDump))
	if res.Status != OK {
		t.Fatalf("status = %v, want OK for a healthy loopback default: %+v", res.Status, res)
	}
	if want := "no microphone source problem; Bluetooth keep-awake: off (enable with voxi mic headset)"; res.Detail != want {
		t.Fatalf("detail = %q, want %q", res.Detail, want)
	}
}

// Negatives: a built-in default never shows the keep-awake line, and the
// line is skipped when no status func is wired.
func TestMicCheckKeepAwakeLineOnlyForBluetoothDefault(t *testing.T) {
	s, err := spec.LoadDoctor()
	if err != nil {
		t.Fatal(err)
	}
	check := checkFor(t, s, "mic")
	called := false
	hd := HostDoctor{Host: psHost(""), Spec: s, KeepAwake: func() string { called = true; return "x" }}
	res := hd.checkMic(context.Background(), check, writeDump(t, builtinDefaultDump))
	if res.Detail != "no microphone source problem" {
		t.Fatalf("built-in default detail = %q", res.Detail)
	}
	if called {
		t.Fatal("KeepAwake consulted for a built-in default")
	}

	bare := HostDoctor{Host: psHost(""), Spec: s}
	res = bare.checkMic(context.Background(), check, writeDump(t, btLoopbackDefaultDump))
	if res.Detail != "no microphone source problem" {
		t.Fatalf("nil KeepAwake detail = %q", res.Detail)
	}
}
