package devsample

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
	"unicode/utf8"

	"golang.org/x/term"
)

// errAborted is returned by the interactive line editor when the user
// presses Ctrl-C while editing a prompt. Record propagates it unchanged so
// Ctrl-C aborts the whole record flow, the same way it would at a shell
// prompt — no partial sample is ever written, since both editable prompts
// run before any disk write in Record.
var errAborted = errors.New("aborted by user (Ctrl-C)")

// escapeTimeout bounds how long readEscapeSequence waits for the bytes that
// follow a lone ESC (0x1b) before giving up and treating it as a bare
// Escape keypress rather than the start of an ANSI sequence. Real escape
// sequences (arrow keys, Home/End, ...) arrive as a contiguous burst from
// the terminal, well under this window; a human pressing the physical
// Escape key alone produces nothing further within it.
const escapeTimeout = 50 * time.Millisecond

// keyKind enumerates the editing operations the line editor understands.
type keyKind int

const (
	keyChar keyKind = iota
	keyBackspace
	keyDelete
	keyLeft
	keyRight
	keyWordLeft
	keyWordRight
	keyHome
	keyEnd
	keyEnter
	keyCtrlC
	keyIgnore // recognized-as-unsupported or undecodable input; a no-op
)

type keyEvent struct {
	kind keyKind
	ch   rune
}

// lineEditor holds an in-progress edit buffer as runes (not bytes) so
// cursor math and word boundaries are correct for multi-byte UTF-8 input,
// alongside the cursor position, also measured in runes.
type lineEditor struct {
	runes  []rune
	cursor int
}

// newLineEditor starts an editor pre-filled with prefill, cursor positioned
// at the end — the same feel as a shell history entry recalled with the
// up-arrow: the text is all there, ready to edit in place.
func newLineEditor(prefill string) *lineEditor {
	r := []rune(prefill)
	return &lineEditor{runes: r, cursor: len(r)}
}

func (e *lineEditor) String() string { return string(e.runes) }

// apply mutates the editor per ev and reports whether editing is finished
// (Enter) and whether it was cancelled (Ctrl-C). Both are reported rather
// than one of a tri-state so callers can't confuse "not done" with a zero
// value meaning something else.
func (e *lineEditor) apply(ev keyEvent) (done, cancelled bool) {
	switch ev.kind {
	case keyChar:
		e.runes = append(e.runes[:e.cursor:e.cursor], append([]rune{ev.ch}, e.runes[e.cursor:]...)...)
		e.cursor++
	case keyBackspace:
		if e.cursor > 0 {
			e.runes = append(e.runes[:e.cursor-1], e.runes[e.cursor:]...)
			e.cursor--
		}
	case keyDelete:
		if e.cursor < len(e.runes) {
			e.runes = append(e.runes[:e.cursor], e.runes[e.cursor+1:]...)
		}
	case keyLeft:
		if e.cursor > 0 {
			e.cursor--
		}
	case keyRight:
		if e.cursor < len(e.runes) {
			e.cursor++
		}
	case keyWordLeft:
		e.cursor = wordLeftIndex(e.runes, e.cursor)
	case keyWordRight:
		e.cursor = wordRightIndex(e.runes, e.cursor)
	case keyHome:
		e.cursor = 0
	case keyEnd:
		e.cursor = len(e.runes)
	case keyEnter:
		done = true
	case keyCtrlC:
		cancelled = true
	case keyIgnore:
		// no-op
	}
	return
}

func isWordRune(r rune) bool { return r != ' ' && r != '\t' }

// wordLeftIndex mirrors a shell readline's Ctrl-Left: skip any whitespace
// immediately to the left of the cursor, then skip the word itself,
// landing on the word's first character.
func wordLeftIndex(runes []rune, cursor int) int {
	i := cursor
	for i > 0 && !isWordRune(runes[i-1]) {
		i--
	}
	for i > 0 && isWordRune(runes[i-1]) {
		i--
	}
	return i
}

// wordRightIndex mirrors Ctrl-Right: skip whitespace, then skip the word,
// landing just past its last character (readline/emacs convention).
func wordRightIndex(runes []rune, cursor int) int {
	i := cursor
	n := len(runes)
	for i < n && !isWordRune(runes[i]) {
		i++
	}
	for i < n && isWordRune(runes[i]) {
		i++
	}
	return i
}

// stdinIsTerminal reports whether f is a real terminal worth switching into
// raw mode for. nil (stdin isn't an *os.File — piped, a test's
// strings.Reader, ...) is never a terminal.
func stdinIsTerminal(f *os.File) bool {
	return f != nil && term.IsTerminal(int(f.Fd()))
}

