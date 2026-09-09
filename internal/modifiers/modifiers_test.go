package modifiers

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestModifierMaskBitwise(t *testing.T) {
	var m ModifierMask

	if m.AnyActive() {
		t.Fatalf("expected empty mask to not be active")
	}

	m |= ModLeftCtrl
	if !m.AnyActive() {
		t.Fatalf("expected mask with ModLeftCtrl to be active")
	}
	if m&ModCtrl == 0 {
		t.Fatalf("expected m to match ModCtrl compound mask")
	}
	if m&ModAlt != 0 {
		t.Fatalf("expected m to not match ModAlt")
	}

	m |= ModRightAlt
	names := m.Names()
	if len(names) != 2 || names[0] != "LeftCtrl" || names[1] != "RightAlt" {
		t.Fatalf("unexpected names: %v", names)
	}

	allMods := []struct {
		mask ModifierMask
		name string
		code int
	}{
		{ModLeftCtrl, "LeftCtrl", keyLeftCtrl},
		{ModRightCtrl, "RightCtrl", keyRightCtrl},
		{ModLeftAlt, "LeftAlt", keyLeftAlt},
		{ModRightAlt, "RightAlt", keyRightAlt},
		{ModLeftSuper, "LeftSuper", keyLeftMeta},
		{ModRightSuper, "RightSuper", keyRightMeta},
		{ModLeftShift, "LeftShift", keyLeftShift},
		{ModRightShift, "RightShift", keyRightShift},
	}

	var combined ModifierMask
	for _, mod := range allMods {
		combined |= mod.mask
		if KeyCodeToModifierMask(mod.code) != mod.mask {
			t.Fatalf("code %d did not map to mask %d", mod.code, mod.mask)
		}
	}

	if combined != 0xFF {
		t.Fatalf("expected combined mask to be 0xFF, got 0x%02X", combined)
	}

	if KeyCodeToModifierMask(30 /* KEY_A */) != 0 {
		t.Fatalf("expected non-modifier key to return 0 mask")
	}
}

func TestModifierReader(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "modifiers")

	reader := NewModifierReader(stateFile)

	t.Run("state file absent returns inactive and no error", func(t *testing.T) {
		active, err := reader.AreModifiersActive()
		if err != nil {
			t.Fatalf("unexpected error on missing state file: %v", err)
		}
		if active {
			t.Fatalf("expected inactive when state file is missing")
		}
	})

	t.Run("empty state file returns inactive", func(t *testing.T) {
		if err := os.WriteFile(stateFile, []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
		active, err := reader.AreModifiersActive()
		if err != nil {
			t.Fatalf("unexpected error on empty state file: %v", err)
		}
		if active {
			t.Fatalf("expected inactive on empty state file")
		}
	})

	t.Run("state file with zero byte returns inactive", func(t *testing.T) {
		if err := os.WriteFile(stateFile, []byte{0}, 0644); err != nil {
			t.Fatal(err)
		}
		active, err := reader.AreModifiersActive()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if active {
			t.Fatalf("expected inactive on 0 byte")
		}
	})

	t.Run("state file with modifier byte returns active", func(t *testing.T) {
		if err := os.WriteFile(stateFile, []byte{byte(ModLeftCtrl | ModLeftAlt)}, 0644); err != nil {
			t.Fatal(err)
		}
		mask, err := reader.ReadMask()
		if err != nil {
			t.Fatalf("read mask error: %v", err)
		}
		if mask != (ModLeftCtrl | ModLeftAlt) {
			t.Fatalf("got mask %d, want %d", mask, ModLeftCtrl|ModLeftAlt)
		}
		active, err := reader.AreModifiersActive()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !active {
			t.Fatalf("expected active on non-zero modifier byte")
		}
	})

	t.Run("atomic write updates state file correctly", func(t *testing.T) {
		if err := WriteAtomicModifierState(stateFile, ModLeftSuper); err != nil {
			t.Fatalf("WriteAtomicModifierState failed: %v", err)
		}
		mask, err := reader.ReadMask()
		if err != nil {
			t.Fatalf("ReadMask failed: %v", err)
		}
		if mask != ModLeftSuper {
			t.Fatalf("got mask %d, want %d", mask, ModLeftSuper)
		}
	})

	t.Run("wait modifiers released unblocks when state becomes zero", func(t *testing.T) {
		if err := WriteAtomicModifierState(stateFile, ModLeftShift); err != nil {
			t.Fatal(err)
		}

		go func() {
			time.Sleep(30 * time.Millisecond)
			_ = WriteAtomicModifierState(stateFile, 0)
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		if err := reader.WaitModifiersReleased(ctx, 500*time.Millisecond); err != nil {
			t.Fatalf("WaitModifiersReleased failed: %v", err)
		}
	})

	t.Run("wait modifiers released times out if held", func(t *testing.T) {
		if err := WriteAtomicModifierState(stateFile, ModLeftCtrl); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		err := reader.WaitModifiersReleased(ctx, 50*time.Millisecond)
		if err == nil {
			t.Fatalf("expected timeout error, got nil")
		}
	})
}

// TestIsDotoolDeviceExcludesOwnSyntheticKeyboard is the issue 101 live-bug
// regression test: dotool's own virtual keyboard (voxi's synthetic typing
// injector, see internal/typing) was previously watched by
// FindPhysicalKeyboards alongside real keyboards, so dotool's own
// Shift-down/up events for typed capitals/symbols read back as a fresh
// physical modifier press -- a feedback loop where voxi's own typed output
// triggered the modifier-release buffering guard with no key ever touched.
func TestIsDotoolDeviceExcludesOwnSyntheticKeyboard(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"dotool keyboard", true},
		{"Dotool Keyboard", true},
		{"input-remapper keyboard", false},
		{"Fnatic Gear Fnatic Gear miniSTREAK Keyboard", false},
		{"Yubico YubiKey OTP+FIDO+CCID", false},
	}
	for _, c := range cases {
		if got := isDotoolDevice(c.name); got != c.want {
			t.Errorf("isDotoolDevice(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}
