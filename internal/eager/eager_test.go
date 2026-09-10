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
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/audio"
	"ubunatic.com/voxi/internal/chunks"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/feedback"
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

func TestApplyEngineReplacementsIsCohereOnlyAndChunkLocal(t *testing.T) {
	rules := []feedback.Replacement{{From: "Voxy", To: "voxi"}}
	if got := applyEngineReplacements(cohereTranscribeEngine, "Voxy one", rules); got != "voxi one" {
		t.Fatalf("Cohere result = %q", got)
	}
	if got := applyEngineReplacements("whisper", "Voxy one", rules); got != "Voxy one" {
		t.Fatalf("Whisper was changed: %q", got)
	}
	// Rules never span completed eager chunks or retroactively edit prior text.
	chunks := []string{"Voxy", "project and Voxy"}
	for i := range chunks {
		chunks[i] = applyEngineReplacements(cohereTranscribeEngine, chunks[i], []feedback.Replacement{{From: "Voxy project", To: "voxi project"}, {From: "Voxy", To: "voxi"}})
	}
	if got := strings.Join(chunks, " "); got != "voxi project and voxi" {
		t.Fatalf("chunk-local aggregate = %q", got)
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

func TestReportEagerFailureIsUserVisibleWithoutTranscriptText(t *testing.T) {
	var out bytes.Buffer
	d := deps.Dependencies{Stdout: &out}
	reportEagerFailure(d, "typing", "session-1", "session-1/2", fmt.Errorf("dotool not found on PATH"))
	want := "voxi eager: typing failed (session=session-1 chunk=session-1/2): dotool not found on PATH\n"
	if out.String() != want {
		t.Fatalf("diagnostic = %q, want %q", out.String(), want)
	}
	if strings.Contains(out.String(), "private transcript") {
		t.Fatal("diagnostic included transcript text")
	}
}

func TestReportEagerFailureIgnoresExpectedRejection(t *testing.T) {
	var out bytes.Buffer
	reportEagerFailure(deps.Dependencies{Stdout: &out}, "transcription", "session-1", "session-1/1", nil)
	if out.Len() != 0 {
		t.Fatalf("nil error produced diagnostic %q", out.String())
	}
}

func TestQueuedJobContextDetachesCanceledQueuedJobs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if got := queuedJobContext(ctx, false); got != ctx {
		t.Fatal("active queued job unexpectedly detached")
	}
	cancel()
	if got := queuedJobContext(ctx, false); got == ctx {
		t.Fatal("queued job after stop retained canceled session context")
	}
	if got := queuedJobContext(context.Background(), true); got == nil {
		t.Fatal("final job returned nil context")
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
		LookPath: func(name string) (string, error) {
			if name == "voxtype" {
				return voxtypePath, nil
			}
			return name, nil
		},
		RunStdin: func(context.Context, string, string, ...string) error { return nil },
		Stdout:   io.Discard,
	}
	opts := EagerOptions{ThresholdRMS: 500, SilenceMs: 60, PreRollMs: 40, MinSpeechMs: 40, MaxWindowMs: 1000, TypeOutput: true, Model: "small.en", SpeechContext: false}
	if err := runEagerCaptureSession(context.Background(), d, opts, tmp, "cat", []string{rawPath}, true, "session-correlation", recorder, nil, nil); err != nil {
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

func TestPathologicalTranscriptProducesZeroInjection(t *testing.T) {
	tmp := t.TempDir()
	rawPath := filepath.Join(tmp, "audio.raw")
	frame := make([]byte, 640)
	for i := 0; i < len(frame); i += 2 {
		binary.LittleEndian.PutUint16(frame[i:i+2], 1000)
	}
	pcm := append([]byte{}, frame...)
	for range 10 {
		pcm = append(pcm, frame...)
	}
	pcm = append(pcm, make([]byte, 640*4)...)
	if err := os.WriteFile(rawPath, pcm, 0600); err != nil {
		t.Fatal(err)
	}
	transcriber := filepath.Join(tmp, "fake-voxtype")
	bad := "Ubun" + strings.Repeat("tuk", 150)
	if err := os.WriteFile(transcriber, []byte("#!/bin/sh\nprintf '%s\\n' '"+bad+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	var injections atomic.Int32
	d := deps.Dependencies{
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
		RunStdin: func(context.Context, string, string, ...string) error { injections.Add(1); return nil },
		Stdout:   io.Discard,
	}
	opts := EagerOptions{ThresholdRMS: 500, SilenceMs: 60, PreRollMs: 40, MinSpeechMs: 40, MaxWindowMs: 1000, TypeOutput: true, Model: "small.en"}
	if err := runEagerCaptureSession(context.Background(), d, opts, tmp, "cat", []string{rawPath}, true, "pathological-session", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := injections.Load(); got != 0 {
		t.Fatalf("captured %d injector calls, want zero", got)
	}
	items, err := chunks.NewBuffer(chunks.StorageDir(tmp, tmp), chunks.DefaultBufferSize).List(false)
	if err != nil || len(items) != 1 {
		t.Fatalf("chunks: len=%d err=%v", len(items), err)
	}
	c := items[0]
	if c.Accepted || c.RejectionReason != "pathological_repetition" || c.RepeatUnit != "tuk" || c.RepeatCount != 150 {
		t.Fatalf("unexpected rejection metadata: %+v", c)
	}
	if c.RawTranscript != "" || c.CleanedTranscript != "" || c.TranscriptDigest == "" {
		t.Fatalf("pathological content was not privacy-safe: %+v", c)
	}
}

func TestStopCancelsInflightTranscriptionBeforeInjection(t *testing.T) {
	tmp := t.TempDir()
	rawPath := filepath.Join(tmp, "audio.raw")
	frame := make([]byte, 640)
	for i := 0; i < len(frame); i += 2 {
		binary.LittleEndian.PutUint16(frame[i:i+2], 1000)
	}
	pcm := bytes.Repeat(frame, 12)
	pcm = append(pcm, make([]byte, 640*4)...)
	if err := os.WriteFile(rawPath, pcm, 0600); err != nil {
		t.Fatal(err)
	}
	started := filepath.Join(tmp, "started")
	transcriber := filepath.Join(tmp, "fake-voxtype")
	script := "#!/bin/sh\n: > '" + started + "'\nexec sleep 30\n"
	if err := os.WriteFile(transcriber, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	var injections atomic.Int32
	d := deps.Dependencies{
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
		RunStdin: func(context.Context, string, string, ...string) error { injections.Add(1); return nil }, Stdout: io.Discard,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	opts := EagerOptions{ThresholdRMS: 500, SilenceMs: 60, PreRollMs: 40, MinSpeechMs: 40, MaxWindowMs: 1000, TypeOutput: true, Model: "small.en"}
	go func() {
		done <- runEagerCaptureSession(ctx, d, opts, tmp, "cat", []string{rawPath}, true, "stopped-session", nil, nil, nil)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("transcription did not start")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("session did not cancel promptly")
	}
	if got := injections.Load(); got != 0 {
		t.Fatalf("captured %d injector calls after stop", got)
	}
}

// TestStopFlushesTrailingUtteranceForTranscriptionAndTyping guards against a
// regression where stopping recording (Super-X in normal use) mid-utterance --
// after the user finished speaking but before the VAD's own silence timeout
// finalized it -- silently dropped that trailing speech. runEagerCaptureSession
// relies on segmenter.Flush() to recover exactly this buffered-but-not-yet-
// finalized audio once capture stops; the flushed job must still transcribe
// and type even though the session's own ctx is already canceled by then. See
// TestStopCancelsInflightTranscriptionBeforeInjection for the complementary
// guarantee: an utterance that was already mid-transcription (not merely
// buffered) when stop arrives must still abort and never type.
func TestStopFlushesTrailingUtteranceForTranscriptionAndTyping(t *testing.T) {
	tmp := t.TempDir()
	fifoPath := filepath.Join(tmp, "audio.fifo")
	if err := syscall.Mkfifo(fifoPath, 0600); err != nil {
		t.Fatal(err)
	}

	frame := make([]byte, 640)
	for i := 0; i < len(frame); i += 2 {
		binary.LittleEndian.PutUint16(frame[i:i+2], 1000)
	}
	written := make(chan struct{})
	go func() {
		f, err := os.OpenFile(fifoPath, os.O_WRONLY, 0)
		if err != nil {
			return
		}
		defer f.Close()
		// 10 frames (200ms) of continuous "speech" -- above both MinSpeechMs
		// (40ms) and the segmenter's default MinVoicedFrames (160ms) acoustic
		// plausibility floor -- but with no trailing silence, so the segmenter
		// never finalizes this utterance on its own; it stays buffered until
		// Flush() on stop.
		for range 10 {
			if _, err := f.Write(frame); err != nil {
				return
			}
		}
		close(written)
		// Keep the writer open (and cat blocked waiting for more input)
		// until the test tears down, so capture only ends via ctx cancel.
		<-t.Context().Done()
	}()

	transcriber := filepath.Join(tmp, "fake-voxtype")
	script := "#!/bin/sh\nprintf '%s\\n' 'this is chunk three'\n"
	if err := os.WriteFile(transcriber, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	var injections atomic.Int32
	var typed string
	d := deps.Dependencies{
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
			injections.Add(1)
			typed = stdin
			return nil
		},
		Stdout: io.Discard,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	opts := EagerOptions{ThresholdRMS: 500, SilenceMs: 60, PreRollMs: 40, MinSpeechMs: 40, MaxWindowMs: 1000, TypeOutput: true, Model: "small.en"}
	go func() {
		done <- runEagerCaptureSession(ctx, d, opts, tmp, "cat", []string{fifoPath}, true, "flushed-session", nil, nil, nil)
	}()

	select {
	case <-written:
	case <-time.After(2 * time.Second):
		t.Fatal("writer did not finish feeding the buffered utterance")
	}
	time.Sleep(50 * time.Millisecond) // let the capture loop actually read the frames
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("session did not shut down promptly")
	}

	if got := injections.Load(); got != 1 {
		t.Fatalf("captured %d injector calls, want exactly 1 for the flushed trailing utterance", got)
	}
	if !strings.Contains(typed, "this is chunk three") {
		t.Fatalf("typed content = %q, want it to contain the flushed transcript", typed)
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

func TestEagerSessionManagerConcurrentTogglesPreserveParity(t *testing.T) {
	var started atomic.Int32
	mgr := newEagerSessionManager(context.Background(), nil, fakeCaptureRun(0, &started))

	const rounds = 20
	for range rounds {
		var wg sync.WaitGroup
		wg.Add(2)
		for range 2 {
			go func() {
				defer wg.Done()
				mgr.Toggle()
			}()
		}
		wg.Wait()
		if mgr.Recording() {
			t.Fatal("two concurrent toggles from idle left a recording session active")
		}
	}

	mgr.Start()
	for range 2 {
		var wg sync.WaitGroup
		wg.Add(2)
		for range 2 {
			go func() {
				defer wg.Done()
				mgr.Toggle()
			}()
		}
		wg.Wait()
		if !mgr.Recording() {
			t.Fatal("two concurrent toggles from active left the manager idle")
		}
	}
	mgr.Stop()
	mgr.Wait()
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
		Getenv:   func(string) string { return "" },
		LookPath: func(name string) (string, error) { return name, nil },
		Stdout:   io.Discard,
	}
	opts := EagerOptions{
		Model:         "small.en",
		SpeechContext: false, // skip repository/vocabulary scanning, irrelevant here
	}

	var stoppedCount atomic.Int32
	done := make(chan error, 1)
	go func() {
		done <- runEagerCaptureSession(ctx, d, opts, t.TempDir(), "sleep", []string{"5"}, true, "test-session", nil, func() {
			stoppedCount.Add(1)
		}, nil)
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

// TestRequireEngineBinaryWhisperMissingVoxtypeGivesPreciseError is issue
// 077's acceptance criterion that an explicit Whisper selection without
// voxtype produces a precise, backend-specific error naming both the model
// and the missing executable -- not a generic PATH failure.
func TestRequireEngineBinaryWhisperMissingVoxtypeGivesPreciseError(t *testing.T) {
	d := deps.Dependencies{
		LookPath: func(name string) (string, error) {
			return "", fmt.Errorf("exec: %q: executable file not found in $PATH", name)
		},
	}
	_, _, err := requireEngineBinary(context.Background(), d, "small.en", "whisper")
	if err == nil {
		t.Fatal("requireEngineBinary() error = nil, want a missing-voxtype error")
	}
	for _, want := range []string{"small.en", "whisper", "voxtype"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("requireEngineBinary() error = %q, want it to mention %q", err, want)
		}
	}
}

// TestRequireEngineBinaryCohereMissingCrispASRGivesPreciseError mirrors the
// whisper case above for the cohere-transcribe engine: a missing crispasr
// binary must name crispasr and the model, and must never mention voxtype
// (which this engine never touches).
func TestRequireEngineBinaryCohereMissingCrispASRGivesPreciseError(t *testing.T) {
	d := deps.Dependencies{
		LookPath: func(name string) (string, error) {
			return "", fmt.Errorf("exec: %q: executable file not found in $PATH", name)
		},
	}
	_, _, err := requireEngineBinary(context.Background(), d, "cohere-transcribe-03-2026", cohereTranscribeEngine)
	if err == nil {
		t.Fatal("requireEngineBinary() error = nil, want a missing-crispasr error")
	}
	for _, want := range []string{"cohere-transcribe-03-2026", "crispasr"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("requireEngineBinary() error = %q, want it to mention %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "voxtype") {
		t.Errorf("requireEngineBinary() error = %q, must not mention voxtype for the cohere-transcribe engine", err)
	}
}

// TestRequireEngineBinaryCohereSucceedsWithoutVoxtype is the direct unit
// counterpart of issue 077's central acceptance criterion: given crispasr on
// PATH and cached weights, the cohere-transcribe engine resolves its binary
// successfully even though LookPath fails outright for voxtype.
func TestRequireEngineBinaryCohereSucceedsWithoutVoxtype(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))
	weightsPath, err := cohereWeightsPath()
	if err != nil {
		t.Fatalf("cohereWeightsPath(): %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(weightsPath), 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(weightsPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(cohereGGUFMinBytes); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	crispasrPath := filepath.Join(tmp, "fake-crispasr")
	d := deps.Dependencies{
		LookPath: func(name string) (string, error) {
			if name == crispasrBinary {
				return crispasrPath, nil
			}
			return "", fmt.Errorf("exec: %q: executable file not found in $PATH", name)
		},
	}
	binPath, gotWeights, err := requireEngineBinary(context.Background(), d, "cohere-transcribe-03-2026", cohereTranscribeEngine)
	if err != nil {
		t.Fatalf("requireEngineBinary() with voxtype absent = %v, want success", err)
	}
	if binPath != crispasrPath {
		t.Errorf("requireEngineBinary() binPath = %q, want %q", binPath, crispasrPath)
	}
	if gotWeights != weightsPath {
		t.Errorf("requireEngineBinary() weightsPath = %q, want %q", gotWeights, weightsPath)
	}
}

// TestRunEagerDaemonReachesReadyWithoutVoxtype is issue 077's live-readiness
// proof for the daemon path (mirrors TestRequireEngineBinaryCohereSucceedsWithoutVoxtype's
// direct-path proof): the actual production runEagerDaemon entry point --
// the same function voxi-agent.service's eager child process runs -- must
// bind its control socket and answer "status" once the default Cohere model
// is resolvable and crispasr+weights are available, even though LookPath
// fails outright for voxtype. Before this issue's fix, runEagerDaemon
// unconditionally required voxtype before ever reaching model/engine
// resolution, so this exact scenario crash-looped voxi-agent.service
// (confirmed live via `journalctl --user -u voxi-agent.service`).
func TestRunEagerDaemonReachesReadyWithoutVoxtype(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))
	weightsPath, err := cohereWeightsPath()
	if err != nil {
		t.Fatalf("cohereWeightsPath(): %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(weightsPath), 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(weightsPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(cohereGGUFMinBytes); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	runtimeDir := filepath.Join(tmp, "runtime")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatal(err)
	}
	crispasrPath := filepath.Join(tmp, "fake-crispasr")

	d := deps.Dependencies{
		Getenv: func(key string) string {
			if key == "XDG_RUNTIME_DIR" {
				return runtimeDir
			}
			if key == "HOME" {
				return tmp
			}
			return ""
		},
		LookPath: func(name string) (string, error) {
			switch name {
			case "voxtype":
				return "", fmt.Errorf("exec: %q: executable file not found in $PATH", name)
			case crispasrBinary:
				return crispasrPath, nil
			default:
				// pw-record/arecord audio-tool discovery: resolving the name
				// is enough since no capture session starts in this test.
				return name, nil
			}
		},
		Stdout: io.Discard,
	}
	opts := DefaultEagerOptions() // opts.Model is the spec default: cohere-transcribe-03-2026

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	daemonErr := make(chan error, 1)
	go func() { daemonErr <- runEagerDaemon(ctx, d, opts) }()

	deadline := time.Now().Add(5 * time.Second)
	var status string
	for time.Now().Before(deadline) {
		select {
		case err := <-daemonErr:
			t.Fatalf("runEagerDaemon exited early (want it still running/ready) with err=%v -- likely still unconditionally requiring voxtype", err)
		default:
		}
		status, err = GetEagerRecordingStatus(ctx, d)
		if err == nil && status == "idle" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if status != "idle" {
		t.Fatalf("eager daemon did not reach ready ('idle' over its control socket) within the deadline without voxtype on PATH; last status=%q err=%v", status, err)
	}

	cancel()
	select {
	case err := <-daemonErr:
		if err != nil {
			t.Fatalf("runEagerDaemon returned error after cancel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runEagerDaemon did not shut down promptly after context cancellation")
	}
}

// TestModifierBufferEntersAndFlushesWithinTimeout verifies issue 101's
// staleness gate: a gating-modifier press recorded within the timeout window
// causes modifierBuffer.EnterIfNeeded to enter (and stay in) buffering mode,
// and that buffered text is exactly what Flush later returns.
func TestModifierBufferEntersAndFlushesWithinTimeout(t *testing.T) {
	mgr := newEagerSessionManager(context.Background(), nil, func(context.Context, string, func()) {})
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	mgr.now = func() time.Time { return now }

	mgr.NoteModifierPress(now)
	now = now.Add(2 * time.Second)

	var buf modifierBuffer
	buffer, justEntered := buf.EnterIfNeeded(mgr, 10*time.Second)
	if !buffer || !justEntered {
		t.Fatalf("EnterIfNeeded() = (%v, %v), want (true, true) within the timeout window", buffer, justEntered)
	}
	buf.Append("hello ")

	// A later chunk, even once the original press is nearly stale, must stay
	// buffered (sticky) and must not re-trigger the notification.
	now = now.Add(5 * time.Second)
	buffer, justEntered = buf.EnterIfNeeded(mgr, 10*time.Second)
	if !buffer || justEntered {
		t.Fatalf("EnterIfNeeded() = (%v, %v), want (true, false) once already buffering (sticky)", buffer, justEntered)
	}
	buf.Append("world ")

	pending, wasBuffering := buf.Flush()
	if !wasBuffering {
		t.Fatal("Flush() wasBuffering = false, want true")
	}
	if want := "hello world "; pending != want {
		t.Fatalf("Flush() pending = %q, want %q", pending, want)
	}
	// Flush is called exactly once per session, right before
	// runEagerCaptureSession returns (see its own doc comment) -- there is
	// no "next chunk in the same session" to re-check afterward; a fresh
	// session gets its own fresh modifierBuffer instead (see
	// TestModifierBufferDoesNotLeakAcrossSessions).
}

// TestModifierBufferIgnoresStalePress verifies the other half of issue 101's
// staleness gate: a modifier press older than the timeout (e.g. unrelated
// hotkey use earlier in the session) must not trigger buffering -- blocking
// indefinitely on a stale press would silently drop output, the same class
// of regression issue 083 section 8 fixed once already for a different
// trigger.
func TestModifierBufferIgnoresStalePress(t *testing.T) {
	mgr := newEagerSessionManager(context.Background(), nil, func(context.Context, string, func()) {})
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	mgr.now = func() time.Time { return now }

	mgr.NoteModifierPress(now)
	now = now.Add(11 * time.Second) // just past a 10s timeout

	var buf modifierBuffer
	if buffer, justEntered := buf.EnterIfNeeded(mgr, 10*time.Second); buffer || justEntered {
		t.Fatalf("EnterIfNeeded() = (%v, %v), want (false, false) for a stale press", buffer, justEntered)
	}
}

// TestEagerSessionManagerModifierPressResetOnStartStop verifies issue 101's
// session-scoping requirement: a modifier press from one session must never
// leak into a later one. eagerSessionManager.Start() always calls Stop()
// first and rebuilds state from scratch (see activeSessionID etc. above) --
// lastModifierPressAt must reset the same way, not just conceptually
// "expire" via the timeout.
func TestEagerSessionManagerModifierPressResetOnStartStop(t *testing.T) {
	captureStarted := make(chan struct{}, 2)
	mgr := newEagerSessionManager(context.Background(), nil, func(ctx context.Context, sessionID string, onCaptureStopped func()) {
		captureStarted <- struct{}{}
		<-ctx.Done()
		onCaptureStopped()
	})

	mgr.Start()
	<-captureStarted
	mgr.NoteModifierPress(time.Now())
	if !mgr.ModifierPressedWithin(time.Hour) {
		t.Fatal("expected a recent press to be observed before Stop()")
	}

	mgr.Stop()
	mgr.Wait()

	if mgr.ModifierPressedWithin(time.Hour) {
		t.Fatal("after Stop(), ModifierPressedWithin() = true, want false -- state must not survive Stop")
	}

	mgr.Start()
	<-captureStarted
	if mgr.ModifierPressedWithin(time.Hour) {
		t.Fatal("a new session must not inherit a modifier press from a prior session")
	}
	mgr.Stop()
	mgr.Wait()
}

// TestModifierBufferDoesNotLeakAcrossSessions is the issue-101-review
// regression test: eagerSessionManager is a single long-lived object shared
// across a rapid Stop-then-Start, and a superseded session's trailing chunk
// can still reach the buffering decision (and flush) after the manager has
// already moved on to a new session. Since modifierBuffer is local to each
// runEagerCaptureSession call rather than a field on the shared manager (see
// its doc comment), two independent buffers against the *same* manager must
// never see each other's buffered text.
func TestModifierBufferDoesNotLeakAcrossSessions(t *testing.T) {
	captureStarted := make(chan struct{}, 2)
	mgr := newEagerSessionManager(context.Background(), nil, func(ctx context.Context, sessionID string, onCaptureStopped func()) {
		captureStarted <- struct{}{}
		<-ctx.Done()
		onCaptureStopped()
	})

	// Session A: a real Start(), a modifier press arms buffering, and A
	// buffers a trailing chunk that models still being "in flight" (not yet
	// flushed) when the session is superseded -- Stop() returns as soon as
	// capture stops (issue 057), without waiting for A's transcription
	// worker, so this exactly mirrors production timing.
	mgr.Start()
	<-captureStarted
	mgr.NoteModifierPress(time.Now())
	var sessionA modifierBuffer
	if buffer, _ := sessionA.EnterIfNeeded(mgr, 10*time.Second); !buffer {
		t.Fatal("expected session A to enter buffering")
	}
	sessionA.Append("leftover from session A ")
	mgr.Stop()
	mgr.Wait()

	// Session B: a real Start() resets lastModifierPressAt (see
	// TestEagerSessionManagerModifierPressResetOnStartStop), so B's own
	// modifierBuffer -- constructed fresh here, exactly as
	// runEagerCaptureSession does per call -- must not buffer without a new
	// press of its own.
	mgr.Start()
	<-captureStarted
	var sessionB modifierBuffer
	if buffer, _ := sessionB.EnterIfNeeded(mgr, 10*time.Second); buffer {
		t.Fatal("session B must not inherit session A's buffering state")
	}
	// Not buffering (per production usage, text is typed immediately, not
	// appended, when EnterIfNeeded returns false) -- B's buffer stays empty.
	mgr.Stop()
	mgr.Wait()

	// A's trailing chunk finally flushes (its own runEagerCaptureSession
	// reaching transWg.Wait(), which can happen after B has already started)
	// -- must return only A's text, never mixed with or replaced by B's.
	pendingA, wasBufferingA := sessionA.Flush()
	if !wasBufferingA || pendingA != "leftover from session A " {
		t.Fatalf("session A Flush() = (%q, %v), want (%q, true)", pendingA, wasBufferingA, "leftover from session A ")
	}

	// B's own text was never buffered (per the EnterIfNeeded check above) --
	// confirm B's buffer holds nothing, uncontaminated by A's leftover text.
	pendingB, wasBufferingB := sessionB.Flush()
	if wasBufferingB || pendingB != "" {
		t.Fatalf("session B Flush() = (%q, %v), want (\"\", false) -- B never buffered, so nothing should be pending", pendingB, wasBufferingB)
	}
}

// TestEagerSessionManagerIgnoresStartHotkeyModifierPress is the live-bug
// regression test for issue 101: the recording-start hotkey (e.g. Super+X)
// is itself a gating-modifier press, and NoteModifierPress fed by the
// daemon's poller observes it around the moment Start() runs. Without a
// grace window, the very first chunk of every session would see that press
// as "recent" and wrongly enter buffering -- confirmed live (starting a
// recording immediately triggered the "Typing paused" notice with no other
// modifier ever touched). NoteModifierPress must drop presses observed
// within modifierStartGrace of Start(), while still honoring a genuine
// later press within the same session.
func TestEagerSessionManagerIgnoresStartHotkeyModifierPress(t *testing.T) {
	mgr := newEagerSessionManager(context.Background(), nil, func(ctx context.Context, sessionID string, onCaptureStopped func()) {
		<-ctx.Done()
		onCaptureStopped()
	})
	mgr.modifierStartGrace = 750 * time.Millisecond
	base := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	mgr.now = func() time.Time { return base }

	mgr.Start()
	// The start hotkey's own press, observed by the poller moments after
	// Start() -- must be ignored.
	mgr.NoteModifierPress(base.Add(50 * time.Millisecond))
	if mgr.ModifierPressedWithin(time.Hour) {
		t.Fatal("ModifierPressedWithin() = true for a press within the start-grace window, want false")
	}

	// A genuine press well after the grace window must still be honored.
	mgr.NoteModifierPress(base.Add(2 * time.Second))
	if !mgr.ModifierPressedWithin(time.Hour) {
		t.Fatal("ModifierPressedWithin() = false for a press after the start-grace window, want true")
	}

	mgr.Stop()
	mgr.Wait()
}

// TestModifierBufferFlushCancelsPendingNotification is the live-bug
// regression test for issue 101: entering buffering scheduled the "Typing
// paused" notification to play immediately, so speaking a sentence and
// pressing the flush trigger (Stop, e.g. Super+X) right after it -- losing
// nothing, since buffering correctly held the text and the flush correctly
// typed it -- still played the notification pointlessly after the session
// had already closed. ScheduleNotify delays playback; Flush must cancel it
// if called first.
func TestModifierBufferFlushCancelsPendingNotification(t *testing.T) {
	var played atomic.Bool
	d := deps.Dependencies{
		LookPath: func(string) (string, error) { return "aplay", nil },
		Run: func(context.Context, string, ...string) error {
			played.Store(true)
			return nil
		},
		Stdout: io.Discard,
	}

	var buf modifierBuffer
	buf.ScheduleNotify(d, 100*time.Millisecond)
	// Flush immediately, well before the delay elapses -- must cancel.
	buf.Flush()

	time.Sleep(200 * time.Millisecond) // longer than the scheduled delay
	if played.Load() {
		t.Fatal("notification played after Flush canceled it before the delay elapsed")
	}
}

// TestModifierBufferScheduleNotifyPlaysWithoutFlush verifies the other half:
// with no Flush (or stop) in between, the delayed notification still plays.
func TestModifierBufferScheduleNotifyPlaysWithoutFlush(t *testing.T) {
	var played atomic.Bool
	d := deps.Dependencies{
		LookPath: func(string) (string, error) { return "aplay", nil },
		Run: func(context.Context, string, ...string) error {
			played.Store(true)
			return nil
		},
		Stdout: io.Discard,
	}

	var buf modifierBuffer
	buf.ScheduleNotify(d, 20*time.Millisecond)

	deadline := time.Now().Add(500 * time.Millisecond)
	for !played.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !played.Load() {
		t.Fatal("notification did not play within the deadline when nothing canceled it")
	}
}
