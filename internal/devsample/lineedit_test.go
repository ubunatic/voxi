package devsample

import (
	"bufio"
	"strings"
	"testing"
)

// applyAll feeds a sequence of key events into a fresh editor pre-filled
// with prefill (cursor at its end, matching editLine's real startup state)
// and returns the resulting line + cursor position.
func applyAll(t *testing.T, prefill string, evs []keyEvent) (line string, cursor int, done, cancelled bool) {
	t.Helper()
	ed := newLineEditor(prefill)
	for _, ev := range evs {
		done, cancelled = ed.apply(ev)
		if done || cancelled {
			break
		}
	}
	return ed.String(), ed.cursor, done, cancelled
}

func TestLineEditorPrefillStartsWithCursorAtEnd(t *testing.T) {
	ed := newLineEditor("hello")
	if ed.String() != "hello" || ed.cursor != 5 {
		t.Fatalf("newLineEditor(%q) = %q cursor=%d, want %q cursor=5", "hello", ed.String(), ed.cursor, "hello")
	}
}

func TestLineEditorEnterSubmitsUnmodifiedPrefill(t *testing.T) {
	line, _, done, cancelled := applyAll(t, "raw asr guess", []keyEvent{{kind: keyEnter}})
	if !done || cancelled {
		t.Fatalf("done=%v cancelled=%v, want done", done, cancelled)
	}
	if line != "raw asr guess" {
		t.Errorf("line = %q, want unmodified prefill", line)
	}
}

func TestLineEditorCtrlCCancels(t *testing.T) {
	_, _, done, cancelled := applyAll(t, "some text", []keyEvent{{kind: keyLeft}, {kind: keyCtrlC}})
	if done || !cancelled {
		t.Fatalf("done=%v cancelled=%v, want cancelled", done, cancelled)
	}
}

func TestLineEditorCharInsertAtCursor(t *testing.T) {
	// "helloworld" with cursor moved left 5 (between "hello" and "world"),
	// then insert " " -> "hello world".
	evs := []keyEvent{
		{kind: keyLeft}, {kind: keyLeft}, {kind: keyLeft}, {kind: keyLeft}, {kind: keyLeft},
		{kind: keyChar, ch: ' '},
	}
	line, cursor, _, _ := applyAll(t, "helloworld", evs)
	if line != "hello world" {
		t.Errorf("line = %q, want %q", line, "hello world")
	}
	if cursor != 6 {
		t.Errorf("cursor = %d, want 6", cursor)
	}
}

func TestLineEditorBackspaceDeletesBeforeCursor(t *testing.T) {
	// cursor at end of "hello!", backspace once -> "hello".
	line, cursor, _, _ := applyAll(t, "hello!", []keyEvent{{kind: keyBackspace}})
	if line != "hello" || cursor != 5 {
		t.Errorf("line=%q cursor=%d, want %q cursor=5", line, cursor, "hello")
	}
}

func TestLineEditorBackspaceAtStartIsNoop(t *testing.T) {
	evs := []keyEvent{{kind: keyHome}, {kind: keyBackspace}}
	line, cursor, _, _ := applyAll(t, "hello", evs)
	if line != "hello" || cursor != 0 {
		t.Errorf("line=%q cursor=%d, want unchanged %q cursor=0", line, cursor, "hello")
	}
}

func TestLineEditorDeleteRemovesAtCursor(t *testing.T) {
	// cursor at Home (0), Delete removes the 'h'.
	evs := []keyEvent{{kind: keyHome}, {kind: keyDelete}}
	line, cursor, _, _ := applyAll(t, "hello", evs)
	if line != "ello" || cursor != 0 {
		t.Errorf("line=%q cursor=%d, want %q cursor=0", line, cursor, "ello")
	}
}

func TestLineEditorDeleteAtEndIsNoop(t *testing.T) {
	line, cursor, _, _ := applyAll(t, "hello", []keyEvent{{kind: keyDelete}})
	if line != "hello" || cursor != 5 {
		t.Errorf("line=%q cursor=%d, want unchanged %q cursor=5", line, cursor, "hello")
	}
}

