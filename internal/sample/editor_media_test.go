package sample

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ubunatic.com/loom"
	"ubunatic.com/voxi/internal/audio"
	"ubunatic.com/voxi/internal/chunks"
)

// TestEditorPlaybackInlineSpawnDoesNotDeadlock runs playback with spawn executing
// inline. The playback goroutine takes mu, so it must start only after ConsumeKey
// releases mu.
func TestEditorPlaybackInlineSpawnDoesNotDeadlock(t *testing.T) {
	e, s := newTestEditor(t)
	putSampleWithAudio(t, s, Sample{ID: "clip", Purpose: Dictation, Transcript: "hello"})
	e.reload("")
	e.spawn = func(f func()) { f() }
	e.play = func(ctx context.Context, wavPath string) error { return nil }
	press(e, "p")
	if e.status != "finished playing clip" || e.playCancel != nil {
		t.Fatalf("inline playback: status=%q playing=%v", e.status, e.playCancel != nil)
	}
}

// TestEditorM2ModesRenderInNarrowPane draws each M2 mode in a 40x14 pane and checks
// that every line fits the width and that the mode's prompt is visible.
func TestEditorM2ModesRenderInNarrowPane(t *testing.T) {
	e, _ := newTestEditor(t)
	e.chunkBuf = &fakeChunkSource{list: []chunks.Chunk{{
		SessionID: "s", Index: 1, AudioDurationSecs: 1,
		CleanedTranscript: "a very long transcript that will certainly not fit in forty columns",
	}}, wav: map[int][]byte{1: make([]byte, 320)}}
	check := func(want string) {
		t.Helper()
		lines := screen(e, 40, 14)
		for _, l := range lines {
			if w := loom.StringWidth(l); w > 40 {
				t.Fatalf("line is %d cells wide in a 40-column pane: %q", w, l)
			}
		}
		if out := strings.Join(lines, "\n"); !strings.Contains(out, want) {
			t.Fatalf("mode %v: missing %q in:\n%s", e.mode, want, out)
		}
	}

	press(e, "a")
	check("Record from mic")
	press(e, "2")
	check("recent chunks")
	press(e, "j")
	press(e, "\r")
	check("New sample")
	typeText(e, "narrow-voice")
	pressKeys(e, "tab", "v", "\r")
	check("consent")
	// Esc in the consent prompt goes back to the form; a second Esc cancels the add.
	press(e, "\x1b")
	if e.mode != modeAddForm {
		t.Fatalf("esc in consent should return to form, mode=%v", e.mode)
	}
	check("New sample")
	press(e, "\x1b")
	if e.mode != modeBrowse {
		t.Fatalf("esc should leave the form, mode=%v", e.mode)
	}
}

// TestEditorRecordingScreenNarrowAndCancel draws the recording prompt in a narrow pane
// and cancels the recording, checking that the temp state is cleared.
func TestEditorRecordingScreenNarrowAndCancel(t *testing.T) {
	e, _ := newTestEditor(t)
	recStarted := make(chan struct{}, 1)
	recCanceled := make(chan struct{}, 1)
	e.record = func(ctx context.Context, stop <-chan struct{}) ([]byte, time.Duration, error) {
		recStarted <- struct{}{}
		select {
		case <-stop:
			return make([]byte, 320), time.Second, nil
		case <-ctx.Done():
			recCanceled <- struct{}{}
			return nil, 0, ctx.Err()
		}
	}
	pressKeys(e, "a", "1")
	<-recStarted
	lines := screen(e, 40, 14)
	for _, l := range lines {
		if loom.StringWidth(l) > 40 {
			t.Fatalf("recording line too wide: %q", l)
		}
	}
	if out := strings.Join(lines, "\n"); !strings.Contains(out, "Press Enter or Space") {
		t.Fatalf("recording prompt missing in narrow pane:\n%s", out)
	}
	press(e, "\x1b")
	<-recCanceled
	if e.mode != modeBrowse || e.recCancel != nil || e.recStop != nil {
		t.Fatalf("cancel left recording state: mode=%v", e.mode)
	}
}

type fakeChunkSource struct {
	list []chunks.Chunk
	wav  map[int][]byte
	err  error
}

func (f *fakeChunkSource) List(bool) ([]chunks.Chunk, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.list, nil
}

func (f *fakeChunkSource) ReadWAV(c chunks.Chunk) ([]byte, error) {
	b, ok := f.wav[c.Index]
	if !ok {
		return nil, os.ErrNotExist
	}
	return b, nil
}

func putSampleWithAudio(t *testing.T, s *Store, x Sample) {
	t.Helper()
	wavPath := filepath.Join(t.TempDir(), x.ID+".wav")
	if err := audio.WriteWAVAudio(wavPath, make([]byte, 320), 16000); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(x, wavPath); err != nil {
		t.Fatal(err)
	}
}