// useLineEditor reports whether the interactive raw-mode line editor should
// be used for this prompt: it needs somewhere to echo to (out) and a real
// terminal to read from.
func useLineEditor(out io.Writer, stdinFile *os.File) bool {
	return out != nil && stdinIsTerminal(stdinFile)
}

// editLine runs an interactive terminal line edit against tty (which
// useLineEditor has already confirmed is a real terminal) starting from
// prefill with the cursor at its end. Raw mode disables the terminal's own
// echo and line editing, so editLine echoes prompt+content itself via out.
//
// Supported keys: Left/Right (character), Ctrl-Left/Ctrl-Right (word),
// Home/End, Backspace/Delete, Enter to submit. Ctrl-C returns errAborted.
//
// tty and the *bufio.Reader in must be reading the same underlying stream
// (Record shares one *bufio.Reader across every stdin prompt so nothing
// buffered ahead by an earlier prompt gets silently dropped); editLine only
// uses tty's file descriptor for raw-mode syscalls and escape-sequence read
// deadlines, never as a second, independent read source.
func editLine(out io.Writer, in *bufio.Reader, tty *os.File, prompt, prefill string) (string, error) {
	fd := int(tty.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		// IsTerminal said yes but raw mode isn't actually available (some
		// unusual pty/CI setups): fall back to a plain blank-or-replace
		// read rather than hang or crash.
		line, rerr := in.ReadString('\n')
		if rerr != nil && rerr != io.EOF {
			return "", fmt.Errorf("read input: %w", rerr)
		}
		if trimmed := sanitizeText(line); trimmed != "" {
			return trimmed, nil
		}
		return prefill, nil
	}
	defer func() { _ = term.Restore(fd, state) }()

	ed := newLineEditor(prefill)
	redraw(out, prompt, ed)
	for {
		ev, rerr := readKeyEvent(in, tty)
		if rerr != nil {
			if rerr == io.EOF {
				fmt.Fprint(out, "\r\n")
				return ed.String(), nil
			}
			return "", fmt.Errorf("read input: %w", rerr)
		}
		done, cancelled := ed.apply(ev)
		if cancelled {
			fmt.Fprint(out, "\r\n")
			return "", errAborted
		}
		if done {
			fmt.Fprint(out, "\r\n")
			return ed.String(), nil
		}
		redraw(out, prompt, ed)
	}
}

// redraw repaints the current prompt+line: return to column 0, print
// prompt and the edited content, clear anything stale to the right of it
// (a previous, longer edit), then move the cursor left to its true
// in-line position. Reprinting the prompt every time (rather than trying
// to only patch the content) keeps this correct regardless of what the
// previous redraw left on screen.
func redraw(out io.Writer, prompt string, ed *lineEditor) {
	if out == nil {
		return
	}
	fmt.Fprintf(out, "\r%s%s\x1b[K", prompt, ed.String())
	if back := len(ed.runes) - ed.cursor; back > 0 {
		fmt.Fprintf(out, "\x1b[%dD", back)
	}
}

// readKeyEvent reads one logical keypress from in (byte(s) for a plain
// character, or a full ANSI escape sequence for arrows/Home/End/etc.).
func readKeyEvent(in *bufio.Reader, tty *os.File) (keyEvent, error) {
	b, err := in.ReadByte()
	if err != nil {
		return keyEvent{}, err
	}
	switch b {
	case 0x03:
		return keyEvent{kind: keyCtrlC}, nil
	case '\r', '\n':
		return keyEvent{kind: keyEnter}, nil
	case 0x7f, 0x08:
		return keyEvent{kind: keyBackspace}, nil
	case 0x1b:
		return readEscapeSequence(in, tty)
	}
	if b < 0x20 {
		return keyEvent{kind: keyIgnore}, nil
	}
	r, err := decodeUTF8Rune(in, b)
	if err != nil {
		return keyEvent{}, err
	}
	return keyEvent{kind: keyChar, ch: r}, nil
}

// decodeUTF8Rune reassembles one UTF-8 rune given its already-read lead
// byte, reading whatever continuation bytes that lead byte says follow.
func decodeUTF8Rune(in *bufio.Reader, lead byte) (rune, error) {
	n := 1
	switch {
	case lead&0x80 == 0:
		n = 1
	case lead&0xE0 == 0xC0:
		n = 2
	case lead&0xF0 == 0xE0:
		n = 3
	case lead&0xF8 == 0xF0:
		n = 4
	default:
		return utf8.RuneError, nil // stray continuation byte; ignore
	}
	buf := make([]byte, n)
	buf[0] = lead
	for i := 1; i < n; i++ {
		next, err := in.ReadByte()
		if err != nil {
			return 0, err
		}
		buf[i] = next
	}
	r, _ := utf8.DecodeRune(buf)
	return r, nil
}

