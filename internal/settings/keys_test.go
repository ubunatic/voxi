package settings

import (
	"bytes"
	"testing"
)

func TestParseKeySequence(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		wantKey  Key
		wantCons int
	}{
		{"empty", []byte{}, KeyUnknown, 0},
		{"standalone escape", []byte{0x1b}, KeyQuit, 1},
		{"arrow up ANSI", []byte{0x1b, '[', 'A'}, KeyUp, 3},
		{"arrow down ANSI", []byte{0x1b, '[', 'B'}, KeyDown, 3},
		{"arrow right ANSI", []byte{0x1b, '[', 'C'}, KeyRight, 3},
		{"arrow left ANSI", []byte{0x1b, '[', 'D'}, KeyLeft, 3},
		{"arrow up SS3", []byte{0x1b, 'O', 'A'}, KeyUp, 3},
		{"arrow down SS3", []byte{0x1b, 'O', 'B'}, KeyDown, 3},
		{"arrow right SS3", []byte{0x1b, 'O', 'C'}, KeyRight, 3},
		{"arrow left SS3", []byte{0x1b, 'O', 'D'}, KeyLeft, 3},
		{"shift tab", []byte{0x1b, '[', 'Z'}, KeyUp, 3},
		{"page up", []byte{0x1b, '[', '5', '~'}, KeyUp, 4},
		{"page down", []byte{0x1b, '[', '6', '~'}, KeyDown, 4},
		{"k up", []byte{'k'}, KeyUp, 1},
		{"K up", []byte{'K'}, KeyUp, 1},
		{"j down", []byte{'j'}, KeyDown, 1},
		{"J down", []byte{'J'}, KeyDown, 1},
		{"tab down", []byte{'\t'}, KeyDown, 1},
		{"h left", []byte{'h'}, KeyLeft, 1},
		{"H left", []byte{'H'}, KeyLeft, 1},
		{"backspace 0x7f", []byte{0x7f}, KeyLeft, 1},
		{"backspace 0x08", []byte{0x08}, KeyLeft, 1},
		{"l right", []byte{'l'}, KeyRight, 1},
		{"L right", []byte{'L'}, KeyRight, 1},
		{"space", []byte{' '}, KeySpace, 1},
		{"enter LF", []byte{'\n'}, KeyEnter, 1},
		{"enter CR", []byte{'\r'}, KeyEnter, 1},
		{"s save", []byte{'s'}, KeySave, 1},
		{"S save", []byte{'S'}, KeySave, 1},
		{"q quit", []byte{'q'}, KeyQuit, 1},
		{"Q quit", []byte{'Q'}, KeyQuit, 1},
		{"ctrl c", []byte{0x03}, KeyQuit, 1},
		{"ctrl d", []byte{0x04}, KeyQuit, 1},
		{"unknown char", []byte{'z'}, KeyUnknown, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKey, gotCons := ParseKeySequence(tt.input)
			if gotKey != tt.wantKey {
				t.Errorf("ParseKeySequence(%v) key = %v, want %v", tt.input, gotKey, tt.wantKey)
			}
			if gotCons != tt.wantCons {
				t.Errorf("ParseKeySequence(%v) consumed = %v, want %v", tt.input, gotCons, tt.wantCons)
			}
		})
	}
}

func TestReadKey(t *testing.T) {
	// 1. Single character read
	r := bytes.NewReader([]byte{'s', 'j', '\n'})
	k1, err := ReadKey(r)
	if err != nil || k1 != KeySave {
		t.Fatalf("k1 = %v, err = %v, want KeySave", k1, err)
	}
	k2, err := ReadKey(r)
	if err != nil || k2 != KeyDown {
		t.Fatalf("k2 = %v, err = %v, want KeyDown", k2, err)
	}
	k3, err := ReadKey(r)
	if err != nil || k3 != KeyEnter {
		t.Fatalf("k3 = %v, err = %v, want KeyEnter", k3, err)
	}
}