func TestEditorPlaybackStartToggleStopAndCancelOnQuit(t *testing.T) {
	e, s := newTestEditor(t)
	putSampleWithAudio(t, s, Sample{ID: "clip", Purpose: Dictation, Transcript: "hello"})
	e.reload("")

	started := make(chan string, 2)
	canceled := make(chan struct{}, 2)
	done := make(chan struct{}, 2)
	e.play = func(ctx context.Context, wavPath string) error {
		started <- wavPath
		<-ctx.Done()
		canceled <- struct{}{}
		done <- struct{}{}
		return ctx.Err()
	}

	// Start with p, toggle-stop with space.
	press(e, "p")
	if got := <-started; !strings.HasSuffix(got, "clip.wav") {
		t.Fatalf("played path = %q", got)
	}
	press(e, " ")
	<-canceled
	<-done
	if e.status != "playback stopped" {
		t.Fatalf("stop status = %q", e.status)
	}

	// Start with space, cancel on quit.
	press(e, " ")
	<-started
	if res := press(e, "q"); !res.Quit {
		t.Fatal("q should quit")
	}
	<-canceled
	<-done
}

func TestEditorAddFromMicDictationAndVoiceConsent(t *testing.T) {
	e, s := newTestEditor(t)
	e.spawn = func(f func()) { f() } // deterministic inline execution
	invCount := 0
	e.invalidate = func() { invCount++ }
	e.record = func(ctx context.Context, stop <-chan struct{}) ([]byte, time.Duration, error) {
		return make([]byte, 320), 1500 * time.Millisecond, nil
	}
	e.transcribe = func(ctx context.Context, pcm []byte) (string, error) {
		return "deploy kubectl cluster", nil
	}
	e.suggestKeyterms = func(text string) []string {
		return []string{"kubectl"}
	}

	// Open add -> [1] Record from mic -> inline completes into modeAddForm.
	pressKeys(e, "a", "1")
	if e.mode != modeAddForm {
		t.Fatalf("expected modeAddForm, got %v (status=%q)", e.mode, e.status)
	}
	if e.addTranscript.Value() != "deploy kubectl cluster" || e.addKeyterms.Value() != "kubectl" {
		t.Fatalf("prefill mismatch: transcript=%q keyterms=%q", e.addTranscript.Value(), e.addKeyterms.Value())
	}
	if invCount == 0 {
		t.Fatal("expected invalidate after recording completed")
	}

	// Invalid ID rejected, form stays open.
	typeText(e, "bad id!")
	press(e, "\r")
	if e.mode != modeAddForm || !strings.Contains(e.status, "invalid sample id") {
		t.Fatalf("invalid id: mode=%v status=%q", e.mode, e.status)
	}

	// Valid ID saves to dictation and cleans up temp WAV.
	tmpWAV := e.addWAVPath
	clearField(e)
	typeText(e, "mic-one")
	press(e, "\r")
	if e.mode != modeBrowse || e.status != "added mic-one" {
		t.Fatalf("save dictation: mode=%v status=%q", e.mode, e.status)
	}
	if _, err := os.Stat(tmpWAV); !os.IsNotExist(err) {
		t.Fatalf("temp wav %s was not cleaned up", tmpWAV)
	}
	got, err := s.Get("mic-one")
	if err != nil {
		t.Fatal(err)
	}
	if got.Purpose != Dictation || got.Transcript != "deploy kubectl cluster" || strings.Join(got.Keyterms, "|") != "kubectl" || got.Source != "record" || got.Audio == "" {
		t.Fatalf("saved dictation sample = %+v", got)
	}

	// Duplicate ID rejected, then save to voice with consent confirmation.
	pressKeys(e, "a", "1")
	typeText(e, "mic-one")
	press(e, "\r")
	if e.mode != modeAddForm || !strings.Contains(e.status, "already exists") {
		t.Fatalf("duplicate id: mode=%v status=%q", e.mode, e.status)
	}
	clearField(e)
	typeText(e, "my-voice")
	pressKeys(e, "tab", "v") // switch to Purpose field and choose voice
	press(e, "\r")
	if e.mode != modeConfirmAddVoice {
		t.Fatalf("expected voice consent prompt, got mode=%v status=%q", e.mode, e.status)
	}
	// Declining returns to form without saving.
	press(e, "n")
	if e.mode != modeAddForm {
		t.Fatalf("declined consent should return to form, got %v", e.mode)
	}
	if has, _ := s.Has("my-voice"); has {
		t.Fatal("declined voice sample should not be saved")
	}
	// Confirming saves with consent timestamp.
	press(e, "\r")
	press(e, "y")
	if e.mode != modeBrowse || e.status != "added my-voice" {
		t.Fatalf("confirmed voice save: mode=%v status=%q", e.mode, e.status)
	}
	v, err := s.Get("my-voice")
	if err != nil {
		t.Fatal(err)
	}
	if v.Purpose != Voice || v.Consent == nil || !v.Consent.Equal(fixedNow) || v.Audio == "" {
		t.Fatalf("saved voice sample = %+v", v)
	}
}

