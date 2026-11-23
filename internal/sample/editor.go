package sample

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"ubunatic.com/loom"
	"ubunatic.com/voxi/internal/chunks"
	"ubunatic.com/voxi/internal/deps"
)

// editorRows is the pane height the sample editor is laid out for.
const editorRows = 14

// chromeRows are the fixed lines around the list and detail body: the tab bar,
// the search line, the status line, and the key hints.
const chromeRows = 4

// minEditorRows is the shortest pane that still draws the body.
const minEditorRows = chromeRows + 1

// splitMinWidth is the narrowest pane that shows the list and detail side by side.
// Narrower panes stack the detail under the list.
const splitMinWidth = 60

// editorMode says which key handler is active. Every mode other than browse is a
// prompt that takes over the keys until it is confirmed or cancelled.
type editorMode int

const (
	modeBrowse editorMode = iota
	modeSearch
	modeEdit
	modeMove
	modeConfirmMove
	modeConfirmDelete
	modeAddChoice
	modeRecording
	modePickChunk
	modeAddForm
	modeConfirmAddVoice
)

// sampleEditor is the loom widget behind voxi sample editor and bare voxi sample edit.
// Background playback, mic recording, and ASR prefill run via spawn and synchronize
// state under mu before calling invalidate.
type sampleEditor struct {
	mu    sync.Mutex
	store *Store
	ctx   context.Context
	deps  deps.Dependencies
	now   func() time.Time

	spawn           func(func())
	pending         []func() // background work queued under mu, started after unlock
	invalidate      func()
	play            func(ctx context.Context, wavPath string) error
	record          func(ctx context.Context, stop <-chan struct{}) ([]byte, time.Duration, error)
	transcribe      func(ctx context.Context, pcm []byte) (string, error)
	suggestKeyterms func(text string) []string
	chunkBuf        chunkSource

	samples []Sample
	tab     Purpose
	query   string
	cursor  int
	mode    editorMode
	status  string

	search *loom.TextEntry

	// Edit form state. The transcript and keyterms fields are live only in modeEdit.
	editID      string
	editPurpose Purpose
	transcript  *loom.TextEntry
	keyterms    *loom.TextEntry
	editField   int // 0 transcript, 1 keyterms

	// pendingID and confirm hold state for move-consent, add-voice-consent, or delete prompts.
	pendingID string
	confirm   *loom.Confirm

	// Playback state.
	playCancel context.CancelFunc
	playID     string
	playSeq    uint64

	// Mic recording state.
	recCancel   context.CancelFunc
	recStop     chan struct{}
	recStopOnce sync.Once
	recSeq      uint64
	recStarted  time.Time

	// Recent chunk picker state.
	recentChunks []chunks.Chunk
	chunkCursor  int

	// Add form state.
	addID         *loom.TextEntry
	addPurpose    Purpose
	addTranscript *loom.TextEntry
	addKeyterms   *loom.TextEntry
	addField      int // 0 ID, 1 Purpose, 2 Transcript, 3 Keyterms
	addSource     string
	addWAVPath    string
}

func newSampleEditor(s *Store) *sampleEditor {
	return newSampleEditorWithDeps(context.Background(), deps.Dependencies{}, s, nil)
}

func newSampleEditorWithDeps(ctx context.Context, d deps.Dependencies, s *Store, invalidate func()) *sampleEditor {
	if ctx == nil {
		ctx = context.Background()
	}
	if invalidate == nil {
		invalidate = func() {}
	}
	e := &sampleEditor{
		store:           s,
		ctx:             ctx,
		deps:            d,
		now:             time.Now,
		spawn:           func(f func()) { go f() },
		invalidate:      invalidate,
		play:            defaultSamplePlayer(d),
		record:          defaultSampleRecorder(d),
		transcribe:      defaultSampleTranscriber(d),
		suggestKeyterms: defaultKeytermSuggester(d),
		chunkBuf:        defaultChunkSource(d),
	}
	e.reload("")
	return e
}

// reload re-reads the store and keeps the cursor on keepID when that sample is still visible.
func (e *sampleEditor) reload(keepID string) {
	xs, err := e.store.List()
	e.samples = xs
	if keepID != "" {
		for i, x := range e.visible() {
			if x.ID == keepID {
				e.cursor = i
				break
			}
		}
	}
	e.clamp()
	if err != nil {
		e.status = "some samples could not be read: " + err.Error()
	}
}

