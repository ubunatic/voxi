package modifiers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/deps"
)

// Modifier bitmask representation.
// Bit 0: Left Ctrl   (1 << 0 = 0x01)
// Bit 1: Right Ctrl  (1 << 1 = 0x02)
// Bit 2: Left Alt    (1 << 2 = 0x04)
// Bit 3: Right Alt   (1 << 3 = 0x08)
// Bit 4: Left Super  (1 << 4 = 0x10)
// Bit 5: Right Super (1 << 5 = 0x20)
// Bit 6: Left Shift  (1 << 6 = 0x40)
// Bit 7: Right Shift (1 << 7 = 0x80)
type ModifierMask uint8

const (
	ModLeftCtrl   ModifierMask = 1 << 0 // 0x01
	ModRightCtrl  ModifierMask = 1 << 1 // 0x02
	ModLeftAlt    ModifierMask = 1 << 2 // 0x04
	ModRightAlt   ModifierMask = 1 << 3 // 0x08
	ModLeftSuper  ModifierMask = 1 << 4 // 0x10
	ModRightSuper ModifierMask = 1 << 5 // 0x20
	ModLeftShift  ModifierMask = 1 << 6 // 0x40
	ModRightShift ModifierMask = 1 << 7 // 0x80

	ModCtrl  ModifierMask = ModLeftCtrl | ModRightCtrl
	ModAlt   ModifierMask = ModLeftAlt | ModRightAlt
	ModSuper ModifierMask = ModLeftSuper | ModRightSuper
	ModShift ModifierMask = ModLeftShift | ModRightShift
	ModAny   ModifierMask = 0xFF
)

// Linux evdev keycodes from <linux/input-event-codes.h>.
const (
	evKey = 0x01

	keyLeftCtrl   = 29
	keyRightCtrl  = 97
	keyLeftShift  = 42
	keyRightShift = 54
	keyLeftAlt    = 56
	keyRightAlt   = 100
	keyLeftMeta   = 125 // Super / Windows key
	keyRightMeta  = 126
	keyCapsLock   = 58

	keyMax = 0x2ff // 767
)

// ModifierKeyDef maps an evdev keycode to a modifier bitmask and human-readable name.
type ModifierKeyDef struct {
	Code int
	Mask ModifierMask
	Name string
}

// WatchedModifierKeys defines all physical modifiers tracked by the daemon.
var WatchedModifierKeys = []ModifierKeyDef{
	{Code: keyLeftCtrl, Mask: ModLeftCtrl, Name: "LeftCtrl"},
	{Code: keyRightCtrl, Mask: ModRightCtrl, Name: "RightCtrl"},
	{Code: keyLeftAlt, Mask: ModLeftAlt, Name: "LeftAlt"},
	{Code: keyRightAlt, Mask: ModRightAlt, Name: "RightAlt"},
	{Code: keyLeftMeta, Mask: ModLeftSuper, Name: "LeftSuper"},
	{Code: keyRightMeta, Mask: ModRightSuper, Name: "RightSuper"},
	{Code: keyLeftShift, Mask: ModLeftShift, Name: "LeftShift"},
	{Code: keyRightShift, Mask: ModRightShift, Name: "RightShift"},
}

// KeyCodeToModifierMask returns the ModifierMask bit corresponding to an evdev keycode, or 0 if not a modifier.
func KeyCodeToModifierMask(code int) ModifierMask {
	for _, def := range WatchedModifierKeys {
		if def.Code == code {
			return def.Mask
		}
	}
	return 0
}

// Names returns a list of human-readable modifier names active in the mask.
func (m ModifierMask) Names() []string {
	var names []string
	for _, def := range WatchedModifierKeys {
		if m&def.Mask != 0 {
			names = append(names, def.Name)
		}
	}
	return names
}

