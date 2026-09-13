package settings

import (
	"io"
	"time"
)

// Key represents a user input key action in the settings TUI.
type Key int

const (
	KeyUnknown Key = iota
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyEnter
	KeySpace
	KeySave
	KeyQuit
)

// ParseKeySequence inspects a byte sequence and returns the recognized Key and bytes consumed.
func ParseKeySequence(b []byte) (Key, int) {
	if len(b) == 0 {
		return KeyUnknown, 0
	}

	// Escape sequences
	if b[0] == 0x1b {
		if len(b) == 1 {
			return KeyQuit, 1 // Standalone Esc
		}
		if len(b) >= 3 {
			if b[1] == '[' || b[1] == 'O' {
				switch b[2] {
				case 'A':
					return KeyUp, 3
				case 'B':
					return KeyDown, 3
				case 'C':
					return KeyRight, 3
				case 'D':
					return KeyLeft, 3
				case 'Z': // Shift-Tab
					return KeyUp, 3
				}
			}
			// PageUp / PageDown, e.g. \x1b[5~ or \x1b[6~
			if len(b) >= 4 && b[1] == '[' && b[3] == '~' {
				switch b[2] {
				case '5': // PageUp
					return KeyUp, 4
				case '6': // PageDown
					return KeyDown, 4
				}
			}
		}
		return KeyQuit, 1
	}

	switch b[0] {
	case '\r', '\n':
		return KeyEnter, 1
	case ' ':
		return KeySpace, 1
	case 'k', 'K':
		return KeyUp, 1
	case 'j', 'J', '\t':
		return KeyDown, 1
	case 'h', 'H', 0x7f, 0x08: // Backspace
		return KeyLeft, 1
	case 'l', 'L':
		return KeyRight, 1
	case 's', 'S':
		return KeySave, 1
	case 'q', 'Q', 0x03, 0x04: // Ctrl+C, Ctrl+D
		return KeyQuit, 1
	}

	return KeyUnknown, 1
}

// ReadKey reads one key event from reader r, resolving escape sequences with a short lookahead.
func ReadKey(r io.Reader) (Key, error) {
	var buf [16]byte
	n, err := r.Read(buf[:1])
	if err != nil {
		return KeyUnknown, err
	}
	if n == 0 {
		return KeyUnknown, nil
	}

	if buf[0] == 0x1b {
		// Escape character: try reading subsequent escape sequence bytes if ready
		type readResult struct {
			n   int
			err error
			buf [15]byte
		}
		ch := make(chan readResult, 1)
		go func() {
			var extra [15]byte
			en, eErr := r.Read(extra[:])
			ch <- readResult{n: en, err: eErr, buf: extra}
		}()

		select {
		case res := <-ch:
			if res.err == nil && res.n > 0 {
				full := append([]byte{0x1b}, res.buf[:res.n]...)
				k, _ := ParseKeySequence(full)
				return k, nil
			}
		case <-time.After(30 * time.Millisecond):
			// Standalone Esc pressed
			return KeyQuit, nil
		}
		return KeyQuit, nil
	}

	k, _ := ParseKeySequence(buf[:1])
	return k, nil
}