// visible returns the samples that match the purpose tab and the search query.
func (e *sampleEditor) visible() []Sample {
	q := strings.ToLower(strings.TrimSpace(e.query))
	var out []Sample
	for _, x := range e.samples {
		if e.tab != "" && x.Purpose != e.tab {
			continue
		}
		if q != "" && !sampleMatches(x, q) {
			continue
		}
		out = append(out, x)
	}
	return out
}

// sampleMatches reports whether the lower-case query q appears in the ID,
// transcript, or keyterms of x.
func sampleMatches(x Sample, q string) bool {
	hay := strings.ToLower(strings.Join(append([]string{x.ID, x.Transcript}, x.Keyterms...), "\n"))
	return strings.Contains(hay, q)
}

func (e *sampleEditor) selected() (Sample, bool) {
	v := e.visible()
	if e.cursor < 0 || e.cursor >= len(v) {
		return Sample{}, false
	}
	return v[e.cursor], true
}

// clamp keeps the cursor inside the visible list, including when it is empty.
func (e *sampleEditor) clamp() {
	n := len(e.visible())
	if e.cursor >= n {
		e.cursor = n - 1
	}
	if e.cursor < 0 {
		e.cursor = 0
	}
}

func (e *sampleEditor) move(delta int) {
	e.cursor += delta
	e.clamp()
}

func (e *sampleEditor) setTab(p Purpose) {
	e.tab = p
	e.cursor = 0
	e.clamp()
}

func (e *sampleEditor) cycleTab(delta int) {
	tabs := editorTabs()
	idx := 0
	for i, p := range tabs {
		if p == e.tab {
			idx = i
		}
	}
	e.setTab(tabs[(idx+delta+len(tabs))%len(tabs)])
}

func (e *sampleEditor) setStatus(s string) { e.status = s }

// editorTabs lists the purpose filters in tab order. The empty purpose is "All".
func editorTabs() []Purpose {
	return []Purpose{"", Dictation, Noise, Voice}
}

// ConsumeKey routes a key to the handler for the current mode. Ctrl-C quits from
// any mode. Every other key is handled here, so loom's default Esc and q exits never fire.
func (e *sampleEditor) ConsumeKey(k loom.KeyEvent) loom.EventResult {
	e.mu.Lock()
	res := e.handleKeyLocked(k)
	work := e.pending
	e.pending = nil
	e.mu.Unlock()
	// Start background work only after mu is released, so an inline spawn
	// (used by tests) can take mu itself without deadlocking.
	for _, f := range work {
		e.spawn(f)
	}
	return res
}

// queueLocked records background work to start once mu is released.
func (e *sampleEditor) queueLocked(f func()) {
	e.pending = append(e.pending, f)
}

func (e *sampleEditor) handleKeyLocked(k loom.KeyEvent) loom.EventResult {
	if k.Name() == "ctrl-c" {
		e.cleanupLocked()
		return loom.Quit()
	}
	quit := false
	switch e.mode {
	case modeBrowse:
		quit = e.browseKey(k)
	case modeSearch:
		e.searchKey(k)
	case modeEdit:
		e.editKey(k)
	case modeMove:
		e.moveKey(k)
	case modeConfirmMove:
		e.consentKey(k)
	case modeConfirmDelete:
		e.deleteKey(k)
	case modeAddChoice:
		e.addChoiceKey(k)
	case modeRecording:
		e.recordingKey(k)
	case modePickChunk:
		e.pickChunkKey(k)
	case modeAddForm:
		e.addFormKey(k)
	case modeConfirmAddVoice:
		e.confirmAddVoiceKey(k)
	}
	if quit {
		e.cleanupLocked()
		return loom.Quit()
	}
	return loom.Handled()
}

// browseKey handles the list keys and reports whether the editor should quit.
func (e *sampleEditor) browseKey(k loom.KeyEvent) (quit bool) {
	switch name := k.Name(); name {
	case "q":
		return true
	case "j", "down":
		e.move(1)
	case "k", "up":
		e.move(-1)
	case "1", "2", "3", "4":
		e.setTab(editorTabs()[int(name[0]-'1')])
	case "tab":
		e.cycleTab(1)
	case "shift-tab":
		e.cycleTab(-1)
	case "/":
		e.beginSearch()
	case "esc":
		if e.query != "" {
			e.query = ""
			e.cursor = 0
			e.setStatus("search cleared")
		}
	case "e":
		e.stopPlaybackLocked()
		e.beginEdit()
	case "m":
		e.stopPlaybackLocked()
		e.beginMove()
	case "d", "delete", "backspace":
		e.stopPlaybackLocked()
		e.beginDelete()
	case "p", " ":
		e.togglePlaybackLocked()
	case "a":
		e.stopPlaybackLocked()
		e.beginAddLocked()
	}
	return false
}