// readEscapeSequence parses what follows a lone ESC (0x1b) byte: either a
// CSI sequence (`ESC [ ...`, the common xterm form) or an SS3 sequence
// (`ESC O ...`, used by some terminals/keypad modes for Home/End/arrows).
// A read deadline is armed on tty for the duration so a bare Escape
// keypress (no bytes follow) degrades to keyIgnore instead of hanging the
// whole prompt waiting for a byte that will never come.
//
// Terminal-compatibility caveat: this covers what xterm-family terminals
// (xterm, gnome-terminal/VTE, kitty, alacritty, foot, ...) send in their
// default (non-application) cursor-key mode. A terminal in application
// cursor mode, or an unusual/legacy emulator, may send sequences not
// listed here; those are parsed as far as recognized and otherwise
// reported as keyIgnore rather than corrupting the edit buffer.
func readEscapeSequence(in *bufio.Reader, tty *os.File) (keyEvent, error) {
	restore := armReadDeadline(tty, escapeTimeout)
	defer restore()

	b1, err := in.ReadByte()
	if err != nil {
		if isTimeout(err) {
			return keyEvent{kind: keyIgnore}, nil
		}
		return keyEvent{}, err
	}
	switch b1 {
	case '[':
		return readCSISequence(in)
	case 'O':
		b2, err := in.ReadByte()
		if err != nil {
			if isTimeout(err) {
				return keyEvent{kind: keyIgnore}, nil
			}
			return keyEvent{}, err
		}
		switch b2 {
		case 'H':
			return keyEvent{kind: keyHome}, nil
		case 'F':
			return keyEvent{kind: keyEnd}, nil
		case 'D':
			return keyEvent{kind: keyLeft}, nil
		case 'C':
			return keyEvent{kind: keyRight}, nil
		}
		return keyEvent{kind: keyIgnore}, nil
	}
	return keyEvent{kind: keyIgnore}, nil
}

// readCSISequence reads the parameter bytes (digits and `;`) of a `ESC [`
// sequence up to its final byte, and maps the well-known ones: plain
// arrows, Ctrl-arrows (xterm's `1;5` modifier parameter), Home/End in both
// their letter (`H`/`F`) and tilde (`1~`/`4~`/`7~`/`8~`) forms, and Delete
// (`3~`).
func readCSISequence(in *bufio.Reader) (keyEvent, error) {
	var params []byte
	for {
		b, err := in.ReadByte()
		if err != nil {
			return keyEvent{}, err
		}
		if (b >= '0' && b <= '9') || b == ';' {
			params = append(params, b)
			continue
		}
		ctrlMod := csiHasCtrlModifier(params)
		switch b {
		case 'D':
			if ctrlMod {
				return keyEvent{kind: keyWordLeft}, nil
			}
			return keyEvent{kind: keyLeft}, nil
		case 'C':
			if ctrlMod {
				return keyEvent{kind: keyWordRight}, nil
			}
			return keyEvent{kind: keyRight}, nil
		case 'H':
			return keyEvent{kind: keyHome}, nil
		case 'F':
			return keyEvent{kind: keyEnd}, nil
		case '~':
			switch string(params) {
			case "3":
				return keyEvent{kind: keyDelete}, nil
			case "1", "7":
				return keyEvent{kind: keyHome}, nil
			case "4", "8":
				return keyEvent{kind: keyEnd}, nil
			}
			return keyEvent{kind: keyIgnore}, nil
		}
		return keyEvent{kind: keyIgnore}, nil
	}
}

// csiHasCtrlModifier reports whether a CSI parameter string carries
// xterm's "modifier 5" (Ctrl) suffix, e.g. "1;5" in `ESC [ 1 ; 5 C`.
func csiHasCtrlModifier(params []byte) bool {
	s := string(params)
	for i := 0; i+1 < len(s); i++ {
		if s[i] == ';' && s[i+1] == '5' {
			return true
		}
	}
	return false
}

// armReadDeadline sets a read deadline on tty and returns a func that
// clears it again. If tty doesn't support deadlines (SetReadDeadline
// errors — not every terminal/PTY combination does), it's a silent no-op:
// escape-sequence reads simply block as they did before this feature,
// which only regresses the "bare Escape key" edge case, not normal use.
func armReadDeadline(tty *os.File, d time.Duration) (restore func()) {
	if tty == nil {
		return func() {}
	}
	if err := tty.SetReadDeadline(time.Now().Add(d)); err != nil {
		return func() {}
	}
	return func() { _ = tty.SetReadDeadline(time.Time{}) }
}

func isTimeout(err error) bool {
	var ne interface{ Timeout() bool }
	return errors.As(err, &ne) && ne.Timeout()
}
