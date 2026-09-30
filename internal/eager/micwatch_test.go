package eager

import (
	"context"
	"testing"
	"time"
)

// Issue 177: a session the mic check stopped must not type anything it captured.
func TestSessionDrainAbandonDropsDelivery(t *testing.T) {
	d := newSessionDrainWithTimeout(time.Minute)
	if ok, _ := d.deliverable(); !ok {
		t.Fatal("fresh drain must deliver")
	}
	d.abandon(dropMicRepaired)
	if ok, reason := d.deliverable(); ok || reason != dropMicRepaired {
		t.Fatalf("abandoned drain: ok=%v reason=%q", ok, reason)
	}
	if d.eligibleAt(time.Now().Add(time.Second)) {
		t.Fatal("frames after abandon must not be eligible")
	}
}

// A session ending itself must never stop a newer session.
func TestStopSessionOnlyStopsActive(t *testing.T) {
	m := newEagerSessionManager(context.Background(), nil, func(ctx context.Context, _ string, _ *stopRequest, stopped func()) {
		<-ctx.Done()
		stopped()
	})
	m.Start()
	m.mu.Lock()
	id := m.activeSessionID
	m.mu.Unlock()
	m.StopSession("some-older-session")
	if !m.Recording() {
		t.Fatal("StopSession with a stale id stopped the active session")
	}
	m.StopSession(id)
	if m.Recording() {
		t.Fatal("StopSession with the active id did not stop it")
	}
	m.Wait()
}