func newEditorTextEntry(value, prompt string, maxH int, focused bool) *loom.TextEntry {
	entry := loom.NewTextEntry(loom.TextEntryOptions{
		Prompt:    prompt,
		Height:    1,
		MinHeight: 1,
		MaxHeight: maxH,
	})
	entry.SetValue(value)
	entry.SetFocus(focused)
	return entry
}

// beginSearch opens the filter line. The query stays live as the user types.
func (e *sampleEditor) beginSearch() {
	e.mode = modeSearch
	e.search = newEditorTextEntry(e.query, "/ ", 1, true)
	e.setStatus("")
}

// searchKey edits the filter. Enter keeps the query and returns to browse.
// Esc clears it. Up and down move the selection while the filter is open.
func (e *sampleEditor) searchKey(k loom.KeyEvent) {
	switch k.Name() {
	case "enter":
		e.mode = modeBrowse
	case "esc":
		e.mode = modeBrowse
		e.query = ""
		e.cursor = 0
		e.setStatus("search cleared")
	case "up":
		e.move(-1)
	case "down":
		e.move(1)
	default:
		before := e.search.Value()
		e.search.ConsumeKey(k)
		if e.search.Value() != before {
			e.query = e.search.Value()
			e.cursor = 0
			e.clamp()
		}
	}
}

func (e *sampleEditor) beginEdit() {
	x, ok := e.selected()
	if !ok {
		e.setStatus("no sample selected")
		return
	}
	e.mode = modeEdit
	e.editID = x.ID
	e.editPurpose = x.Purpose
	e.transcript = newEditorTextEntry(x.Transcript, "", 4, true)
	e.keyterms = newEditorTextEntry(strings.Join(x.Keyterms, ", "), "", 2, false)
	e.editField = 0
	e.setStatus("edit " + x.ID)
}

func (e *sampleEditor) syncEditFocus() {
	if e.transcript != nil {
		e.transcript.SetFocus(e.editField == 0)
	}
	if e.keyterms != nil {
		e.keyterms.SetFocus(e.editField == 1)
	}
}

// editKey edits the transcript and keyterms fields. Enter saves, Esc cancels.
// Empty transcripts are refused for dictation and voice, and the form stays open.
func (e *sampleEditor) editKey(k loom.KeyEvent) {
	switch k.Name() {
	case "esc":
		e.mode = modeBrowse
		e.setStatus("edit cancelled")
	case "tab", "shift-tab":
		e.editField = 1 - e.editField
		e.syncEditFocus()
	case "enter":
		e.saveEdit()
	default:
		if e.editField == 0 {
			e.transcript.ConsumeKey(k)
		} else {
			e.keyterms.ConsumeKey(k)
		}
	}
}

func (e *sampleEditor) saveEdit() {
	text := strings.TrimSpace(e.transcript.Value())
	if requiresTranscript(e.editPurpose) && text == "" {
		e.setStatus(fmt.Sprintf("%s samples need a transcript; type one, or Esc to cancel", e.editPurpose))
		return
	}
	keyterms := NormalizeKeyterms(strings.Split(e.keyterms.Value(), ","))
	if err := e.store.UpdateDetails(e.editID, text, keyterms); err != nil {
		e.setStatus("save failed: " + err.Error())
		return
	}
	id := e.editID
	e.mode = modeBrowse
	e.reload(id)
	e.setStatus("saved " + id)
}

// requiresTranscript reports whether a sample of purpose p needs a non-empty
// transcript. Noise samples may have none.
func requiresTranscript(p Purpose) bool { return p != Noise }

func (e *sampleEditor) beginMove() {
	if _, ok := e.selected(); !ok {
		e.setStatus("no sample selected")
		return
	}
	e.mode = modeMove
	e.setStatus("move to:")
}

