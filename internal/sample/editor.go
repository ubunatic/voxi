package sample

import (
	"fmt"
	"strings"
	"time"

	"ubunatic.com/loom"
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
)

// editorTabs lists the purpose filters in tab order. The empty purpose is "All".
func editorTabs() []Purpose {
	return []Purpose{"", Dictation, Noise, Voice}
}

// sampleEditor is the loom widget behind voxi sample editor and bare voxi sample edit.
// It reads and writes only through its Store and holds no audio state. M2 adds
// playback and add-from-mic or chunk flows by handling the reserved p and a keys.
type sampleEditor struct {
	store *Store
	now   func() time.Time

	samples []Sample
	tab     Purpose
	query   string
	cursor  int
	mode    editorMode
	status  string

	search *loom.TextInput

	// Edit form state. The transcript and keyterms fields are live only in modeEdit.
	editID      string
	editPurpose Purpose
	transcript  *loom.TextInput
	keyterms    *loom.TextInput
	editField   int // 0 transcript, 1 keyterms

	// pendingID is the sample a move-consent or delete prompt is about.
	pendingID string
}

func newSampleEditor(s *Store) *sampleEditor {
	e := &sampleEditor{store: s, now: time.Now}
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

// ConsumeKey routes a key to the handler for the current mode. Ctrl-C quits from
// any mode. Every other key is handled here, so loom's default Esc and q exits never fire.
func (e *sampleEditor) ConsumeKey(k loom.KeyEvent) loom.EventResult {
	if k.Name() == "ctrl-c" {
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
	}
	if quit {
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
		e.beginEdit()
	case "m":
		e.beginMove()
	case "d", "delete", "backspace":
		e.beginDelete()
	case "p":
		// M2 hook: playback lands with M2. Until then the key only reports that.
		e.setStatus("playback arrives in M2")
	case "a":
		// M2 hook: add-from-mic and add-from-chunks land with M2.
		e.setStatus("add arrives in M2")
	}
	return false
}

// beginSearch opens the filter line. The query stays live as the user types.
func (e *sampleEditor) beginSearch() {
	e.mode = modeSearch
	e.search = loom.NewTextInput(e.query)
	e.search.Prompt = "/ "
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
	e.transcript = loom.NewTextInput(x.Transcript)
	e.keyterms = loom.NewTextInput(strings.Join(x.Keyterms, ", "))
	e.editField = 0
	e.setStatus("edit " + x.ID)
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

// chooseMove refuses voice moves without a transcript and asks for own-voice
// consent before any other move into voice. Other moves run at once.
func (e *sampleEditor) chooseMove(x Sample, target Purpose) {
	switch {
	case x.Purpose == target:
		e.setStatus(fmt.Sprintf("%s is already in %s", x.ID, target))
	case target == Voice && strings.TrimSpace(x.Transcript) == "":
		e.setStatus(x.ID + " has no transcript; voice samples need one (press e first)")
	case target == Voice:
		e.mode = modeConfirmMove
		e.pendingID = x.ID
		e.setStatus("This is my own voice and I consent to cloning it? [y/N]")
	default:
		e.doMove(x.ID, target)
	}
}

// consentKey confirms or declines own-voice consent. The default is no.
func (e *sampleEditor) consentKey(k loom.KeyEvent) {
	switch k.Name() {
	case "y", "Y":
		e.mode = modeBrowse
		if err := e.store.GrantConsent(e.pendingID, e.now()); err != nil {
			e.setStatus("consent failed: " + err.Error())
			return
		}
		e.doMove(e.pendingID, Voice)
	case "n", "N", "esc", "enter":
		e.mode = modeBrowse
		e.setStatus("move to voice cancelled: no consent")
	}
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
	e.mode = modeConfirmDelete
	e.pendingID = x.ID
	e.setStatus(fmt.Sprintf("Delete %s and its audio? [y/N]", x.ID))
}

func (e *sampleEditor) deleteKey(k loom.KeyEvent) {
	switch k.Name() {
	case "y", "Y":
		id := e.pendingID
		e.mode = modeBrowse
		if err := e.store.Delete(id); err != nil {
			e.reload("")
			e.setStatus("delete failed: " + err.Error())
			return
		}
		e.reload("")
		e.setStatus("deleted " + id)
	case "n", "N", "esc", "enter":
		e.mode = modeBrowse
		e.setStatus("delete cancelled")
	}
}

// ConsumeMouse ignores the mouse; every action is a key.
func (e *sampleEditor) ConsumeMouse(loom.MouseEvent) loom.EventResult {
	return loom.Ignored()
}

// Draw lays out the tab bar, the search line, the list and detail body, the
// status line, and the key hints. Every line is clipped to the pane width by
// display cells, so narrow panes and Unicode text never panic or split a rune.
func (e *sampleEditor) Draw(c *loom.Canvas, r loom.Rect) {
	if r.W <= 0 {
		return
	}
	if r.H < minEditorRows {
		for i, line := range wrapText(fmt.Sprintf("Sample editor needs %d rows, have %d", minEditorRows, r.H), r.W) {
			writeLine(c, r, i, line, loom.Style{})
		}
		return
	}
	e.drawTabs(c, r.X, r.Y, r.W)
	e.drawSearch(c, r.X, r.Y+1, r.W)
	e.drawBody(c, loom.Rect{X: r.X, Y: r.Y + 2, W: r.W, H: r.H - chromeRows})
	writeLine(c, r, r.H-2, e.status, loom.Style{Bold: true})
	writeLine(c, r, r.H-1, e.hints(), loom.Style{Dim: true})
}

func (e *sampleEditor) drawTabs(c *loom.Canvas, x, y, w int) {
	used := 0
	for i, p := range editorTabs() {
		label := fmt.Sprintf("[%d] %s", i+1, tabName(p))
		if i > 0 {
			label = "  " + label
		}
		if used+loom.StringWidth(label) > w {
			break
		}
		style := loom.Style{Dim: true}
		if p == e.tab {
			style = loom.Style{Bold: true, Underline: true}
		}
		c.Write(x+used, y, label, style)
		used += loom.StringWidth(label)
	}
}

func (e *sampleEditor) drawSearch(c *loom.Canvas, x, y, w int) {
	if e.mode == modeSearch && e.search != nil {
		e.search.Draw(c, loom.Rect{X: x, Y: y, W: w, H: 1}, true)
		return
	}
	if e.query == "" {
		c.Write(x, y, clip("/ to search", w), loom.Style{Dim: true})
		return
	}
	c.Write(x, y, clip(fmt.Sprintf("/ %s (%d shown)", e.query, len(e.visible())), w), loom.Style{})
}

// drawBody puts the list beside the detail at splitMinWidth and above it when narrower.
func (e *sampleEditor) drawBody(c *loom.Canvas, r loom.Rect) {
	if r.H <= 0 {
		return
	}
	cur, ok := e.selected()
	if r.W >= splitMinWidth {
		listW := r.W * 2 / 5
		for y := 0; y < r.H; y++ {
			c.Write(r.X+listW, r.Y+y, "│", loom.Style{Dim: true})
		}
		e.drawList(c, loom.Rect{X: r.X, Y: r.Y, W: listW, H: r.H})
		e.drawDetail(c, loom.Rect{X: r.X + listW + 1, Y: r.Y, W: r.W - listW - 1, H: r.H}, cur, ok)
		return
	}
	listH := r.H / 2
	e.drawList(c, loom.Rect{X: r.X, Y: r.Y, W: r.W, H: listH})
	e.drawDetail(c, loom.Rect{X: r.X, Y: r.Y + listH, W: r.W, H: r.H - listH}, cur, ok)
}

func (e *sampleEditor) drawList(c *loom.Canvas, r loom.Rect) {
	if r.H <= 0 {
		return
	}
	xs := e.visible()
	if len(xs) == 0 {
		msg := "No samples in this purpose."
		switch {
		case len(e.samples) == 0:
			msg = "No samples yet. Record one with voxi sample record."
		case e.query != "":
			msg = fmt.Sprintf("No matches for %q. Esc clears the search.", e.query)
		}
		for i, line := range wrapText(msg, r.W) {
			if i < r.H {
				writeLine(c, r, i, line, loom.Style{Dim: true})
			}
		}
		return
	}
	start := 0
	if e.cursor >= r.H {
		start = e.cursor - r.H + 1
	}
	for i := 0; i < r.H && start+i < len(xs); i++ {
		idx := start + i
		writeLine(c, r, i, sampleRow(xs[idx], idx == e.cursor), loom.Style{Bold: idx == e.cursor})
	}
}

// drawDetail shows the selected sample, or the edit form while editing.
func (e *sampleEditor) drawDetail(c *loom.Canvas, r loom.Rect, x Sample, ok bool) {
	if r.H <= 0 || r.W <= 0 {
		return
	}
	if e.mode == modeEdit && e.transcript != nil {
		e.drawEditForm(c, r)
		return
	}
	if !ok {
		writeLine(c, r, 0, "Nothing selected.", loom.Style{Dim: true})
		return
	}
	lines := []string{
		"ID: " + x.ID,
		"Purpose: " + string(x.Purpose),
		"Audio: " + orDefault(x.Audio, "(none)"),
		"Source: " + orDefault(x.Source, "(unknown)"),
		"Created: " + timeLabel(x.Created, "unknown"),
		"Consent: " + consentLabel(x),
		"Transcript:",
	}
	transcript := wrapText(x.Transcript, r.W)
	if len(transcript) == 0 {
		transcript = []string{"(empty)"}
	}
	lines = append(lines, transcript...)
	lines = append(lines, "Keyterms (jargon):")
	if len(x.Keyterms) == 0 {
		lines = append(lines, "(none)")
	} else {
		lines = append(lines, wrapText(strings.Join(x.Keyterms, ", "), r.W)...)
	}
	for i, line := range lines {
		if i >= r.H {
			break
		}
		writeLine(c, r, i, line, loom.Style{})
	}
}

// drawEditForm draws the transcript and keyterms inputs with the focused field marked.
func (e *sampleEditor) drawEditForm(c *loom.Canvas, r loom.Rect) {
	y := r.Y
	end := r.Y + r.H
	label := func(text string) {
		if y < end {
			c.Write(r.X, y, clip(text, r.W), loom.Style{Bold: true})
		}
		y++
	}
	input := func(in *loom.TextInput, focused bool) {
		if y < end {
			in.Draw(c, loom.Rect{X: r.X, Y: y, W: r.W, H: 1}, focused)
		}
		y++
	}
	label("Edit " + e.editID + " (" + string(e.editPurpose) + ")")
	label("Transcript:")
	input(e.transcript, e.editField == 0)
	label("Keyterms, comma-separated:")
	input(e.keyterms, e.editField == 1)
}

// hints returns the key hints for the active mode.
func (e *sampleEditor) hints() string {
	switch e.mode {
	case modeSearch:
		return "type to filter · Enter keep · Esc clear · ↑/↓ move"
	case modeEdit:
		return "Tab next field · Enter save · Esc cancel"
	case modeMove:
		return "d dictation · n noise · v voice · Esc cancel"
	case modeConfirmMove:
		return "y consent and move · n/Esc cancel"
	case modeConfirmDelete:
		return "y delete · n/Esc cancel"
	}
	return "j/k move · / search · 1-4 tab · e edit · m move · d delete · p/a (M2) · q quit"
}

// sampleRow renders one list line: marker, purpose badge, ★ for keyterms,
// ✓ for voice consent, ID, and a one-line transcript preview.
func sampleRow(x Sample, selected bool) string {
	mark := "  "
	if selected {
		mark = "> "
	}
	star := " "
	if len(x.Keyterms) > 0 {
		star = "★"
	}
	check := " "
	if x.Purpose == Voice && x.Consent != nil {
		check = "✓"
	}
	preview := strings.Join(strings.Fields(x.Transcript), " ")
	if preview == "" {
		preview = "(no transcript)"
	}
	return mark + purposeBadge(x.Purpose) + " " + star + check + " " + x.ID + "  " + preview
}

func purposeBadge(p Purpose) string {
	switch p {
	case Dictation:
		return "[dict]"
	case Noise:
		return "[nois]"
	case Voice:
		return "[voic]"
	}
	return "[????]"
}

func tabName(p Purpose) string {
	if p == "" {
		return "All"
	}
	return string(p)
}

func consentLabel(x Sample) string {
	if x.Consent == nil {
		return "none"
	}
	return "given " + timeLabel(*x.Consent, "unknown")
}

func timeLabel(t time.Time, zero string) string {
	if t.IsZero() {
		return zero
	}
	return t.Local().Format("2006-01-02 15:04")
}

func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// clip fits text to w display cells, adding an ellipsis only when it is cut.
func clip(text string, w int) string {
	return loom.TruncateText(text, w, "…")
}

// writeLine draws one clipped line at row dy inside r. Rows outside r are skipped.
func writeLine(c *loom.Canvas, r loom.Rect, dy int, text string, style loom.Style) {
	if dy < 0 || dy >= r.H || r.W <= 0 {
		return
	}
	c.Write(r.X, r.Y+dy, clip(text, r.W), style)
}

// wrapText breaks text into lines no wider than width display cells. It breaks at
// spaces where it can and hard-breaks words that are too wide, always on rune
// boundaries, so Unicode transcripts never split a rune.
func wrapText(text string, width int) []string {
	if width <= 0 {
		return nil
	}
	var lines []string
	for _, para := range strings.Split(text, "\n") {
		line := ""
		for _, word := range strings.Fields(para) {
			for loom.StringWidth(word) > width {
				head, rest := splitWidth(word, width)
				if line != "" {
					lines = append(lines, line)
					line = ""
				}
				lines = append(lines, head)
				word = rest
			}
			switch {
			case line == "":
				line = word
			case loom.StringWidth(line)+1+loom.StringWidth(word) <= width:
				line += " " + word
			default:
				lines = append(lines, line)
				line = word
			}
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// splitWidth cuts s after the longest rune prefix that fits in width cells. It
// always keeps at least one rune, so a glyph wider than width still makes progress.
func splitWidth(s string, width int) (head, rest string) {
	used := 0
	for i, r := range s {
		w := loom.RuneWidth(r)
		if i > 0 && used+w > width {
			return s[:i], s[i:]
		}
		used += w
	}
	return s, ""
}

// runSampleEditor opens the interactive editor in a loom pane on the real terminal.
// Callers check for a terminal first.
func runSampleEditor(s *Store) error {
	pane, err := loom.New(editorRows)
	if err != nil {
		return fmt.Errorf("start sample editor: %w", err)
	}
	defer pane.Close()
	pane.SetScreenMode(loom.ScreenAlt)
	pane.Resizeable = true
	pane.MaxCols = 0
	return pane.Run(newSampleEditor(s))
}
