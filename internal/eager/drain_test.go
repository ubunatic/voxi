package eager

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/chunks"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/telemetry"
)

// Issue 115 regression tests. Both live incidents lost fully transcribed,
// accepted speech because the post-stop drain was gated on a five-second wall
// clock: chunks #563/#564 on the modifier-buffered flush path, and chunk #1120
// (issue 120) on the plain path. The replacement gate is "a newer session
// started", so the scenarios below hold transcription hostage far past the
// removed lease and assert the text still lands.

// removedLeaseMargin is deliberately longer than the deleted five-second
// stopDrainTimeout: a shorter hold would pass against the buggy code too.
const removedLeaseMargin = 5400 * time.Millisecond

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func speechFrame() []byte {
	frame := make([]byte, 640)
	for i := 0; i < len(frame); i += 2 {
		binary.LittleEndian.PutUint16(frame[i:i+2], 1000)
	}
	return frame
}

// speechPCM builds segments utterances, each followed by enough silence for
// the segmenter to finalize it while the audio source stays open.
func speechPCM(segments int) []byte {
	frame := speechFrame()
	silence := make([]byte, 640)
	var pcm []byte
	for range segments {
		pcm = append(pcm, bytes.Repeat(frame, 12)...)
		pcm = append(pcm, bytes.Repeat(silence, 5)...)
	}
	return pcm
}

// heldTranscriber writes a fake voxtype that numbers its invocations in
// countPath and blocks until tmp/release-N appears, so a test can hold any
// individual job hostage for as long as it likes.
func heldTranscriber(t *testing.T, tmp string) (path, countPath string) {
	t.Helper()
	countPath = filepath.Join(tmp, "count")
	path = filepath.Join(tmp, "fake-voxtype")
	script := "#!/bin/sh\n" +
		"count=0\n" +
		"if test -f '" + countPath + "'\n" +
		"then count=$(cat '" + countPath + "')\n" +
		"fi\n" +
		"count=$((count + 1))\n" +
		"printf '%s' \"$count\" > '" + countPath + "'\n" +
		"while ! test -e '" + tmp + "/release-'\"$count\"\n" +
		"do sleep 0.01\n" +
		"done\n" +
		"case \"$count\" in 1) echo first ;; 2) echo second ;; 3) echo third ;; esac\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return path, countPath
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func transcriberInvocations(countPath string) func(int) bool {
	return func(n int) bool {
		data, err := os.ReadFile(countPath)
		return err == nil && string(data) == fmt.Sprint(n)
	}
}

func release(t *testing.T, tmp string, n int) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(tmp, fmt.Sprintf("release-%d", n)), nil, 0600); err != nil {
		t.Fatal(err)
	}
}

func drainTestDeps(tmp, transcriber string, stdout io.Writer, typed *[]string, mu *sync.Mutex) deps.Dependencies {
	return deps.Dependencies{
		Getenv: func(key string) string {
			if key == "HOME" || key == "XDG_RUNTIME_DIR" {
				return tmp
			}
			return ""
		},
		LookPath: func(name string) (string, error) {
			if name == "voxtype" {
				return transcriber, nil
			}
			return name, nil
		},
		RunStdin: func(_ context.Context, stdin, _ string, _ ...string) error {
			mu.Lock()
			defer mu.Unlock()
			*typed = append(*typed, stdin)
			return nil
		},
		Stdout: stdout,
	}
}

func drainTestOptions() EagerOptions {
	return EagerOptions{ThresholdRMS: 500, SilenceMs: 60, PreRollMs: 40, MinSpeechMs: 40, MaxWindowMs: 1000, TypeOutput: true, Model: "small.en"}
}

type drainScenario struct {
	tmp    string
	typed  []string
	events []telemetry.Event
	stdout string
}