func (e *sampleEditor) moveKey(k loom.KeyEvent) {
	var target Purpose
	switch k.Name() {
	case "esc":
		e.mode = modeBrowse
		e.setStatus("move cancelled")
		return
	case "d":
		target = Dictation
	case "n":
		target = Noise
	case "v":
		target = Voice
	default:
		return
	}
	e.mode = modeBrowse
	x, ok := e.selected()
	if !ok {
		e.setStatus("no sample selected")
		return
	}
	e.chooseMove(x, target)
}

func (e *sampleEditor) beginConfirm(mode editorMode, prompt string) {
	e.mode = mode
	e.confirm = loom.NewConfirm("").DefaultNo()
	e.setStatus(prompt)
}

// consumeConfirmKey routes a key through loom.Confirm and reports whether a choice
// was finalized. Note: loom v0.16.0's Confirm.ConsumeKey returns Ignored on
// left/right/h/l even though it toggles selection, so we check Answered directly.
func (e *sampleEditor) consumeConfirmKey(k loom.KeyEvent) (answered, confirmed bool) {
	if e.confirm == nil {
		return true, false
	}
	e.confirm.ConsumeKey(k)
	if !e.confirm.Answered() {
		return false, false
	}
	return true, e.confirm.Confirmed()
}

// chooseMove refuses voice moves without a transcript and asks for own-voice
// consent before any other move into voice. Other moves run at once.
func (e *sampleEditor) chooseMove(x Sample, target Purpose) {
	switch {
	case x.Purpose == target:
		e.setStatus(fmt.Sprintf("%s is already in %s", x.ID, target))
	case target == Voice && strings.TrimSpace(x.Transcript) == "":
		e.setStatus(x.ID + " has no transcript; voice samples need one (press e first)")
	case target == Voice:
		e.pendingID = x.ID
		e.beginConfirm(modeConfirmMove, "This is my own voice and I consent to cloning it?")
	default:
		e.doMove(x.ID, target)
	}
}

// consentKey confirms or declines own-voice consent. The default is no.
func (e *sampleEditor) consentKey(k loom.KeyEvent) {
	answered, confirmed := e.consumeConfirmKey(k)
	if !answered {
		return
	}
	e.mode = modeBrowse
	if !confirmed {
		e.setStatus("move to voice cancelled: no consent")
		return
	}
	if err := e.store.GrantConsent(e.pendingID, e.now()); err != nil {
		e.setStatus("consent failed: " + err.Error())
		return
	}
	e.doMove(e.pendingID, Voice)
}

func (e *sampleEditor) doMove(id string, target Purpose) {
	if err := e.store.Move(id, target); err != nil {
		e.reload(id)
		e.setStatus("move failed: " + err.Error())
		return
	}
	e.reload(id)
	e.setStatus(fmt.Sprintf("moved %s to %s", id, target))
}

func (e *sampleEditor) beginDelete() {
	x, ok := e.selected()
	if !ok {
		e.setStatus("no sample selected")
		return
	}
	e.pendingID = x.ID
	e.beginConfirm(modeConfirmDelete, fmt.Sprintf("Delete %s and its audio?", x.ID))
}

func (e *sampleEditor) deleteKey(k loom.KeyEvent) {
	answered, confirmed := e.consumeConfirmKey(k)
	if !answered {
		return
	}
	e.mode = modeBrowse
	if !confirmed {
		e.setStatus("delete cancelled")
		return
	}
	id := e.pendingID
	if err := e.store.Delete(id); err != nil {
		e.reload("")
		e.setStatus("delete failed: " + err.Error())
		return
	}
	e.reload("")
	e.setStatus("deleted " + id)
}

// ConsumeMouse ignores the mouse; every action is a key.
func (e *sampleEditor) ConsumeMouse(loom.MouseEvent) loom.EventResult {
	return loom.Ignored()
}

// runSampleEditor opens the interactive editor in a loom pane on the real terminal.
// Callers check for a terminal first.
func runSampleEditor(ctx context.Context, d deps.Dependencies, s *Store) error {
	pane, err := loom.New(editorRows)
	if err != nil {
		return fmt.Errorf("start sample editor: %w", err)
	}
	defer pane.Close()
	ed := newSampleEditorWithDeps(ctx, d, s, pane.Invalidate)
	defer ed.cleanup()
	pane.SetScreenMode(loom.ScreenAlt)
	pane.Resizeable = true
	pane.MaxCols = 0
	return pane.Run(ed)
}
