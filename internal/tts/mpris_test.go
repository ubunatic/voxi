package tts

import (
	"context"
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestMPRISRegistersMethodsAndRoutesNext(t *testing.T) {
	backend := &fakeBackend{started: make(chan string, 4), players: make(chan *fakePlayback, 4)}
	manager := NewManager(context.Background(), backend)
	defer manager.Close()
	if _, err := manager.Enqueue("First. Second."); err != nil {
		t.Fatal(err)
	}
	first := waitForPlayer(t, backend)
	defer first.finish(nil)

	service, err := StartMPRIS(context.Background(), manager)
	if err != nil {
		t.Skipf("session D-Bus is unavailable: %v", err)
	}
	defer service.Close()
	client, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	obj := client.Object(mprisName, mprisPath)
	var xml string
	if err := obj.Call("org.freedesktop.DBus.Introspectable.Introspect", 0).Store(&xml); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"PlayPause", "Previous", "Next", "Pause", "Play"} {
		if !strings.Contains(xml, `name="`+method+`"`) {
			t.Errorf("MPRIS introspection omits %s: %s", method, xml)
		}
	}
	if err := obj.Call(playerIF+".Pause", 0).Err; err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return manager.Snapshot().Status == statusPaused })
	if err := obj.Call(playerIF+".Play", 0).Err; err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return manager.Snapshot().Status == statusPlaying })
	if err := obj.Call(playerIF+".Next", 0).Err; err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return manager.Snapshot().Current == "Second." })
}