// runStopDrainScenario captures two utterances, stops recording while the
// first is still inside the fake ASR, waits postStopDelay, and only then lets
// both jobs finish. No new session is ever started, so every chunk must be
// typed. buffered arms the issue 101 modifier buffer (a still-held Super), so
// delivery happens on the post-drain flush path instead of the plain one.
func runStopDrainScenario(t *testing.T, buffered bool, postStopDelay time.Duration) drainScenario {
	t.Helper()
	tmp := t.TempDir()
	fifoPath := filepath.Join(tmp, "audio.fifo")
	if err := syscall.Mkfifo(fifoPath, 0600); err != nil {
		t.Fatal(err)
	}
	transcriber, countPath := heldTranscriber(t, tmp)
	started := transcriberInvocations(countPath)

	written := make(chan struct{})
	go func() {
		f, err := os.OpenFile(fifoPath, os.O_WRONLY, 0)
		if err != nil {
			return
		}
		defer f.Close()
		if _, err := f.Write(speechPCM(2)); err != nil {
			return
		}
		close(written)
		<-t.Context().Done()
	}()

	var mu sync.Mutex
	var typed []string
	stdout := &lockedBuffer{}
	telemetryPath := filepath.Join(tmp, "telemetry.jsonl")
	recorder := telemetry.NewRecorder(telemetryPath)
	d := drainTestDeps(tmp, transcriber, stdout, &typed, &mu)
	opts := drainTestOptions()

	done := make(chan error, 1)
	var mgr *eagerSessionManager
	mgr = newEagerSessionManager(context.Background(), recorder, func(sessCtx context.Context, sessionID string, request *stopRequest, onCaptureStopped func()) {
		done <- runEagerCaptureSessionAt(sessCtx, d, opts, tmp, "cat", []string{fifoPath}, true, sessionID, request, recorder, onCaptureStopped, mgr)
	})
	mgr.Start()
	select {
	case <-written:
	case <-time.After(5 * time.Second):
		t.Fatal("audio writer did not finish")
	}
	waitFor(t, "the first transcription to start", func() bool { return started(1) })
	mgr.Stop()

	// The live failure: the trailing chunks finish transcription (and LLM
	// cleanup) well after the stop request, with no newer session in sight.
	time.Sleep(postStopDelay)
	if buffered {
		// A Super still physically held after Super+X: the daemon's poller
		// keeps reporting it, which is what armed the buffer in the original
		// incident even though Stop had already reset the press state.
		mgr.NoteModifierPress(time.Now())
	}
	release(t, tmp, 1)
	waitFor(t, "the second transcription to start", func() bool { return started(2) })
	release(t, tmp, 2)

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("session drain did not finish")
	}
	mgr.Wait()

	events, err := telemetry.ReadAll(telemetryPath)
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	return drainScenario{tmp: tmp, typed: slices.Clone(typed), events: events, stdout: stdout.String()}
}

func staleEvents(events []telemetry.Event) []telemetry.Event {
	var stale []telemetry.Event
	for _, e := range events {
		if e.Event == telemetry.DeliveryStale {
			stale = append(stale, e)
		}
	}
	return stale
}

func eventsOfType(events []telemetry.Event, name string) []telemetry.Event {
	var found []telemetry.Event
	for _, e := range events {
		if e.Event == name {
			found = append(found, e)
		}
	}
	return found
}

// assertFlushPathUsed proves the modifier buffer really engaged rather than
// the test silently falling back to the plain path: buffered deliveries are
// typed only after the session's whole transcription queue has drained, so no
// typing may start before the last transcription completes.
func assertFlushPathUsed(t *testing.T, events []telemetry.Event) {
	t.Helper()
	firstTyping, lastTranscription := -1, -1
	for i, e := range events {
		switch e.Event {
		case telemetry.TypingStarted:
			if firstTyping < 0 {
				firstTyping = i
			}
		case telemetry.TranscriptionComplete:
			lastTranscription = i
		}
	}
	if firstTyping < 0 || lastTranscription < 0 {
		t.Fatalf("expected both transcription and typing events, got typing=%d transcription=%d", firstTyping, lastTranscription)
	}
	if firstTyping < lastTranscription {
		t.Fatal("typing began before the queue drained: the modifier buffer never engaged, so this exercised the plain path")
	}
}

// EC1: the modifier-buffered flush path, the chunk #563/#564 incident. The
// buffered deliveries were discarded because the five-second lease had expired
// five milliseconds before FlushDeliveries ran.
func TestBufferedFlushSurvivesDrainDeadlineWhenNoNewSessionStarts(t *testing.T) {
	t.Parallel()
	got := runStopDrainScenario(t, true, removedLeaseMargin)
	if want := []string{"type first \n", "type second \n"}; !slices.Equal(got.typed, want) {
		t.Fatalf("typed = %#v, want both buffered chunks in order %#v", got.typed, want)
	}
	if stale := staleEvents(got.events); len(stale) != 0 {
		t.Fatalf("recorded %d delivery_stale events for valid speech: %+v", len(stale), stale)
	}
	assertFlushPathUsed(t, got.events)
}

// EC1b: the plain, non-buffered path, the chunk #1120 incident (issue 120).
// Same root cause, no modifier gating involved -- just queue backlog at stop.
func TestPlainDeliverySurvivesSlowQueueBacklogAfterStop(t *testing.T) {
	t.Parallel()
	got := runStopDrainScenario(t, false, removedLeaseMargin)
	if want := []string{"type first \n", "type second \n"}; !slices.Equal(got.typed, want) {
		t.Fatalf("typed = %#v, want both queued chunks in order %#v", got.typed, want)
	}
	if stale := staleEvents(got.events); len(stale) != 0 {
		t.Fatalf("recorded %d delivery_stale events for valid speech: %+v", len(stale), stale)
	}
}

