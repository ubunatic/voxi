package eager

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/audio"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/telemetry"
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
// default: the SpeechContext *flag* defaults on, and still actually fires
// for small.en with no explicit --speech-context flag. It does NOT assert
// that the flag fires for whatever model is currently spec/models.yaml's
// default_model: since issue 074, default_model can be cohere-transcribe-03-2026,
// which has no vocabulary-biasing hook (shouldUseSpeechContext is deliberately
// gated to "small.en" only, per issue 066 §7.5) — so a default `voxi eager`
// invocation on a non-small.en default model has SpeechContext=true but
// shouldUseSpeechContext=false, and that is expected, not a bug. Explicit
// --speech-context=false still overrides everything via the Cobra flag
// binding in cmd/voxi/main.go, exercised by
// TestSpeechContextIsSmallEnOnlyAndDisableable above (the enabled=false cases).
func TestDefaultEagerOptionsEnableSpeechContext(t *testing.T) {
	opts := DefaultEagerOptions()
	if !opts.SpeechContext {
		t.Fatal("DefaultEagerOptions().SpeechContext = false, want true (issue 046: default-on)")
	}
	if !shouldUseSpeechContext("small.en", opts.SpeechContext) {
		t.Fatal("shouldUseSpeechContext(\"small.en\", true) = false, want true")
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

func TestProbableSilenceUsesInspectableVoicedRatio(t *testing.T) {
	if !probableSilence(audioStats(100, 4)) {
		t.Fatal("4% voiced chunk was not marked probable silence")
	}
	if probableSilence(audioStats(100, 5)) {
		t.Fatal("5% voiced chunk was marked probable silence")
	}
}

func audioStats(total, voiced int) audio.AudioStats {
	return audio.AudioStats{TotalFrames: total, VoicedFrames: voiced, VoicedRatio: float64(voiced) / float64(total)}
}

func TestCaptureTelemetryCorrelatesChunkThroughTyping(t *testing.T) {
	tmp := t.TempDir()
	rawPath := filepath.Join(tmp, "audio.raw")
	var pcm []byte
	appendFrame := func(amplitude int16) {
		frame := make([]byte, 640)
		for i := 0; i < len(frame); i += 2 {
			binary.LittleEndian.PutUint16(frame[i:i+2], uint16(amplitude))
		}
		pcm = append(pcm, frame...)
	}
	for range 2 {
		appendFrame(0)
	}
	for range 10 {
		appendFrame(1000)
	}
	for range 3 {
		appendFrame(0)
	}
	if err := os.WriteFile(rawPath, pcm, 0600); err != nil {
		t.Fatal(err)
	}
	voxtypePath := filepath.Join(tmp, "fake-voxtype")
	if err := os.WriteFile(voxtypePath, []byte("#!/bin/sh\nprintf 'hello telemetry world\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}

	telemetryPath := filepath.Join(tmp, "telemetry.jsonl")
	recorder := telemetry.NewRecorder(telemetryPath)
	d := deps.Dependencies{
		Getenv: func(key string) string {
			if key == "HOME" || key == "XDG_RUNTIME_DIR" {
				return tmp
			}
			return ""
		},
		LookPath: func(name string) (string, error) { return name, nil },
		RunStdin: func(context.Context, string, string, ...string) error { return nil },
		Stdout:   io.Discard,
	}
	opts := EagerOptions{ThresholdRMS: 500, SilenceMs: 60, PreRollMs: 40, MinSpeechMs: 40, MaxWindowMs: 1000, TypeOutput: true, Model: "small.en", SpeechContext: false}
	if err := runEagerCaptureSession(context.Background(), d, opts, tmp, voxtypePath, "cat", []string{rawPath}, true, "session-correlation", recorder, nil); err != nil {
		t.Fatalf("runEagerCaptureSession: %v", err)
	}

	events, err := telemetry.ReadAll(telemetryPath)
	if err != nil {
		t.Fatal(err)
	}
	wantStages := []string{telemetry.ChunkFinalized, telemetry.TranscriptionStarted, telemetry.TranscriptionComplete, telemetry.TypingStarted, telemetry.TypingComplete}
	var stages []telemetry.Event
	for _, event := range events {
		if slices.Contains(wantStages, event.Event) {
			stages = append(stages, event)
		}
	}
	if len(stages) != len(wantStages) {
		t.Fatalf("chunk lifecycle events = %v, want %v (all events: %+v)", eventNames(stages), wantStages, events)
	}
	for i, event := range stages {
		if event.Event != wantStages[i] {
			t.Fatalf("stage %d = %q, want %q", i, event.Event, wantStages[i])
		}
		if event.SessionID != "session-correlation" || event.ChunkID != "session-correlation/1" || event.ChunkIndex != 1 {
			t.Fatalf("stage %s lost correlation: %+v", event.Event, event)
		}
		if i > 0 && event.Timestamp.Before(stages[i-1].Timestamp) {
			t.Fatalf("stage %s timestamp precedes %s", event.Event, stages[i-1].Event)
		}
	}
	if stages[0].Audio == nil || stages[0].Audio.MeanRMS <= 0 || stages[0].Audio.PeakRMS != 1000 || stages[0].Audio.ProbableSilence {
		t.Fatalf("unexpected independent audio metrics: %+v", stages[0].Audio)
	}
	if stages[2].TranscriptWordCount == nil || *stages[2].TranscriptWordCount != 3 {
		t.Fatalf("transcript word count = %+v, want 3", stages[2].TranscriptWordCount)
	}
}

func eventNames(events []telemetry.Event) []string {
	names := make([]string, len(events))
	for i, event := range events {
		names[i] = event.Event
	}
	return names
}

// fakeCaptureRun returns an eagerSessionManager run function that stands in
// for runEagerCaptureSession: it blocks until the session context is
// canceled (like real audio capture blocked on io.ReadFull), then calls
// onCaptureStopped (like the real reap-and-signal step added for issue 057),
// then sleeps for drain to simulate a still-in-flight `voxtype transcribe`
// subprocess draining in the background before the session fully returns.
// startedCount, if non-nil, is incremented once per invocation so a test can
// assert exactly how many sessions actually ran.
func fakeCaptureRun(drain time.Duration, startedCount *atomic.Int32) func(context.Context, string, func()) {
	return func(sessCtx context.Context, _ string, onCaptureStopped func()) {
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
	mgr := newEagerSessionManager(context.Background(), nil, fakeCaptureRun(drain, nil))

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

func TestEagerSessionManagerRecordsExactMicControlBoundaries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	recorder := telemetry.NewRecorder(path)
	base := time.Date(2026, 9, 5, 10, 0, 0, 123, time.UTC)
	call := 0
	seenSession := make(chan string, 1)
	mgr := newEagerSessionManager(context.Background(), recorder, func(ctx context.Context, sessionID string, stopped func()) {
		seenSession <- sessionID
		<-ctx.Done()
		stopped()
	})
	mgr.now = func() time.Time {
		at := base.Add(time.Duration(call) * time.Nanosecond)
		call++
		return at
	}

	mgr.Start()
	sessionID := <-seenSession
	mgr.Stop()
	mgr.Wait()

	events, err := telemetry.ReadAll(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Event != telemetry.MicActivated || events[1].Event != telemetry.MicDeactivated {
		t.Fatalf("mic events = %+v, want activated then deactivated", events)
	}
	if events[0].SessionID != sessionID || events[1].SessionID != sessionID {
		t.Fatalf("mic event correlation mismatch: %+v", events)
	}
	if !events[0].Timestamp.Equal(base) || !events[1].Timestamp.Equal(base.Add(time.Nanosecond)) {
		t.Fatalf("control timestamps changed: %s, %s", events[0].Timestamp, events[1].Timestamp)
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
	mgr := newEagerSessionManager(context.Background(), nil, fakeCaptureRun(drain, &started))

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
		done <- runEagerCaptureSession(ctx, d, opts, t.TempDir(), "voxtype", "sleep", []string{"5"}, true, "test-session", nil, func() {
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