// ShortNames returns concise, deduplicated modifier names (e.g. "Ctrl", "Super", "Alt", "Shift").
func (m ModifierMask) ShortNames() []string {
	var names []string
	if m&ModCtrl != 0 {
		names = append(names, "Ctrl")
	}
	if m&ModSuper != 0 {
		names = append(names, "Super")
	}
	if m&ModAlt != 0 {
		names = append(names, "Alt")
	}
	if m&ModShift != 0 {
		names = append(names, "Shift")
	}
	return names
}

// AnyActive returns true if any modifier bit is set.
func (m ModifierMask) AnyActive() bool {
	return m != 0
}

// DefaultModifierStatePath returns the standard world-readable runtime state file path.
const DefaultModifierStatePath = "/run/voxi/modifiers"

// ModifierReader reads instantaneous modifier key state from a shared memory/file state.
type ModifierReader struct {
	Path string
}

// NewModifierReader creates a new reader for the specified modifier state file path.
// If path is empty, ResolveModifierStatePath is used.
func NewModifierReader(path string) *ModifierReader {
	if path == "" {
		path = ResolveModifierStatePath("")
	}
	return &ModifierReader{Path: path}
}

// ResolveModifierStatePath finds the active or default modifier state file path.
func ResolveModifierStatePath(envRuntimeDir string) string {
	// First preference: standard system path /run/voxi/modifiers
	if _, err := os.Stat(DefaultModifierStatePath); err == nil {
		return DefaultModifierStatePath
	}

	// Legacy preference: /run/harnez/modifiers
	legacySysPath := "/run/harnez/modifiers"
	if _, err := os.Stat(legacySysPath); err == nil {
		return legacySysPath
	}

	// User runtime directory fallback
	runtimeDir := envRuntimeDir
	if runtimeDir == "" {
		runtimeDir = os.Getenv("XDG_RUNTIME_DIR")
	}
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	userPath := filepath.Join(runtimeDir, "voxi", "modifiers")
	if _, err := os.Stat(userPath); err == nil {
		return userPath
	}
	legacyUserPath := filepath.Join(runtimeDir, "harnez", "modifiers")
	if _, err := os.Stat(legacyUserPath); err == nil {
		return legacyUserPath
	}

	return DefaultModifierStatePath
}