// EC5: a buffered flush must leave the same metadata trail the plain path
// does, so `voxi chunks show N` does not report a delivered chunk as
// never-typed (ticket section 3.4).
func TestBufferedFlushPersistsTypingTimestamps(t *testing.T) {
	got := runStopDrainScenario(t, true, 0)
	if len(got.typed) != 2 {
		t.Fatalf("typed = %#v, want two buffered chunks", got.typed)
	}
	assertFlushPathUsed(t, got.events)
	items, err := chunks.NewBuffer(chunks.StorageDir(got.tmp, got.tmp), chunks.DefaultBufferSize).List(false)
	if err != nil {
		t.Fatal(err)
	}
	accepted := 0
	for _, c := range items {
		if !c.Accepted {
			continue
		}
		accepted++
		if c.TypingStartedAt.IsZero() || c.TypingEndedAt.IsZero() {
			t.Errorf("chunk %s: typing timestamps = (%v, %v), want real times for a flushed delivery", c.ChunkID, c.TypingStartedAt, c.TypingEndedAt)
		}
		if c.TypingEndedAt.Before(c.TypingStartedAt) {
			t.Errorf("chunk %s: typing ended before it started", c.ChunkID)
		}
	}
	if accepted != 2 {
		t.Fatalf("accepted chunks = %d, want 2", accepted)
	}
	if n := len(eventsOfType(got.events, telemetry.TypingStarted)); n != 2 {
		t.Errorf("typing_started events = %d, want 2 -- the flush path recorded no typing telemetry", n)
	}
	if n := len(eventsOfType(got.events, telemetry.TypingComplete)); n != 2 {
		t.Errorf("typing_completed events = %d, want 2", n)
	}
}

// EC2: the one case the removed lease was actually protecting against. A
// superseded generation must stay silent, say so, and lose to the new session
// -- on the flush path, where the original incident happened.
func TestSupersededGenerationDoesNotTypeIntoNewSession(t *testing.T) {
	tmp := t.TempDir()
	fifoPath := filepath.Join(tmp, "audio.fifo")
	if err := syscall.Mkfifo(fifoPath, 0600); err != nil {
		t.Fatal(err)
	}
	rawPath := filepath.Join(tmp, "session-b.raw")
	if err := os.WriteFile(rawPath, speechPCM(1), 0600); err != nil {
		t.Fatal(err)
	}
	transcriber, countPath := heldTranscriber(t, tmp)
	started := transcriberInvocations(countPath)

	written := make(chan struct{})
	go func() {
		f, err := os.OpenFile(fifoPath, os.O_WRONLY, 0)
		if err != nil {
			return
		}
		defer f.Close()
		if _, err := f.Write(speechPCM(2)); err != nil {
			return
		}
		close(written)
		<-t.Context().Done()
	}()

	var mu sync.Mutex
	var typed []string
	stdout := &lockedBuffer{}
	telemetryPath := filepath.Join(tmp, "telemetry.jsonl")
	recorder := telemetry.NewRecorder(telemetryPath)
	d := drainTestDeps(tmp, transcriber, stdout, &typed, &mu)
	opts := drainTestOptions()

	done := make(chan error, 2)
	var sessions sync.Map // sessionID -> generation order
	var order int
	var mgr *eagerSessionManager
	mgr = newEagerSessionManager(context.Background(), recorder, func(sessCtx context.Context, sessionID string, request *stopRequest, onCaptureStopped func()) {
		mu.Lock()
		order++
		source := fifoPath
		if order == 2 {
			// Session B reads a finite file, so it ends on its own.
			source = rawPath
		}
		sessions.Store(sessionID, order)
		mu.Unlock()
		done <- runEagerCaptureSessionAt(sessCtx, d, opts, tmp, "cat", []string{source}, true, sessionID, request, recorder, onCaptureStopped, mgr)
	})

	// Session A: two utterances, stopped while the first is still in ASR.
	mgr.Start()
	select {
	case <-written:
	case <-time.After(5 * time.Second):
		t.Fatal("audio writer did not finish")
	}
	waitFor(t, "session A transcription to start", func() bool { return started(1) })
	mgr.Stop()
	// A still-held Super arms the issue 101 buffer, so A's first chunk takes
	// the flush path that lost chunks #563/#564 instead of typing directly.
	mgr.NoteModifierPress(time.Now())
	release(t, tmp, 1)
	// A's second chunk entering ASR proves the first is transcribed and
	// buffered, and parks A short of its flush.
	waitFor(t, "session A to buffer its first chunk", func() bool { return started(2) })

	// Session B starts before A has drained -- the supersede signal.
	mgr.Start()
	waitFor(t, "session B transcription to start", func() bool { return started(3) })
	release(t, tmp, 2)
	release(t, tmp, 3)

	for range 2 {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("sessions did not finish")
		}
	}
	mgr.Wait()

	mu.Lock()
	gotTyped := slices.Clone(typed)
	mu.Unlock()
	if want := []string{"type third \n"}; !slices.Equal(gotTyped, want) {
		t.Fatalf("typed = %#v, want only the new session's text %#v", gotTyped, want)
	}

	events, err := telemetry.ReadAll(telemetryPath)
	if err != nil {
		t.Fatal(err)
	}
	stale := staleEvents(events)
	if len(stale) != 1 {
		t.Fatalf("delivery_stale events = %d, want exactly one for the superseded generation: %+v", len(stale), stale)
	}
	if stale[0].CancelReason != dropSuperseded {
		t.Errorf("drop reason = %q, want %q", stale[0].CancelReason, dropSuperseded)
	}
	if generation, ok := sessions.Load(stale[0].SessionID); !ok || generation != 1 {
		t.Errorf("dropped chunk belonged to session %q (generation %v), want the superseded first generation", stale[0].SessionID, generation)
	}
	// Never drop silently: the daemon's stdout is the journal.
	out := stdout.String()
	for _, want := range []string{"transcript not typed", "reason=" + dropSuperseded} {
		if !strings.Contains(out, want) {
			t.Errorf("daemon output does not mention %q; got:\n%s", want, out)
		}
	}
	assertRecoveryHintIsUsable(t, out)
	if strings.Contains(out, "first") {
		t.Errorf("drop message leaked the transcript text, violating the privacy contract:\n%s", out)
	}
}

