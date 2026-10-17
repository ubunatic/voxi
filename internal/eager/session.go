package eager

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"ubunatic.com/voxi/internal/chunks"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/eager/notify"
	"ubunatic.com/voxi/internal/telemetry"
)

// Drop reasons carried in telemetry.Event.CancelReason and in the
// user-visible drop message (issue 115).
const (
	// dropSuperseded: a newer recording session started, so this generation's
	// leftover text must not be typed into whatever the new session owns.
	dropSuperseded = "superseded"
	// dropDrainDeadline: the absolute post-stop safety net expired. Should be
	// unreachable in practice -- see spec/eager.yaml drain.delivery_deadline_ms.
	dropDrainDeadline = "drain_deadline"
	// dropCaptureBoundary: the audio frame itself completed after the stop
	// request, so it was never part of the utterance the user intended.
	dropCaptureBoundary = "capture_boundary"
	// dropMicRepaired: the mic check (issue 177) found the recording's mic
	// broken and stopped it; nothing it captured is typed.
	dropMicRepaired = "mic_repaired"
)

type sessionDrain struct {
	// parent is the generation context: cancelled when a *newer* session
	// starts. It, not the wall clock, is the delivery policy (issue 115).
	parent  context.Context
	ctx     context.Context
	cancel  context.CancelFunc
	timeout time.Duration

	mu         sync.Mutex
	stopped    bool
	boundaryAt time.Time
	deadline   time.Time
	abandoned  string // drop reason once the whole session is discarded
}

// abandon stops the drain and drops everything this session would still type.
func (d *sessionDrain) abandon(reason string) {
	d.stop()
	d.mu.Lock()
	d.abandoned = reason
	d.mu.Unlock()
}

type stopRequest struct {
	mu sync.Mutex
	at time.Time
	// generation is cancelled when a newer session starts, i.e. the real
	// "this generation is abandoned" signal that the old five-second
	// stopDrainTimeout only approximated with a wall clock (issue 115).
	// A context rather than a channel so the drain can simply derive from it,
	// with no watcher goroutine.
	generation context.Context
	supersede  context.CancelFunc
}

func newStopRequest() *stopRequest {
	ctx, cancel := context.WithCancel(context.Background())
	return &stopRequest{generation: ctx, supersede: cancel}
}

func (r *stopRequest) mark(at time.Time) {
	r.mu.Lock()
	r.at = at
	r.mu.Unlock()
}

func (r *stopRequest) timestamp() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.at
}

// generationCtx is nil-safe: a nil or zero-value request is never superseded.
// That covers the standalone CLI path (no session manager, so no newer
// generation can exist) and tests that pass neither.
func (r *stopRequest) generationCtx() context.Context {
	if r == nil || r.generation == nil {
		return context.Background()
	}
	return r.generation
}

// markSuperseded declares this generation abandoned. Called only from
// eagerSessionManager.Start -- never from Stop, since stopping must not
// shorten a drain that is still the newest thing the user asked for.
func (r *stopRequest) markSuperseded() {
	if r != nil && r.supersede != nil {
		r.supersede()
	}
}

// newSessionDrainWithTimeout builds a drain that can never be superseded,
// for the standalone CLI path and for unit tests of the wall-clock net alone.
func newSessionDrainWithTimeout(timeout time.Duration) *sessionDrain {
	return newSessionDrainFor(context.Background(), timeout)
}

func newSessionDrainFor(parent context.Context, timeout time.Duration) *sessionDrain {
	ctx, cancel := context.WithCancel(parent)
	return &sessionDrain{parent: parent, ctx: ctx, cancel: cancel, timeout: timeout}
}

func (d *sessionDrain) stop() {
	d.stopAtBoundary(time.Now())
}

func (d *sessionDrain) stopAt(at time.Time) {
	d.stopAtBoundary(at)
}

func (d *sessionDrain) stopAtBoundary(boundary time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return
	}
	d.stopped = true
	d.boundaryAt = boundary
	d.deadline = time.Now().Add(d.timeout)
	time.AfterFunc(d.timeout, d.cancel)
}

func (d *sessionDrain) eligibleAt(at time.Time) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return !d.stopped || at.Before(d.boundaryAt)
}

// deliverable reports whether this generation may still type, and if not, why.
// The policy is "deliver unless a newer generation exists"; the deadline is
// only an absolute net against a hung, never-superseded generation typing
// minutes later (issue 115).
func (d *sessionDrain) deliverable() (bool, string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.abandoned != "" {
		return false, d.abandoned
	}
	if d.parent != nil && d.parent.Err() != nil {
		return false, dropSuperseded
	}
	if d.stopped && !time.Now().Before(d.deadline) {
		return false, dropDrainDeadline
	}
	return true, ""
}