func TestLineEditorLeftRightMoveByOneChar(t *testing.T) {
	evs := []keyEvent{{kind: keyLeft}, {kind: keyLeft}, {kind: keyRight}}
	_, cursor, _, _ := applyAll(t, "hello", evs) // 5 -> 4 -> 3 -> 4
	if cursor != 4 {
		t.Errorf("cursor = %d, want 4", cursor)
	}
}

func TestLineEditorLeftAtStartAndRightAtEndAreNoops(t *testing.T) {
	_, cursor, _, _ := applyAll(t, "hi", []keyEvent{{kind: keyHome}, {kind: keyLeft}})
	if cursor != 0 {
		t.Errorf("left at start: cursor = %d, want 0", cursor)
	}
	_, cursor, _, _ = applyAll(t, "hi", []keyEvent{{kind: keyRight}})
	if cursor != 2 {
		t.Errorf("right at end: cursor = %d, want 2", cursor)
	}
}

func TestLineEditorHomeAndEnd(t *testing.T) {
	_, cursor, _, _ := applyAll(t, "hello world", []keyEvent{{kind: keyHome}})
	if cursor != 0 {
		t.Errorf("Home: cursor = %d, want 0", cursor)
	}
	_, cursor, _, _ = applyAll(t, "hello world", []keyEvent{{kind: keyHome}, {kind: keyEnd}})
	if cursor != 11 {
		t.Errorf("End: cursor = %d, want 11", cursor)
	}
}

func TestLineEditorWordLeftAndWordRight(t *testing.T) {
	// "the quick fox", cursor starts at end (13).
	// Ctrl-Left once -> start of "fox" (10).
	_, cursor, _, _ := applyAll(t, "the quick fox", []keyEvent{{kind: keyWordLeft}})
	if cursor != 10 {
		t.Errorf("word-left once: cursor = %d, want 10", cursor)
	}
	// Ctrl-Left twice -> start of "quick" (4).
	_, cursor, _, _ = applyAll(t, "the quick fox", []keyEvent{{kind: keyWordLeft}, {kind: keyWordLeft}})
	if cursor != 4 {
		t.Errorf("word-left twice: cursor = %d, want 4", cursor)
	}
	// From Home, Ctrl-Right once -> just past "the" (3).
	_, cursor, _, _ = applyAll(t, "the quick fox", []keyEvent{{kind: keyHome}, {kind: keyWordRight}})
	if cursor != 3 {
		t.Errorf("word-right once: cursor = %d, want 3", cursor)
	}
	// From Home, Ctrl-Right twice -> just past "quick" (9).
	_, cursor, _, _ = applyAll(t, "the quick fox", []keyEvent{{kind: keyHome}, {kind: keyWordRight}, {kind: keyWordRight}})
	if cursor != 9 {
		t.Errorf("word-right twice: cursor = %d, want 9", cursor)
	}
}

func TestLineEditorWordLeftFromMidWordSkipsToWordStart(t *testing.T) {
	// cursor placed inside "quick" (index 7, between "qui" and "ck"),
	// word-left should land at the start of "quick" (4), not "the" (0).
	evs := []keyEvent{{kind: keyHome}, {kind: keyWordRight}, {kind: keyRight}, {kind: keyRight}, {kind: keyRight}, {kind: keyWordLeft}}
	_, cursor, _, _ := applyAll(t, "the quick fox", evs)
	if cursor != 4 {
		t.Errorf("cursor = %d, want 4 (start of \"quick\")", cursor)
	}
}

// --- Escape-sequence / byte-level parsing (no real TTY needed: readByte
// parsing is pure stream decoding; the deadline path is only exercised for
// a lone ESC and armReadDeadline no-ops when tty is nil). ---

