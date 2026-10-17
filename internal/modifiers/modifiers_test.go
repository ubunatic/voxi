package modifiers

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/deps"
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

func TestModifierMaskShortNames(t *testing.T) {
	cases := []struct {
		mask ModifierMask
		want []string
	}{
		{0, nil},
		{ModLeftCtrl, []string{"Ctrl"}},
		{ModRightCtrl, []string{"Ctrl"}},
		{ModLeftSuper, []string{"Super"}},
		{ModLeftAlt, []string{"Alt"}},
		{ModRightShift, []string{"Shift"}},
		{ModLeftCtrl | ModRightSuper | ModLeftAlt | ModLeftShift, []string{"Ctrl", "Super", "Alt", "Shift"}},
	}

	for _, tc := range cases {
		got := tc.mask.ShortNames()
		if len(got) != len(tc.want) {
			t.Fatalf("mask 0x%02x: got %v, want %v", tc.mask, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("mask 0x%02x at index %d: got %s, want %s", tc.mask, i, got[i], tc.want[i])
			}
		}
	}
}

func TestResolveModifierStatePath(t *testing.T) {
	t.Run("default fallback when paths do not exist", func(t *testing.T) {
		got := resolveModifierStatePathWithStat(func(string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}, t.TempDir())
		if got != DefaultModifierStatePath {
			t.Fatalf("got %q, want %q", got, DefaultModifierStatePath)
		}
	})

	t.Run("resolves standard system path when present", func(t *testing.T) {
		got := resolveModifierStatePathWithStat(func(p string) (os.FileInfo, error) {
			if p == DefaultModifierStatePath {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}, t.TempDir())
		if got != DefaultModifierStatePath {
			t.Fatalf("got %q, want %q", got, DefaultModifierStatePath)
		}
	})

	t.Run("resolves user runtime path when present", func(t *testing.T) {
		runtimeDir := "/custom/run"
		userState := filepath.Join(runtimeDir, "voxi", "modifiers")
		got := resolveModifierStatePathWithStat(func(p string) (os.FileInfo, error) {
			if p == userState {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}, runtimeDir)
		if got != userState {
			t.Fatalf("got %q, want %q", got, userState)
		}
	})

	t.Run("resolves legacy user runtime path when present", func(t *testing.T) {
		runtimeDir := "/custom/run"
		legacyState := filepath.Join(runtimeDir, "harnez", "modifiers")
		got := resolveModifierStatePathWithStat(func(p string) (os.FileInfo, error) {
			if p == legacyState {
				return nil, nil
			}
			return nil, os.ErrNotExist
		}, runtimeDir)
		if got != legacyState {
			t.Fatalf("got %q, want %q", got, legacyState)
		}
	})
}

func TestIsBitSetInSlice(t *testing.T) {
	buf := []byte{0b00000001, 0b10000000} // bit 0 in byte 0, bit 7 in byte 1 (overall bit 15)

	if !isBitSetInSlice(buf, 0) {
		t.Error("expected bit 0 to be set")
	}
	if isBitSetInSlice(buf, 1) {
		t.Error("expected bit 1 to not be set")
	}
	if !isBitSetInSlice(buf, 15) {
		t.Error("expected bit 15 to be set")
	}
	if isBitSetInSlice(buf, 16) {
		t.Error("expected bit 16 out of bounds to return false")
	}
	if isBitSetInSlice(buf, 100) {
		t.Error("expected bit 100 out of bounds to return false")
	}
}

func TestDefaultDaemonOptionsAndCommand(t *testing.T) {
	opts := DefaultDaemonOptions()
	if opts.StateFile != DefaultModifierStatePath {
		t.Errorf("opts.StateFile = %q, want %q", opts.StateFile, DefaultModifierStatePath)
	}
	if opts.Interval != 10*time.Millisecond {
		t.Errorf("opts.Interval = %v, want 10ms", opts.Interval)
	}

	cmd := NewModifierDaemonCommand(deps.Dependencies{})
	if cmd.Use != "modifier-service" {
		t.Errorf("cmd.Use = %q, want modifier-service", cmd.Use)
	}
	stateFlag := cmd.Flag("state-file")
	if stateFlag == nil || stateFlag.DefValue != DefaultModifierStatePath {
		t.Errorf("missing or incorrect default for --state-file flag")
	}
	intervalFlag := cmd.Flag("interval")
	if intervalFlag == nil || intervalFlag.DefValue != "10ms" {
		t.Errorf("missing or incorrect default for --interval flag")
	}
}

func TestGlobalModifierFunctions(t *testing.T) {
	oldPath := defaultReader.Path
	defer func() {
		defaultReader.Path = oldPath
	}()

	tempState := filepath.Join(t.TempDir(), "modifiers")
	defaultReader.Path = tempState

	active, err := AreModifiersActive()
	if err != nil {
		t.Fatalf("AreModifiersActive error: %v", err)
	}
	if active {
		t.Error("expected inactive for nonexistent state file")
	}

	if err := WriteAtomicModifierState(tempState, ModLeftSuper); err != nil {
		t.Fatal(err)
	}

	active, err = AreModifiersActive()
	if err != nil {
		t.Fatalf("AreModifiersActive error: %v", err)
	}
	if !active {
		t.Error("expected active after writing ModLeftSuper")
	}

	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = WriteAtomicModifierState(tempState, 0)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	if err := WaitModifiersReleased(ctx, 200*time.Millisecond); err != nil {
		t.Fatalf("WaitModifiersReleased error: %v", err)
	}
}

func TestWriteAtomicModifierStateError(t *testing.T) {
	// A file where a directory is expected will cause MkdirAll to fail.
	regularFile := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(regularFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	invalidPath := filepath.Join(regularFile, "sub", "modifiers")
	if err := WriteAtomicModifierState(invalidPath, ModLeftCtrl); err == nil {
		t.Error("expected error writing to invalid directory path")
	}
}

func TestQueryDeviceModifiersInvalidFD(t *testing.T) {
	mask := QueryDeviceModifiers(-1)
	if mask != 0 {
		t.Fatalf("expected 0 mask on invalid fd, got %d", mask)
	}
}

func TestFindPhysicalKeyboards(t *testing.T) {
	keyboards, _, err := FindPhysicalKeyboards()
	if err != nil {
		t.Logf("FindPhysicalKeyboards error: %v", err)
		return
	}
	for _, k := range keyboards {
		if k.fd > 0 {
			_ = QueryDeviceModifiers(k.fd)
			_ = syscall.Close(k.fd)
		}
	}
}