// eagerSessionManager serializes start/stop/toggle requests against a single
// active recording session for the eager daemon.
//
// Issue 057 root cause: the daemon previously blocked a stop/start/toggle
// request on the *entire* session's lifetime (audio capture, plus draining
// any transcription job still queued or in flight, bounded only by
// transcribeTimeout, up to 30s). Since the agent control-plane
// (internal/agent/agent.go Agent.Record) holds a single mutex across the
// whole backend call, one stalled toggle stalled every other status/mode/
// record request behind it -- observed as a client-side socket read timeout
// (the CLI's 5s deadline firing while the daemon was still stuck) followed
// by a burst of queued requests all resolving back-to-back once the stale
// transcription finally finished, producing exactly the rapid-fire
// started/stopped/started/stopped log sequence from the ticket.
//
// The fix: decouple "capture has stopped" (bounded only by killing and
// reaping the recording subprocess -- fast) from "the session's full
// lifetime, including transcription drain, has finished" (potentially slow).
// Stop/Start/Toggle wait only for the former via captureStopped; a session's
// leftover transcription drain continues in the background and is only
// waited on by Wait, which the daemon calls once at shutdown.
type eagerSessionManager struct {
	ctx context.Context
	// run launches one recording session against sessCtx and must call
	// onCaptureStopped as soon as audio capture ends and its recording
	// subprocess is reaped, then may continue running (draining
	// transcription) until it returns.
	run      func(sessCtx context.Context, sessionID string, request *stopRequest, onCaptureStopped func())
	recorder *telemetry.Recorder
	now      func() time.Time
	// modifierStartGrace: see ignorePressesBefore below. Zero (the default
	// for tests that don't set it) disables the grace window entirely.
	modifierStartGrace time.Duration

	mu            sync.Mutex
	toggleMu      sync.Mutex // serializes check-and-act Toggle operations
	activeCancel  context.CancelFunc
	activeStopped chan struct{}
	activeRequest *stopRequest
	// prevRequest is the most recently started session's request, kept after
	// Stop clears activeRequest so the *next* Start can supersede it. A
	// stopped session still owns the desktop until something newer begins
	// (issue 115), so nothing but Start may ever cancel this.
	prevRequest         *stopRequest
	activeSessionID     string
	isRecording         bool
	sessWg              sync.WaitGroup
	lastModifierPressAt time.Time // zero value = no gating-modifier press seen yet this session; reset on Start/Stop
	ignorePressesBefore time.Time // NoteModifierPress drops presses before this (issue 101 start-grace fix); set in Start
}

func newEagerSessionManager(ctx context.Context, recorder *telemetry.Recorder, run func(context.Context, string, *stopRequest, func())) *eagerSessionManager {
	if recorder == nil {
		recorder = telemetry.NewRecorder("")
	}
	return &eagerSessionManager{ctx: ctx, recorder: recorder, now: time.Now, run: run}
}

// Stop cancels the active session, if any, and waits only until its audio
// capture has stopped (recording subprocess killed and reaped) -- not until
// its transcription drain (if any is still in flight) has finished.
func (m *eagerSessionManager) Stop() {
	m.mu.Lock()
	cancel := m.activeCancel
	stopped := m.activeStopped
	sessionID := m.activeSessionID
	request := m.activeRequest
	deactivatedAt := time.Time{}
	if cancel != nil {
		deactivatedAt = m.now()
	}
	m.activeCancel = nil
	m.activeStopped = nil
	m.activeSessionID = ""
	m.activeRequest = nil
	// Deliberately not cleared or superseded: the stopped session keeps the
	// right to type its already-captured speech until a new session starts.
	if request != nil {
		m.prevRequest = request
	}
	m.isRecording = false
	m.lastModifierPressAt = time.Time{}
	m.ignorePressesBefore = time.Time{}
	m.mu.Unlock()

	if cancel != nil {
		if request != nil {
			request.mark(deactivatedAt)
		}
		_ = m.recorder.Record(telemetry.Event{Event: telemetry.MicDeactivated, Timestamp: deactivatedAt, SessionID: sessionID})
		cancel()
	}
	if stopped != nil {
		<-stopped
	}
}