// ReadMask reads the single-byte instantaneous modifier mask.
func (r *ModifierReader) ReadMask() (ModifierMask, error) {
	data, err := os.ReadFile(r.Path)
	if err != nil {
		if os.IsNotExist(err) || os.IsPermission(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read modifier state %s: %w", r.Path, err)
	}
	if len(data) == 0 {
		return 0, nil
	}
	return ModifierMask(data[0]), nil
}

// AreModifiersActive returns true if any physical modifier keys are currently depressed.
func (r *ModifierReader) AreModifiersActive() (bool, error) {
	mask, err := r.ReadMask()
	if err != nil {
		return false, err
	}
	return mask.AnyActive(), nil
}

// WaitModifiersReleased blocks until all physical modifier keys are released,
// or until the context is cancelled / timeout expires.
func (r *ModifierReader) WaitModifiersReleased(ctx context.Context, timeout time.Duration) error {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		active, err := r.AreModifiersActive()
		if err != nil || !active {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Global default modifier reader instance for convenience.
var defaultReader = NewModifierReader("")

// AreModifiersActive returns whether any modifiers are active using the default reader.
func AreModifiersActive() (bool, error) {
	return defaultReader.AreModifiersActive()
}

// WaitModifiersReleased waits for modifier release using the default reader.
func WaitModifiersReleased(ctx context.Context, timeout time.Duration) error {
	return defaultReader.WaitModifiersReleased(ctx, timeout)
}

// DaemonOptions configures the modifier daemon.
type DaemonOptions struct {
	StateFile string
	Interval  time.Duration
}

// DefaultDaemonOptions returns standard daemon defaults.
func DefaultDaemonOptions() DaemonOptions {
	return DaemonOptions{
		StateFile: DefaultModifierStatePath,
		Interval:  10 * time.Millisecond,
	}
}

// NewModifierDaemonCommand returns the `voxi daemon modifier-service` command.
func NewModifierDaemonCommand(d deps.Dependencies) *cobra.Command {
	opts := DefaultDaemonOptions()

	cmd := &cobra.Command{
		Use:   "modifier-service",
		Short: "Dedicated evdev modifier key state monitoring daemon",
		Long: "Monitors physical modifier keys (Ctrl, Alt, Super, Shift) across all /dev/input/event* keyboards,\n" +
			"and exports an atomic 1-byte bitmask state to a world-readable file (/run/voxi/modifiers).\n" +
			"Never captures or records non-modifier keycodes, preserving strict privacy.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunModifierDaemon(cmd.Context(), d, opts)
		},
	}

	cmd.Flags().StringVar(&opts.StateFile, "state-file", opts.StateFile, "path to world-readable modifier state file (default: /run/voxi/modifiers)")
	cmd.Flags().DurationVar(&opts.Interval, "interval", opts.Interval, "polling and update interval (default: 10ms)")

	return cmd
}

// evdev ioctl helpers
func evdevIoctl(fd int, req uintptr, arg unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, uintptr(arg))
	if errno != 0 {
		return errno
	}
	return nil
}

func evdevEviocgbit(ev int, length int) uintptr {
	const iocRead = 2
	return uintptr((iocRead << 30) | (int('E') << 8) | (0x20 + ev) | (length << 16))
}

func evdevEviocgkey(length int) uintptr {
	const iocRead = 2
	return uintptr((iocRead << 30) | (int('E') << 8) | 0x18 | (length << 16))
}

func evdevEviocgname(length int) uintptr {
	const iocRead = 2
	return uintptr((iocRead << 30) | (int('E') << 8) | 0x06 | (length << 16))
}

type evdevKeyboardDevice struct {
	path string
	name string
	fd   int
}

func isBitSetInSlice(buf []byte, bit int) bool {
	byteIdx := bit / 8
	bitIdx := bit % 8
	if byteIdx >= len(buf) {
		return false
	}
	return (buf[byteIdx] & (1 << bitIdx)) != 0
}

// isDotoolDevice reports whether an evdev device name looks like dotool's
// own synthetic virtual keyboard, which "physical" modifier gating must
// never watch (see the FindPhysicalKeyboards call site). Matches by
// substring, case-insensitive, rather than an exact "dotool keyboard"
// string, since dotool versions/configurations can vary the exact name.
func isDotoolDevice(name string) bool {
	return strings.Contains(strings.ToLower(name), "dotool")
}

// FindPhysicalKeyboards scans /dev/input/event* and filters for physical keyboards via EVIOCGBIT.
func FindPhysicalKeyboards() ([]*evdevKeyboardDevice, []string, error) {
	matches, err := filepath.Glob("/dev/input/event*")
	if err != nil {
		return nil, nil, err
	}

	var keyboards []*evdevKeyboardDevice
	var permissionDenied []string

	for _, path := range matches {
		fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			if os.IsPermission(err) || err == syscall.EACCES {
				permissionDenied = append(permissionDenied, path)
			}
			continue
		}

		var keyBits [(keyMax + 7) / 8]byte
		if err := evdevIoctl(fd, evdevEviocgbit(evKey, len(keyBits)), unsafe.Pointer(&keyBits[0])); err != nil {
			_ = syscall.Close(fd)
			continue
		}

		if isBitSetInSlice(keyBits[:], keyLeftCtrl) && isBitSetInSlice(keyBits[:], keyLeftAlt) && isBitSetInSlice(keyBits[:], 30 /* KEY_A */) {
			var nameBuf [256]byte
			name := "Keyboard"
			if err := evdevIoctl(fd, evdevEviocgname(len(nameBuf)), unsafe.Pointer(&nameBuf[0])); err == nil {
				name = string(bytes.TrimRight(nameBuf[:], "\x00"))
			}
			if isDotoolDevice(name) {
				// dotool is voxi's own synthetic typing-injection tool
				// (see internal/typing) -- its virtual keyboard reports its
				// own key-down/key-up events, including Shift for typed
				// capitals/symbols. Treating that as a physical modifier
				// press creates a feedback loop where voxi's own typed
				// output looks like a fresh gating-modifier press to
				// itself (issue 101 live verification: every chunk after
				// one containing a capital letter wrongly entered
				// buffering with no physical key ever touched). "Physical"
				// modifier gating must exclude devices voxi itself drives.
				_ = syscall.Close(fd)
				continue
			}
			keyboards = append(keyboards, &evdevKeyboardDevice{
				path: path,
				name: name,
				fd:   fd,
			})
		} else {
			_ = syscall.Close(fd)
		}
	}

	sort.Slice(keyboards, func(i, j int) bool {
		return keyboards[i].path < keyboards[j].path
	})

	return keyboards, permissionDenied, nil
}