func parseAll(t *testing.T, input string) []keyEvent {
	t.Helper()
	r := bufio.NewReader(strings.NewReader(input))
	var evs []keyEvent
	for {
		ev, err := readKeyEvent(r, nil)
		if err != nil {
			break
		}
		evs = append(evs, ev)
	}
	return evs
}

func TestReadKeyEventParsesPlainArrowsHomeEndDelete(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  keyKind
	}{
		{"left", "\x1b[D", keyLeft},
		{"right", "\x1b[C", keyRight},
		{"home-letter", "\x1b[H", keyHome},
		{"end-letter", "\x1b[F", keyEnd},
		{"home-tilde", "\x1b[1~", keyHome},
		{"end-tilde", "\x1b[4~", keyEnd},
		{"delete", "\x1b[3~", keyDelete},
		{"ctrl-left", "\x1b[1;5D", keyWordLeft},
		{"ctrl-right", "\x1b[1;5C", keyWordRight},
		{"ss3-home", "\x1bOH", keyHome},
		{"ss3-end", "\x1bOF", keyEnd},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			evs := parseAll(t, c.input)
			if len(evs) != 1 || evs[0].kind != c.want {
				t.Errorf("parseAll(%q) = %+v, want single event kind %v", c.input, evs, c.want)
			}
		})
	}
}

func TestReadKeyEventParsesPlainCharsBackspaceEnter(t *testing.T) {
	evs := parseAll(t, "ab\x7f\r")
	want := []keyKind{keyChar, keyChar, keyBackspace, keyEnter}
	if len(evs) != len(want) {
		t.Fatalf("parseAll = %+v, want %d events", evs, len(want))
	}
	for i, k := range want {
		if evs[i].kind != k {
			t.Errorf("event %d kind = %v, want %v", i, evs[i].kind, k)
		}
	}
	if evs[0].ch != 'a' || evs[1].ch != 'b' {
		t.Errorf("chars = %q %q, want a b", evs[0].ch, evs[1].ch)
	}
}

func TestReadKeyEventCtrlC(t *testing.T) {
	evs := parseAll(t, "\x03")
	if len(evs) != 1 || evs[0].kind != keyCtrlC {
		t.Errorf("parseAll(Ctrl-C) = %+v, want single keyCtrlC event", evs)
	}
}

func TestReadKeyEventUnknownEscapeSequenceIsIgnoredNotFatal(t *testing.T) {
	// An unrecognized final byte on an otherwise well-formed CSI sequence
	// must not error or desync parsing of what follows.
	evs := parseAll(t, "\x1b[Za")
	if len(evs) != 2 {
		t.Fatalf("parseAll = %+v, want 2 events (ignore + char)", evs)
	}
	if evs[0].kind != keyIgnore {
		t.Errorf("event 0 kind = %v, want keyIgnore", evs[0].kind)
	}
	if evs[1].kind != keyChar || evs[1].ch != 'a' {
		t.Errorf("event 1 = %+v, want char 'a'", evs[1])
	}
}

func TestReadKeyEventMultiByteUTF8(t *testing.T) {
	evs := parseAll(t, "héllo")
	if len(evs) != 5 {
		t.Fatalf("parseAll(héllo) = %+v, want 5 rune events", evs)
	}
	got := string([]rune{evs[0].ch, evs[1].ch, evs[2].ch, evs[3].ch, evs[4].ch})
	if got != "héllo" {
		t.Errorf("decoded = %q, want héllo", got)
	}
}

// --- Non-TTY fallback plumbing: useLineEditor/stdinIsTerminal must reject
// everything a test or a pipe ever hands them, so promptText/promptKeyterms
// always take the pre-045 fallback path in this sandbox. ---

func TestUseLineEditorFalseWithoutRealTTY(t *testing.T) {
	if useLineEditor(&strings.Builder{}, nil) {
		t.Error("useLineEditor with nil stdinFile must be false")
	}
	if stdinIsTerminal(nil) {
		t.Error("stdinIsTerminal(nil) must be false")
	}
}
