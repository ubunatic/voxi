package eager

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/deps"
)

func TestAcceptTranscriptRejectsIsolatedSilenceArtifactBeforeTypingAndHistory(t *testing.T) {
	artifacts := []string{"bye"}
	if acceptTranscript(nil, "Bye!", nil, artifacts) {
		t.Fatal("isolated artifact reached the eager typing/history gate")
	}
	for _, text := range []string{"goodbye", "hello bye", "bye for now", "say goodbye"} {
		if !acceptTranscript(nil, text, nil, artifacts) {
			t.Errorf("longer genuine transcript %q did not reach eager typing/history gate", text)
		}
	}
}

func TestVoxtypeTranscribeArgsPreserveDisabledBehavior(t *testing.T) {
	got := voxtypeTranscribeArgs("small.en", "/tmp/one.wav", "")
	want := []string{"--model", "small.en", "--threads", "6", "-q", "transcribe", "/tmp/one.wav"}
	if !slices.Equal(got, want) {
		t.Fatalf("voxtypeTranscribeArgs() = %#v, want %#v", got, want)
	}
}

func TestVoxtypeTranscribeArgsPutInitialPromptBeforeSubcommand(t *testing.T) {
	got := voxtypeTranscribeArgs("small.en", "/tmp/one.wav", "Terms: Voxi")
	want := []string{"--model", "small.en", "--threads", "6", "--initial-prompt", "Terms: Voxi", "-q", "transcribe", "/tmp/one.wav"}
	if !slices.Equal(got, want) {
		t.Fatalf("voxtypeTranscribeArgs() = %#v, want %#v", got, want)
	}
}

func TestSpeechContextIsSmallEnOnlyAndDisableable(t *testing.T) {
	for _, test := range []struct {
		model   string
		enabled bool
		want    bool
	}{
		// enabled=false models an explicit --speech-context=false: prompting
		// must be fully disabled regardless of model (issue 046).
		{model: "small.en", enabled: false, want: false},
		{model: "small.en", enabled: true, want: true},
		{model: "base.en", enabled: true, want: false},
		{model: "large-v3-turbo", enabled: true, want: false},
	} {
		if got := shouldUseSpeechContext(test.model, test.enabled); got != test.want {
			t.Errorf("shouldUseSpeechContext(%q, %t) = %t, want %t", test.model, test.enabled, got, test.want)
		}
	}
}

// TestDefaultEagerOptionsEnableSpeechContext locks in issue 046's flipped
// default: a fresh `voxi eager` invocation with no --speech-context flag
// must have prompting active (small.en is also the default model, so the
// shouldUseSpeechContext gate above will fire). Explicit
// --speech-context=false still overrides this via the Cobra flag binding
// in cmd/voxi/main.go, which is exercised by TestSpeechContextIsSmallEnOnlyAndDisableable
// above (the enabled=false cases).
func TestDefaultEagerOptionsEnableSpeechContext(t *testing.T) {
	opts := DefaultEagerOptions()
	if !opts.SpeechContext {
		t.Fatal("DefaultEagerOptions().SpeechContext = false, want true (issue 046: default-on)")
	}
	if !shouldUseSpeechContext(opts.Model, opts.SpeechContext) {
		t.Fatalf("shouldUseSpeechContext(%q, true) = false, want true for the default model", opts.Model)
	}
}

func TestRejectionReason(t *testing.T) {
	artifacts := []string{"bye"}
	stopWords := []string{"thank you"}

	if r := rejectionReason(fmt.Errorf("fail"), "", "", nil, nil); !strings.HasPrefix(r, "transcribe_error") {
		t.Errorf("expected transcribe_error, got %q", r)
	}
	if r := rejectionReason(nil, "", "", nil, nil); r != "empty" {
		t.Errorf("expected empty, got %q", r)
	}
	if r := rejectionReason(nil, "bye.", "bye", nil, artifacts); r != "silence_artifact" {
		t.Errorf("expected silence_artifact, got %q", r)
	}
	if r := rejectionReason(nil, "Thank you.", "Thank you.", stopWords, nil); r != "stop_word" {
		t.Errorf("expected stop_word, got %q", r)
	}
	if r := rejectionReason(nil, "Hello world", "Hello world", stopWords, artifacts); r != "" {
		t.Errorf("expected empty reason for valid text, got %q", r)
	}
}

// fakeCaptureRun returns an eagerSessionManager run function that stands in
// for runEagerCaptureSession: it blocks until the session context is
// canceled (like real audio capture blocked on io.ReadFull), then calls
// onCaptureStopped (like the real reap-and-signal step added for issue 057),
// then sleeps for drain to simulate a still-in-flight `voxtype transcribe`
// subprocess draining in the background before the session fully returns.
// startedCount, if non-nil, is incremented once per invocation so a test can
// assert exactly how many sessions actually ran.
func fakeCaptureRun(drain time.Duration, startedCount *atomic.Int32) func(context.Context, func()) {
	return func(sessCtx context.Context, onCaptureStopped func()) {
		if startedCount != nil {
			startedCount.Add(1)
		}
		<-sessCtx.Done()
		onCaptureStopped()
		time.Sleep(drain)
	}
}