// assertRecoveryHintIsUsable checks the advice a user gets at the worst
// possible moment actually works. `voxi history retype` resolves an entry by
// its 8-character ID via an exact-match lookup (history.FindHistoryEntry), so
// a hint naming a position -- `retype 1` -- fails with `no history entry with
// id "1"`. Asserting only that the string "voxi history retype" appears is
// what let exactly that ship.
func assertRecoveryHintIsUsable(t *testing.T, out string) {
	t.Helper()
	const hint = "voxi history list, then voxi history retype <ID>"
	if !strings.Contains(out, hint) {
		t.Errorf("drop message does not offer the recovery hint %q; got:\n%s", hint, out)
	}
	// A literal argument after `retype` would be a positional index or some
	// other invented token: the real ID is only knowable from `history list`.
	for _, bogus := range []string{"retype 1", "retype 0", "retype <N>", "retype N"} {
		if strings.Contains(out, bogus) {
			t.Errorf("drop message suggests %q, but retype takes an 8-character history ID, not an index; got:\n%s", bogus, out)
		}
	}
}

// EC4: an unvoiced trailing transient (breath, key click) must short-circuit
// before ASR, so it can never extend or block a stop drain.
func TestImplausibleTrailingChunkSkipsTranscriptionDuringDrain(t *testing.T) {
	tmp := t.TempDir()
	transcriber, countPath := heldTranscriber(t, tmp)

	silence := make([]byte, 640)
	spike := make([]byte, 640)
	for i := 0; i < len(spike); i += 2 {
		binary.LittleEndian.PutUint16(spike[i:i+2], 4000)
	}
	var pcm []byte
	pcm = append(pcm, bytes.Repeat(silence, 3)...)
	pcm = append(pcm, spike...)
	pcm = append(pcm, bytes.Repeat(silence, 8)...)
	rawPath := filepath.Join(tmp, "transient.raw")
	if err := os.WriteFile(rawPath, pcm, 0600); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var typed []string
	d := drainTestDeps(tmp, transcriber, io.Discard, &typed, &mu)
	opts := drainTestOptions()

	done := make(chan error, 1)
	go func() {
		done <- runEagerCaptureSession(context.Background(), d, opts, tmp, "cat", []string{rawPath}, true, "transient-session", nil, nil, nil)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		// The fake ASR blocks forever without a release file, so a timeout
		// here means the transient reached transcription.
		t.Fatal("session did not return promptly on an implausible trailing chunk")
	}

	if _, err := os.Stat(countPath); !os.IsNotExist(err) {
		t.Fatalf("transcriber was invoked for an implausible transient (count file stat err = %v)", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(typed) != 0 {
		t.Fatalf("typed %#v for an implausible transient, want nothing", typed)
	}
}