// QueryDeviceModifiers reads only modifier key state from an open keyboard device.
func QueryDeviceModifiers(fd int) ModifierMask {
	var keyState [(keyMax + 7) / 8]byte
	if err := evdevIoctl(fd, evdevEviocgkey(len(keyState)), unsafe.Pointer(&keyState[0])); err != nil {
		return 0
	}

	var mask ModifierMask
	for _, def := range WatchedModifierKeys {
		if isBitSetInSlice(keyState[:], def.Code) {
			mask |= def.Mask
		}
	}
	return mask
}

// WriteAtomicModifierState updates the state file atomically with world-readable (0644) permissions.
func WriteAtomicModifierState(path string, mask ModifierMask) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create state directory %s: %w", dir, err)
	}

	tmpFile := filepath.Join(dir, fmt.Sprintf(".modifiers-%d.tmp", os.Getpid()))
	data := []byte{byte(mask)}

	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("write temp state: %w", err)
	}

	if err := os.Rename(tmpFile, path); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("rename state file: %w", err)
	}

	return nil
}

// RunModifierDaemon runs the modifier monitoring event loop.
func RunModifierDaemon(ctx context.Context, d deps.Dependencies, opts DaemonOptions) error {
	keyboards, denied, err := FindPhysicalKeyboards()
	if err != nil {
		return fmt.Errorf("scan /dev/input: %w", err)
	}

	if len(keyboards) == 0 {
		if len(denied) > 0 {
			return fmt.Errorf("permission denied opening %d /dev/input devices (needs input group or root)", len(denied))
		}
		return fmt.Errorf("no physical keyboard devices found in /dev/input")
	}

	defer func() {
		for _, k := range keyboards {
			_ = syscall.Close(k.fd)
		}
	}()

	fmt.Fprintf(d.Stdout, "voxi-modifierd started. Monitoring %d keyboard device(s):\n", len(keyboards))
	for _, k := range keyboards {
		fmt.Fprintf(d.Stdout, "  • %s (%s)\n", k.path, k.name)
	}
	fmt.Fprintf(d.Stdout, "State output: %s\n", opts.StateFile)

	defer func() {
		_ = WriteAtomicModifierState(opts.StateFile, 0)
	}()

	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()

	var lastMask ModifierMask = 0xFF // Force initial write

	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(d.Stdout, "voxi-modifierd stopping...")
			return nil
		case <-ticker.C:
			var currentMask ModifierMask
			for _, k := range keyboards {
				currentMask |= QueryDeviceModifiers(k.fd)
			}

			if currentMask != lastMask {
				if err := WriteAtomicModifierState(opts.StateFile, currentMask); err != nil {
					fmt.Fprintf(d.Stdout, "Error writing state: %v\n", err)
				}
				lastMask = currentMask
			}
		}
	}
}