func TestEditorAddFromMicStopCancelAndASRFallback(t *testing.T) {
	e, s := newTestEditor(t)
	recStarted := make(chan struct{}, 2)
	recCanceled := make(chan struct{}, 1)
	formReady := make(chan struct{}, 1)
	e.invalidate = func() {
		select {
		case formReady <- struct{}{}:
		default:
		}
	}
	e.record = func(ctx context.Context, stop <-chan struct{}) ([]byte, time.Duration, error) {
		recStarted <- struct{}{}
		select {
		case <-stop:
			return make([]byte, 320), time.Second, nil
		case <-ctx.Done():
			recCanceled <- struct{}{}
			return nil, 0, ctx.Err()
		}
	}
	e.transcribe = func(ctx context.Context, pcm []byte) (string, error) {
		return "", errors.New("asr offline")
	}

	// Cancel recording with Esc.
	pressKeys(e, "a", "1")
	<-recStarted
	if out := screenText(e, 80, 14); !strings.Contains(out, "Recording") {
		t.Fatalf("recording screen missing:\n%s", out)
	}
	press(e, "\x1b")
	<-recCanceled
	if e.mode != modeBrowse || e.status != "recording cancelled" {
		t.Fatalf("cancel recording: mode=%v status=%q", e.mode, e.status)
	}

	// Stop recording with Enter; ASR fails non-fatally and user can still type & save.
	// Drain any invalidate signal left over from the cancelled run first.
	select {
	case <-formReady:
	default:
	}
	pressKeys(e, "a", "1")
	<-recStarted
	press(e, "\r")
	<-formReady
	if e.mode != modeAddForm || !strings.Contains(e.status, "ASR unavailable") {
		t.Fatalf("asr fallback: mode=%v status=%q", e.mode, e.status)
	}
	typeText(e, "manual-mic")
	pressKeys(e, "tab", "tab") // ID -> Purpose -> Transcript
	typeText(e, "typed manually")
	press(e, "\r")
	if e.mode != modeBrowse || e.status != "added manual-mic" {
		t.Fatalf("manual save: mode=%v status=%q", e.mode, e.status)
	}
	got, err := s.Get("manual-mic")
	if err != nil || got.Transcript != "typed manually" {
		t.Fatalf("saved manual sample = %+v, err=%v", got, err)
	}
}

func TestEditorAddFromRecentChunksAndCancelCleanup(t *testing.T) {
	e, s := newTestEditor(t)
	e.chunkBuf = &fakeChunkSource{}
	pressKeys(e, "a", "2")
	if e.mode != modeBrowse || !strings.Contains(e.status, "no recent chunks") {
		t.Fatalf("empty chunks: mode=%v status=%q", e.mode, e.status)
	}

	wavPath := filepath.Join(t.TempDir(), "c.wav")
	if err := audio.WriteWAVAudio(wavPath, make([]byte, 320), 16000); err != nil {
		t.Fatal(err)
	}
	wavBytes, err := os.ReadFile(wavPath)
	if err != nil {
		t.Fatal(err)
	}
	e.chunkBuf = &fakeChunkSource{
		list: []chunks.Chunk{
			{SessionID: "sess1", Index: 2, AudioDurationSecs: 1.2, CleanedTranscript: "first chunk"},
			{SessionID: "sess1", Index: 1, AudioDurationSecs: 2.3, RawTranscript: "raw voxi chunk"},
		},
		wav: map[int][]byte{1: wavBytes, 2: wavBytes},
	}
	e.suggestKeyterms = func(text string) []string { return []string{"voxi"} }

	// Select second chunk and cancel from form -> temp WAV cleaned up.
	pressKeys(e, "a", "2", "j", "\r")
	if e.mode != modeAddForm || e.addTranscript.Value() != "raw voxi chunk" || e.addKeyterms.Value() != "voxi" {
		t.Fatalf("chunk prefill: mode=%v tr=%q kt=%q", e.mode, e.addTranscript.Value(), e.addKeyterms.Value())
	}
	tmpWAV := e.addWAVPath
	press(e, "\x1b")
	if e.mode != modeBrowse || e.status != "add cancelled" {
		t.Fatalf("cancel chunk add: mode=%v status=%q", e.mode, e.status)
	}
	if _, err := os.Stat(tmpWAV); !os.IsNotExist(err) {
		t.Fatalf("temp chunk wav %s not removed on cancel", tmpWAV)
	}

	// Select first chunk and save.
	pressKeys(e, "a", "2", "\r")
	typeText(e, "chunk-one")
	press(e, "\r")
	if e.mode != modeBrowse || e.status != "added chunk-one" {
		t.Fatalf("save chunk: mode=%v status=%q", e.mode, e.status)
	}
	got, err := s.Get("chunk-one")
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "chunk:sess1/2" || got.Transcript != "first chunk" || got.Audio == "" {
		t.Fatalf("saved chunk sample = %+v", got)
	}
}