// StopSession stops the session only if it is still the active one, so a
// session ending itself (issue 177) never stops a newer one the user started.
func (m *eagerSessionManager) StopSession(sessionID string) {
	m.toggleMu.Lock()
	defer m.toggleMu.Unlock()
	m.mu.Lock()
	active := m.activeSessionID == sessionID
	m.mu.Unlock()
	if active {
		m.Stop()
	}
}

// Start stops any active session (see Stop) and begins a new one.
func (m *eagerSessionManager) Start() {
	m.Stop()

	m.mu.Lock()
	// Starting is the only event that abandons the previous generation: from
	// here on, its leftover transcripts must not reach the desktop the new
	// session is about to type into (issue 115). Stop() above already ended
	// that session's capture, so this happens strictly before the new session
	// produces any output.
	m.prevRequest.markSuperseded()
	activatedAt := m.now()
	sessionID := m.recorder.NewSessionID(activatedAt)
	sessCtx, cancel := context.WithCancel(m.ctx)
	stopped := make(chan struct{})
	request := newStopRequest()
	m.prevRequest = request
	m.activeCancel = cancel
	m.activeStopped = stopped
	m.activeSessionID = sessionID
	m.activeRequest = request
	m.isRecording = true
	// The start hotkey (e.g. Super+X) is itself a gating-modifier press --
	// without this grace window, the very press that starts the session
	// looks "recent" to the first chunk and wrongly enters buffering every
	// time (confirmed live during issue 101 review). Modifier presses within
	// modifierStartGrace of activatedAt are dropped.
	if m.modifierStartGrace > 0 {
		m.ignorePressesBefore = activatedAt.Add(m.modifierStartGrace)
	}
	m.sessWg.Add(1)
	m.mu.Unlock()

	_ = m.recorder.Record(telemetry.Event{Event: telemetry.MicActivated, Timestamp: activatedAt, SessionID: sessionID})

	var signalOnce sync.Once
	signalStopped := func() {
		signalOnce.Do(func() { close(stopped) })
	}

	go func() {
		defer m.sessWg.Done()
		defer signalStopped()
		m.run(sessCtx, sessionID, request, signalStopped)
	}()
}

// Toggle flips between recording and stopped. It returns a user-facing status string.
// Serialized across callers so rapid external signals/commands (e.g. SIGUSR1
// followed immediately by socket toggle) cannot race and start multiple
// concurrent sessions (issue 092).
func (m *eagerSessionManager) Toggle() string {
	m.toggleMu.Lock()
	defer m.toggleMu.Unlock()

	m.mu.Lock()
	recording := m.isRecording
	m.mu.Unlock()

	if recording {
		m.Stop()
		return "Recording stopped"
	}
	m.Start()
	return "Recording started"
}

func (m *eagerSessionManager) Recording() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.isRecording
}

func (m *eagerSessionManager) NoteModifierPress(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.ignorePressesBefore.IsZero() && t.Before(m.ignorePressesBefore) {
		// Within the post-Start grace window: almost certainly the start
		// hotkey's own press, not a genuine mid-dictation modifier use.
		return
	}
	m.lastModifierPressAt = t
}

// ModifierPressedWithin reports whether the most recent gating-modifier
// press was within timeout of now. Read-only and safe to call from any
// session's worker at any time -- unlike buffered text (see modifierBuffer
// below), this field is deliberately shared across the manager's whole
// lifetime and only reset at Start/Stop, since it is fed by a single
// daemon-wide poller rather than being session-exclusive state.
func (m *eagerSessionManager) ModifierPressedWithin(timeout time.Duration) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return !m.lastModifierPressAt.IsZero() && m.now().Sub(m.lastModifierPressAt) <= timeout
}

// modifierBuffer holds eager output buffered during the modifier-release
// race guard (issue 101). Deliberately a *local* value owned by one
// runEagerCaptureSession call, not a field on eagerSessionManager: the
// manager is a single long-lived object shared across a rapid Stop/Start
// sequence, while a session's trailing chunk can still be transcribing (and
// reach the buffering decision) after Stop() has already returned and a new
// session has begun -- storing pending/buffering state on the shared
// manager let a superseded session's leftover text bleed into, or get
// flushed by, the next session (confirmed live during review). Since each
// session has exactly one sequential transcription worker goroutine (see
// the jobChan loop below), no locking is needed here.
type modifierBuffer struct {
	buffering bool
	pending   strings.Builder
	entries   []bufferedDelivery
	// cancelNotify cancels a scheduled-but-not-yet-played notification (see
	// ScheduleNotify/Flush below); nil once played or canceled.
	cancelNotify context.CancelFunc
}

