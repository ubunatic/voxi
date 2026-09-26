package tts

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestManagerPrefetchesAndControlsQueue(t *testing.T) {
	backend := &fakeBackend{started: make(chan string, 8), players: make(chan *fakePlayback, 8)}
	manager := NewManager(context.Background(), backend)
	defer manager.Close()

	count, err := manager.Enqueue("First sentence. Second sentence.")
	if err != nil || count != 2 {
		t.Fatalf("Enqueue() = %d, %v; want 2 chunks", count, err)
	}
	first := waitForPlayer(t, backend)
	waitFor(t, func() bool {
		return manager.Snapshot().Current == "First sentence." && manager.Snapshot().Status == statusPlaying
	})
	if rows := manager.Snapshot().History; len(rows) != 1 || rows[0].Text != "First sentence." || rows[0].Status != statusPlaying {
		t.Fatalf("feed history while playing = %+v", rows)
	}
	select {
	case got := <-backend.started:
		if got != "First sentence." {
			t.Fatalf("current chunk synthesis = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("first chunk was not synthesized")
	}
	select {
	case got := <-backend.started:
		if got != "Second sentence." {
			t.Fatalf("prefetched chunk = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("second chunk was not synthesized while the first was playing")
	}

	if err := manager.Control(ActionPlayPause); err != nil {
		t.Fatalf("pause: %v", err)
	}
	waitFor(t, func() bool { return manager.Snapshot().Status == statusPaused })
	if err := manager.Control(ActionPlayPause); err != nil {
		t.Fatalf("resume: %v", err)
	}
	waitFor(t, func() bool { return manager.Snapshot().Status == statusPlaying })
	if err := manager.Control(ActionNext); err != nil {
		t.Fatalf("next: %v", err)
	}
	second := waitForPlayer(t, backend)
	waitFor(t, func() bool { return manager.Snapshot().Current == "Second sentence." })
	if !first.stopped() {
		t.Fatal("next did not stop the first chunk")
	}
	if rows := manager.Snapshot().History; len(rows) != 2 || rows[0].Text != "First sentence." || rows[0].Status != "skipped" || rows[1].Text != "Second sentence." {
		t.Fatalf("feed history after next = %+v", rows)
	}
	if count, err := manager.Enqueue("Third sentence."); err != nil || count != 1 {
		t.Fatalf("Enqueue(third) = %d, %v", count, err)
	}
	waitFor(t, func() bool { return len(manager.Snapshot().Queue) == 1 })
	second.finish(nil)
	third := waitForPlayer(t, backend)
	waitFor(t, func() bool { return manager.Snapshot().Current == "Third sentence." })
	if err := manager.Control(ActionClear); err != nil {
		t.Fatalf("clear: %v", err)
	}
	waitFor(t, func() bool {
		return len(manager.Snapshot().Queue) == 0 && manager.Snapshot().Current == "Third sentence."
	})
	if err := manager.Control(ActionStop); err != nil {
		t.Fatalf("stop: %v", err)
	}
	waitFor(t, func() bool { return manager.Snapshot().Status == statusIdle && manager.Snapshot().Current == "" })
	if !third.stopped() {
		t.Fatal("stop did not stop the current player")
	}
	if got := manager.Snapshot().Queue; len(got) != 0 {
		t.Fatalf("queue after stop = %#v, want empty", got)
	}
	if rows := manager.Snapshot().History; len(rows) != 3 || rows[2].Status != "stopped" {
		t.Fatalf("feed history after stop = %+v", rows)
	}
}

func TestRetainFeedRowCapsOutputHistoryAtTen(t *testing.T) {
	var rows []ChunkRecord
	for i := 1; i <= 12; i++ {
		rows = retainFeedRow(rows, ChunkRecord{ID: uint64(i)})
	}
	if len(rows) != 10 || rows[0].ID != 3 || rows[9].ID != 12 {
		t.Fatalf("retained feed rows = %+v, want ids 3..12", rows)
	}
}

func TestManagerRejectsOversizedText(t *testing.T) {
	manager := NewManager(context.Background(), &fakeBackend{})
	defer manager.Close()
	if _, err := manager.Enqueue(strings.Repeat("x", maxTextBytes+1)); err == nil {
		t.Fatal("oversized text accepted")
	}
}

func TestRecordingEpochPermanentlyDiscardsQueuedPlayback(t *testing.T) {
	backend := &fakeBackend{started: make(chan string, 4), players: make(chan *fakePlayback, 4)}
	manager := NewManager(context.Background(), backend)
	defer manager.Close()
	if _, err := manager.Enqueue("current chunk. queued chunk."); err != nil {
		t.Fatal(err)
	}
	player := waitForPlayer(t, backend)
	waitFor(t, func() bool {
		snapshot := manager.Snapshot()
		return snapshot.Current == "current chunk." && len(snapshot.Queue) == 1
	})
	for _, want := range []string{"current chunk.", "queued chunk."} {
		select {
		case got := <-backend.started:
			if got != want {
				t.Fatalf("pre-recording synthesis = %q, want %q", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("%q was not synthesized before recording", want)
		}
	}
	if err := manager.Control(ActionRecordingStart); err != nil {
		t.Fatal(err)
	}
	if !player.stopped() {
		t.Fatal("recording start did not stop current playback")
	}
	waitFor(t, func() bool {
		snapshot := manager.Snapshot()
		return snapshot.Current == "" && len(snapshot.Queue) == 0
	})
	if err := manager.Control(ActionRecordingEnd); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-backend.started:
		t.Fatalf("synthesized discarded speech after recording ended: %q", got)
	case <-time.After(80 * time.Millisecond):
	}
}

func TestManagerReplaceStopsCurrentAndUsesOnlyNewQueue(t *testing.T) {
	backend := &fakeBackend{started: make(chan string, 8), players: make(chan *fakePlayback, 8)}
	manager := NewManager(context.Background(), backend)
	defer manager.Close()
	if _, err := manager.Enqueue("Old current. Old queued."); err != nil {
		t.Fatal(err)
	}
	player := waitForPlayer(t, backend)
	waitFor(t, func() bool { return len(manager.Snapshot().Queue) == 1 })
	count, err := manager.Replace("New current. New next.")
	if err != nil || count != 2 {
		t.Fatalf("Replace() = %d, %v", count, err)
	}
	if !player.stopped() {
		t.Fatal("replacement did not stop current playback")
	}
	newPlayer := waitForPlayer(t, backend)
	waitFor(t, func() bool { return manager.Snapshot().Current == "New current." && len(manager.Snapshot().Queue) == 1 })
	if got := manager.Snapshot().Queue[0]; got != "New next." {
		t.Fatalf("replacement queue = %q", got)
	}
	newPlayer.finish(nil)
}

func TestDisabledManagerReportsConfigOptOutAndRejectsSpeech(t *testing.T) {
	manager := NewManagerWithEnabled(context.Background(), &fakeBackend{}, false)
	defer manager.Close()
	if _, err := manager.Enqueue("must not play"); !errors.Is(err, ErrTTSDisabled) {
		t.Fatalf("Enqueue() error = %v, want disabled", err)
	}
	if got := manager.Snapshot().BackendStatus; got != "disabled by configuration" {
		t.Fatalf("BackendStatus = %q", got)
	}
}

func TestManagerCloseStopsPlaybackAndDiscardsQueue(t *testing.T) {
	backend := &fakeBackend{started: make(chan string, 4), players: make(chan *fakePlayback, 4)}
	manager := NewManager(context.Background(), backend)
	if _, err := manager.Enqueue("Current chunk. Queued chunk."); err != nil {
		t.Fatal(err)
	}
	player := waitForPlayer(t, backend)
	waitFor(t, func() bool { return len(manager.Snapshot().Queue) == 1 })
	manager.Close()
	if !player.stopped() {
		t.Fatal("manager close did not stop current playback")
	}
	snapshot := manager.Snapshot()
	if snapshot.Status != statusIdle || snapshot.Current != "" || len(snapshot.Queue) != 0 {
		t.Fatalf("snapshot after close = %+v", snapshot)
	}
	if _, err := manager.Enqueue("after close"); !errors.Is(err, ErrNoMonitor) {
		t.Fatalf("enqueue after close error = %v, want no monitor", err)
	}
}

type fakeBackend struct {
	mu      sync.Mutex
	started chan string
	players chan *fakePlayback
}

func (b *fakeBackend) Synthesize(ctx context.Context, text string) (audioFile, time.Duration, error) {
	select {
	case <-ctx.Done():
		return audioFile{}, 0, ctx.Err()
	default:
	}
	dir, err := os.MkdirTemp("", "voxi-tts-test-")
	if err != nil {
		return audioFile{}, 0, err
	}
	path := filepath.Join(dir, "audio.wav")
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		_ = os.RemoveAll(dir)
		return audioFile{}, 0, err
	}
	if b.started != nil {
		b.started <- text
	}
	return audioFile{path: path, dir: dir}, 5 * time.Millisecond, nil
}

func (b *fakeBackend) StartPlayback(_ context.Context, _ string) (Playback, error) {
	p := &fakePlayback{done: make(chan error, 1)}
	if b.players != nil {
		b.players <- p
	}
	return p, nil
}

type fakePlayback struct {
	mu         sync.Mutex
	done       chan error
	doneOnce   sync.Once
	paused     bool
	wasStopped bool
}

func (p *fakePlayback) Done() <-chan error { return p.done }

func (p *fakePlayback) Pause() error {
	p.mu.Lock()
	p.paused = true
	p.mu.Unlock()
	return nil
}

func (p *fakePlayback) Resume() error {
	p.mu.Lock()
	p.paused = false
	p.mu.Unlock()
	return nil
}

func (p *fakePlayback) Stop() error {
	p.mu.Lock()
	p.wasStopped = true
	p.mu.Unlock()
	p.finish(context.Canceled)
	return nil
}

func (p *fakePlayback) finish(err error) {
	p.doneOnce.Do(func() {
		p.done <- err
	})
}

func (p *fakePlayback) stopped() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.wasStopped
}

func waitForPlayer(t *testing.T, b *fakeBackend) *fakePlayback {
	t.Helper()
	select {
	case p := <-b.players:
		return p
	case <-time.After(2 * time.Second):
		t.Fatal("player did not start")
		return nil
	}
}

func waitFor(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition did not become true")
}