// TestEditorPlaybackChildContextCancelledOnCompletion checks that the context
// handed to play is live while it runs and cancelled once playback returns
// normally. Spawn runs inline, so the closure has finished when press returns.
func TestEditorPlaybackChildContextCancelledOnCompletion(t *testing.T) {
	e, s := newTestEditor(t)
	putSampleWithAudio(t, s, Sample{ID: "clip", Purpose: Dictation, Transcript: "hello"})
	e.reload("")
	e.spawn = func(f func()) { f() }
	var child context.Context
	e.play = func(ctx context.Context, wavPath string) error {
		child = ctx
		if err := ctx.Err(); err != nil {
			t.Fatalf("child context already cancelled while playing: %v", err)
		}
		return nil
	}
	press(e, "p")
	if child == nil {
		t.Fatal("play was not called")
	}
	if err := child.Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("child playback context after normal completion: err=%v, want canceled", err)
	}
	if e.playCancel != nil {
		t.Fatal("playCancel should be cleared after completion")
	}
}

// TestEditorPlaybackChildContextCancelledOnError covers a play call that fails.
func TestEditorPlaybackChildContextCancelledOnError(t *testing.T) {
	e, s := newTestEditor(t)
	putSampleWithAudio(t, s, Sample{ID: "clip", Purpose: Dictation, Transcript: "hello"})
	e.reload("")
	e.spawn = func(f func()) { f() }
	var child context.Context
	e.play = func(ctx context.Context, wavPath string) error {
		child = ctx
		return errors.New("no output device")
	}
	press(e, "p")
	if child == nil || child.Err() == nil {
		t.Fatalf("child playback context not cancelled after failed play: err=%v", child)
	}
	if e.status != "playback failed: no output device" {
		t.Fatalf("status = %q", e.status)
	}
}

// TestEditorRecordingChildContextCancelledOnEarlyError checks that the recorder's
// context is cancelled when recording fails before the user stops it.
func TestEditorRecordingChildContextCancelledOnEarlyError(t *testing.T) {
	e, _ := newTestEditor(t)
	e.spawn = func(f func()) { f() }
	var child context.Context
	e.record = func(ctx context.Context, stop <-chan struct{}) ([]byte, time.Duration, error) {
		child = ctx
		if err := ctx.Err(); err != nil {
			t.Fatalf("recorder context already cancelled at start: %v", err)
		}
		return nil, 0, errors.New("device busy")
	}
	pressKeys(e, "a", "1")
	if child == nil || !errors.Is(child.Err(), context.Canceled) {
		t.Fatalf("recorder context after early error: err=%v, want canceled", child)
	}
	if e.mode != modeBrowse || e.status != "recording failed: device busy" {
		t.Fatalf("early error: mode=%v status=%q", e.mode, e.status)
	}
	if e.recCancel != nil || e.recStop != nil {
		t.Fatal("recording state should be cleared after early error")
	}
}

// TestEditorRecordingChildContextCancelledOnCompletion checks the normal path:
// the recorder returns audio, ASR fills the form, and the context is released.
func TestEditorRecordingChildContextCancelledOnCompletion(t *testing.T) {
	e, _ := newTestEditor(t)
	e.spawn = func(f func()) { f() }
	var child context.Context
	e.record = func(ctx context.Context, stop <-chan struct{}) ([]byte, time.Duration, error) {
		child = ctx
		return make([]byte, 320), time.Second, nil
	}
	e.transcribe = func(ctx context.Context, pcm []byte) (string, error) { return "ok", nil }
	e.suggestKeyterms = func(string) []string { return nil }
	t.Cleanup(e.cleanup) // removes the temp WAV the form is holding
	pressKeys(e, "a", "1")
	if e.mode != modeAddForm {
		t.Fatalf("expected add form, mode=%v status=%q", e.mode, e.status)
	}
	if child == nil || !errors.Is(child.Err(), context.Canceled) {
		t.Fatalf("recorder context after normal completion: err=%v, want canceled", child)
	}
}