// TestEagerSessionManagerStopDoesNotBlockOnTranscriptionDrain is the
// issue 057 regression test: Stop must return as soon as audio capture has
// stopped, not block for the session's full lifetime including a slow
// transcription drain. Before the fix, stopCurrent's sessWg.Wait() blocked
// on exactly this drain, which is what let a stop/start/toggle request
// stall the eager-daemon socket (and, transitively, the agent control-plane
// mutex in internal/agent/agent.go) for up to transcribeTimeout.
func TestEagerSessionManagerStopDoesNotBlockOnTranscriptionDrain(t *testing.T) {
	const drain = 300 * time.Millisecond
	mgr := newEagerSessionManager(context.Background(), fakeCaptureRun(drain, nil))

	mgr.Start()
	// Give the session goroutine a moment to actually start capturing before
	// we stop it, so this exercises a real active session rather than racing
	// mgr.Start's own internal bookkeeping.
	time.Sleep(20 * time.Millisecond)

	stopStart := time.Now()
	mgr.Stop()
	stopElapsed := time.Since(stopStart)

	if stopElapsed >= drain/2 {
		t.Fatalf("Stop() took %v, want well under the %v transcription drain -- "+
			"it must return once capture stops, not once the session's background drain finishes", stopElapsed, drain)
	}
	if mgr.Recording() {
		t.Fatal("Recording() reports true immediately after Stop() returned")
	}

	// The drain must still be happening in the background: Wait (used only
	// at daemon shutdown) should block for roughly the full drain duration.
	waitStart := time.Now()
	mgr.Wait()
	waitElapsed := time.Since(waitStart)
	if waitElapsed < drain/2 {
		t.Fatalf("Wait() returned after %v, want it to block for the in-flight drain (~%v) -- "+
			"a Wait() that returns instantly would let a daemon exit before flushing a queued utterance", waitElapsed, drain)
	}
}

// TestEagerSessionManagerRapidToggleDuringDrainStartsFreshSession
// reproduces the ticket's rapid-fire toggle/stop/start-during-transcription
// scenario end to end: while a previous session's capture has stopped but
// its transcription is still draining in the background, a new Start/Toggle
// must be able to begin a brand new session immediately, without waiting for
// the stale drain, and the two sessions' run invocations must not deadlock
// or leave the manager stuck reporting a stale state. Run with -race to
// catch the mutex-scoping mistakes this class of bug tends to hide.
func TestEagerSessionManagerRapidToggleDuringDrainStartsFreshSession(t *testing.T) {
	const drain = 300 * time.Millisecond
	var started atomic.Int32
	mgr := newEagerSessionManager(context.Background(), fakeCaptureRun(drain, &started))

	done := make(chan struct{})
	go func() {
		defer close(done)
		mgr.Toggle() // start
		time.Sleep(10 * time.Millisecond)
		mgr.Toggle() // stop (returns fast; drain #1 continues in background)
		mgr.Toggle() // start again -- must not block on drain #1
		mgr.Toggle() // stop again (returns fast; drain #2 continues)
		mgr.Toggle() // start a third session
	}()

	select {
	case <-done:
	case <-time.After(drain):
		t.Fatalf("rapid toggle sequence did not complete within one drain duration (%v) -- "+
			"a stop/start is blocking on a stale transcription drain", drain)
	}

	if !mgr.Recording() {
		t.Fatal("expected a recording session active after an odd number of toggles")
	}

	mgr.Stop()
	mgr.Wait()

	if got := started.Load(); got != 3 {
		t.Fatalf("fake capture run invoked %d times, want exactly 3 sessions started", got)
	}
}

// TestRunEagerCaptureSessionSignalsCaptureStoppedOnEarlyReturn is the
// issue 057 regression test for a second, narrower bug found while live-
// verifying the eagerSessionManager fix above: exec.Command's Start()
// returns ctx.Err() immediately, without ever spawning a process, when its
// context is already canceled by the time Start() is called -- which
// routinely happens here, since a session can be told to stop before its
// capture goroutine has gotten past setup. An earlier version of this fix
// only called onCaptureStopped from the single "happy path" reap step after
// the capture loop, so this early `return fmt.Errorf("start audio
// capture"...)` skipped it entirely, permanently deadlocking
// eagerSessionManager.Stop() (confirmed live via a SIGQUIT goroutine dump:
// Stop() parked forever on `<-stopped`, and the corresponding capture
// goroutine had already exited without ever signaling it). The fix wraps
// onCaptureStopped in a deferred, sync.Once-guarded call so every return
// path -- not just the happy one -- signals it exactly once.
func TestRunEagerCaptureSessionSignalsCaptureStoppedOnEarlyReturn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled before runEagerCaptureSession (and thus
	// recCmd.Start()) ever runs, reproducing the race.

	d := deps.Dependencies{
		Getenv: func(string) string { return "" },
		Stdout: io.Discard,
	}
	opts := EagerOptions{
		Model:         "small.en",
		SpeechContext: false, // skip repository/vocabulary scanning, irrelevant here
	}

	var stoppedCount atomic.Int32
	done := make(chan error, 1)
	go func() {
		done <- runEagerCaptureSession(ctx, d, opts, t.TempDir(), "voxtype", "sleep", []string{"5"}, true, func() {
			stoppedCount.Add(1)
		})
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("runEagerCaptureSession with an already-canceled context returned nil error, want a start-audio-capture failure")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runEagerCaptureSession did not return promptly for an already-canceled context")
	}

	if got := stoppedCount.Load(); got != 1 {
		t.Fatalf("onCaptureStopped invoked %d times on the early-return path, want exactly 1 -- "+
			"a caller blocked in eagerSessionManager.Stop() waiting on this signal would hang forever", got)
	}
}