// bufferedDelivery carries enough of the chunk to finish its bookkeeping at
// flush time: Index for telemetry correlation, and Meta so the typing
// timestamps can be persisted the way the plain path already does (issue 115).
type bufferedDelivery struct {
	ID    string
	Text  string
	Index int
	Meta  chunks.Chunk
}

// EnterIfNeeded reports whether eager output should be buffered rather than
// typed immediately: either buffering is already active for this session
// (sticky -- see below), or sessions' most recent gating-modifier press is
// within timeout of now. Once entered, buffering stays active for the rest
// of the session regardless of later staleness, since a flush is only ever
// triggered explicitly by stopping the session (issue 101) -- a modifier
// press going stale mid-buffer does not retroactively make it safe to type.
// sessions may be nil (the standalone, non-daemon CLI path), which never
// buffers.
func (b *modifierBuffer) EnterIfNeeded(sessions *eagerSessionManager, timeout time.Duration) (buffer, justEntered bool) {
	if sessions == nil {
		return false, false
	}
	if b.buffering {
		return true, false
	}
	if sessions.ModifierPressedWithin(timeout) {
		b.buffering = true
		return true, true
	}
	return false, false
}

// Append adds s to the pending buffered output. Callers must have already
// confirmed buffering via EnterIfNeeded.
func (b *modifierBuffer) Append(s string) {
	b.pending.WriteString(s)
}

func (b *modifierBuffer) AppendDelivery(id, text string, index int, meta chunks.Chunk) {
	b.pending.WriteString(text)
	b.entries = append(b.entries, bufferedDelivery{ID: id, Text: text, Index: index, Meta: meta})
}

// ScheduleNotify plays the typing-paused notification after delay, unless
// canceled first (see Flush). Delaying rather than playing immediately
// avoids an unnecessary audible interruption when the session's flush
// trigger (Stop, e.g. pressing Super+X right after the last chunk) fires
// almost immediately after entering buffering: nothing was actually left
// for the user to be told to do, since the flush already typed it
// (confirmed live -- issue 101: speaking a sentence and immediately
// pressing Super+X lost nothing, yet still played the notification
// pointlessly after the session had already closed).
func (b *modifierBuffer) ScheduleNotify(d deps.Dependencies, delay time.Duration) {
	ctx, cancel := context.WithCancel(context.Background())
	b.cancelNotify = cancel
	go func() {
		select {
		case <-ctx.Done():
			return // canceled by Flush before the delay elapsed
		case <-time.After(delay):
		}
		if err := notify.PlayTypingPaused(context.Background(), d); err != nil {
			// Buffering itself still protects against injection with no
			// player available; log so the missing cue is discoverable
			// (journalctl --user -u voxi-agent.service) instead of a
			// silent dead-end with no hint to press Super+X.
			fmt.Fprintf(d.Stdout, "Warning: cannot play typing-paused notification: %v\n", err)
		}
	}()
}

// Flush returns and clears any buffered output, along with whether
// buffering was active. Intended to be called once, after this session's
// transcription worker has fully drained (see runEagerCaptureSession) --
// calling it while more chunks may still be queued would flush prematurely.
// Also cancels any notification scheduled but not yet played (see
// ScheduleNotify): reaching Flush means the session is already done, so
// there is nothing left to tell the user to finish.
func (b *modifierBuffer) Flush() (text string, wasBuffering bool) {
	entries, wasBuffering := b.FlushDeliveries()
	for _, entry := range entries {
		text += entry.Text
	}
	return text, wasBuffering
}

func (b *modifierBuffer) FlushDeliveries() (entries []bufferedDelivery, wasBuffering bool) {
	if b.cancelNotify != nil {
		b.cancelNotify()
		b.cancelNotify = nil
	}
	wasBuffering = b.buffering
	if len(b.entries) > 0 {
		entries = append(entries, b.entries...)
	} else if text := b.pending.String(); text != "" {
		entries = []bufferedDelivery{{Text: text}}
	}
	b.pending.Reset()
	b.entries = nil
	b.buffering = false
	return entries, wasBuffering
}

// Wait blocks until every session's full lifetime (capture + transcription
// drain) has completed. Intended for daemon shutdown only.
func (m *eagerSessionManager) Wait() {
	m.sessWg.Wait()
	// Every session has returned, so releasing the last generation context
	// cannot revoke a delivery that is still pending; without this the final
	// session's context would outlive the manager.
	m.mu.Lock()
	last := m.prevRequest
	m.prevRequest = nil
	m.mu.Unlock()
	last.markSuperseded()
}
