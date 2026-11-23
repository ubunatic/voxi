package sample

import (
	"fmt"
	"strings"
	"time"

	"ubunatic.com/loom"
)

// Rendering for the sample editor: the tab bar, search line, list and detail body,
// status line, and key hints. Nothing here changes editor state.

// Draw lays out the tab bar, the search line, the list and detail body, the
// status line, and the key hints. Every line is clipped to the pane width by
// display cells, so narrow panes and Unicode text never panic or split a rune.
func (e *sampleEditor) Draw(c *loom.Canvas, r loom.Rect) {
	e.mu.Lock()
	defer e.mu.Unlock()
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
	if e.drawAddMode(c, r) {
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
	case modeAddChoice:
		return "1 record mic · 2 recent chunks · Esc cancel"
	case modeRecording:
		return "Enter/Space stop · Esc cancel"
	case modePickChunk:
		return "j/k move · Enter select · Esc cancel"
	case modeAddForm:
		return "Tab next · d/n/v purpose · Enter save · Esc cancel"
	case modeConfirmAddVoice:
		return "y consent and save · n/Esc back"
	}
	return "j/k move · / search · 1-4 tab · p/Space play · a add · e edit · m move · d delete · q quit"
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
