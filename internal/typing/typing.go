package typing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/inputsource"
	"ubunatic.com/voxi/internal/modifiers"
)

var typingMu sync.Mutex

const sourceCacheTTL = 500 * time.Millisecond
const dotoolDeviceReadyTimeout = 5 * time.Second

var layoutCache struct {
	activeKey           string
	active              inputsource.Source
	activeErr           error
	activeAt            time.Time
	daemonKey           string
	daemon              inputsource.Source
	daemonOK            bool
	fallbackRestoredKey string
}

// LayoutStatus is the active desktop source and the layout currently used by dotoold.
type LayoutStatus struct {
	ActiveSource string
	Dotoold      string
	Fallback     bool
	Warning      string
}

// InjectionAttempt describes one and only one submission to an injector.
// PID is zero when the test/legacy dependency boundary cannot expose it.
type InjectionAttempt struct {
	Path      string
	PID       int
	StartedAt time.Time
	EndedAt   time.Time
	Err       error
}

// InjectionObserver receives lifecycle notifications around the irreversible
// injector call. Implementations must not block the typing operation.
type InjectionObserver interface {
	Started(path string, at time.Time)
	Completed(attempt InjectionAttempt)
}

// BuildDotoolCommands renders the dotool script command stream for typing text.
func BuildDotoolCommands(text string, typeDelayMs int) string {
	var b strings.Builder
	if typeDelayMs > 0 {
		fmt.Fprintf(&b, "typedelay %d\n", typeDelayMs)
		fmt.Fprintf(&b, "typehold %d\n", typeDelayMs)
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		fmt.Fprintf(&b, "type %s\n", line)
		if i < len(lines)-1 {
			b.WriteString("key enter\n")
		}
	}
	return b.String()
}

func dotoolPipePath(getenv func(string) string) string {
	if getenv != nil {
		if p := getenv("DOTOOL_PIPE"); p != "" {
			return p
		}
	}
	return "/tmp/dotool-pipe"
}

func dotoolDaemonReady(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		return false
	}
	fd, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return false
	}
	_ = fd.Close()
	return true
}

// TypeText synthesizes keystrokes into the focused window using dotool/dotoold.
// It gates on active modifier keys (e.g. Ctrl, Alt, Super) to prevent hotkey collisions.
func TypeText(ctx context.Context, d deps.Dependencies, text string) error {
	return TypeTextObserved(ctx, d, text, nil)
}

// TypeTextObserved is TypeText with an injectable lifecycle observer. The
// selected path is announced immediately before the single injector attempt;
// completion includes the child PID when the dependency boundary supports it.
func TypeTextObserved(ctx context.Context, d deps.Dependencies, text string, observer InjectionObserver) error {
	if text == "" {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	typingMu.Lock()
	defer typingMu.Unlock()

	// Gate on active physical modifier keys: wait up to 5s for user to release modifiers
	if reader := modifiers.NewModifierReader(""); reader != nil {
		_ = reader.WaitModifiersReleased(ctx, 5*time.Second)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	typeDelayMs := 0
	if home := d.Getenv("HOME"); home != "" {
		if ms, ok, err := config.ReadTypeDelayMs(config.VoxtypeConfigPath(home)); err == nil && ok {
			typeDelayMs = ms
		}
	}
	commands := BuildDotoolCommands(text, typeDelayMs)

	if dotoolDaemonReady(dotoolPipePath(d.Getenv)) {
		status, err := synchronizeDotoold(ctx, d)
		if err != nil {
			return err
		}
		if status.Warning != "" && d.Stderr != nil {
			fmt.Fprintf(d.Stderr, "voxi: warning: %s\n", status.Warning)
		}
		if !dotoolDaemonReady(dotoolPipePath(d.Getenv)) {
			return fmt.Errorf("dotoold FIFO is not ready after layout synchronization")
		}
		// Once a FIFO submission is attempted its partial-write status is
		// unknowable. Never retry the whole script through standalone dotool.
		return runInjector(ctx, d, commands, "dotoolc", observer)
	}
	if _, err := d.LookPath("dotool"); err != nil {
		return fmt.Errorf("dotool not found on PATH: %w", err)
	}
	return runInjector(ctx, d, commands, "dotool", observer)
}

func formatSource(source inputsource.Source) string {
	if source.Layout == "" {
		return "unknown"
	}
	if source.Variant == "" {
		return source.Layout
	}
	return source.Layout + "+" + source.Variant
}

func daemonSource(ctx context.Context, d deps.Dependencies) (inputsource.Source, bool) {
	key := cacheKey(d)
	if layoutCache.daemonKey == key && layoutCache.daemonOK {
		return layoutCache.daemon, true
	}
	source, ok := readDaemonSource(ctx, d)
	if ok {
		layoutCache.daemonKey, layoutCache.daemon, layoutCache.daemonOK = key, source, true
	}
	return source, ok
}

func readDaemonSource(ctx context.Context, d deps.Dependencies) (inputsource.Source, bool) {
	if d.RunOutput == nil {
		return inputsource.Source{}, false
	}
	value, err := d.RunOutput(ctx, "systemctl", "--user", "show", "dotoold.service", "--property=Environment", "--value")
	if err != nil {
		return inputsource.Source{}, false
	}
	source := inputsource.Source{}
	for _, token := range strings.Fields(value) {
		if strings.HasPrefix(token, "DOTOOL_XKB_LAYOUT=") {
			source.Layout = strings.TrimPrefix(token, "DOTOOL_XKB_LAYOUT=")
		}
		if strings.HasPrefix(token, "DOTOOL_XKB_VARIANT=") {
			source.Variant = strings.TrimPrefix(token, "DOTOOL_XKB_VARIANT=")
		}
	}
	if source.Layout == "" {
		return inputsource.Source{}, false
	}
	return source, true
}

func cacheKey(d deps.Dependencies) string {
	if d.Getenv != nil {
		return d.Getenv("HOME") + "\x00" + dotoolPipePath(d.Getenv)
	}
	return dotoolPipePath(nil)
}

func detectActiveCached(ctx context.Context, d deps.Dependencies) (inputsource.Source, error) {
	key := cacheKey(d)
	if layoutCache.activeKey == key && time.Since(layoutCache.activeAt) < sourceCacheTTL {
		return layoutCache.active, layoutCache.activeErr
	}
	source, err := inputsource.DetectActive(ctx, d)
	layoutCache.activeKey, layoutCache.active, layoutCache.activeErr, layoutCache.activeAt = key, source, err, time.Now()
	return source, err
}

func synchronizeDotoold(ctx context.Context, d deps.Dependencies) (LayoutStatus, error) {
	active, detectErr := detectActiveCached(ctx, d)
	return synchronizeDotooldToSource(ctx, d, active, detectErr)
}

func synchronizeDotooldToSource(ctx context.Context, d deps.Dependencies, active inputsource.Source, detectErr error) (LayoutStatus, error) {
	current, currentOK := daemonSource(ctx, d)
	status := LayoutStatus{ActiveSource: formatSource(active), Dotoold: formatSource(current)}
	if detectErr != nil {
		status.Fallback = true
		status.Warning = fmt.Sprintf("detect active keyboard layout: %v; restoring install-time dotoold layout", detectErr)
		key := cacheKey(d)
		if layoutCache.fallbackRestoredKey != key {
			if err := restoreInstallLayout(ctx, d); err != nil {
				status.Warning += fmt.Sprintf(" (restore failed: %v)", err)
				if strings.Contains(err.Error(), "GNOME did not open") {
					return status, err
				}
			} else {
				layoutCache.fallbackRestoredKey = key
			}
		}
		if restored, ok := readDaemonSource(ctx, d); ok {
			layoutCache.daemonKey, layoutCache.daemon, layoutCache.daemonOK = cacheKey(d), restored, true
			status.Dotoold = formatSource(restored)
		}
		return status, nil
	}
	if layoutCache.fallbackRestoredKey == cacheKey(d) {
		layoutCache.fallbackRestoredKey = ""
	}
	if !currentOK {
		status.Warning = "cannot read dotoold layout from systemd; using its configured install-time layout"
		return status, nil
	}
	if current != active {
		if err := restartDotooldForSource(ctx, d, active); err != nil {
			if strings.Contains(err.Error(), "GNOME did not open") {
				return status, err
			}
			status.Warning = fmt.Sprintf("update dotoold layout to %s: %v; continuing with current layout %s", formatSource(active), err, formatSource(current))
			return status, nil
		}
		layoutCache.daemonKey, layoutCache.daemon, layoutCache.daemonOK = cacheKey(d), active, true
		current = active
	}
	if layoutCache.fallbackRestoredKey == cacheKey(d) {
		layoutCache.fallbackRestoredKey = ""
	}
	status.Dotoold = formatSource(current)
	return status, nil
}

func restoreInstallLayout(ctx context.Context, d deps.Dependencies) error {
	if d.Getenv == nil || d.Getenv("HOME") == "" || d.Remove == nil || d.Run == nil {
		return fmt.Errorf("systemd layout dependencies are unavailable")
	}
	path := filepath.Join(d.Getenv("HOME"), ".config", "systemd", "user", "dotoold.service.d", "voxi-layout.conf")
	if err := d.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	baseline := snapshotDotoolKeyboardDevices()
	if err := d.Run(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	if err := d.Run(ctx, "systemctl", "--user", "restart", "dotoold.service"); err != nil {
		return err
	}
	if !waitForRestartReadyAfter(ctx, d, baseline, dotoolDeviceReadyTimeout) {
		return fmt.Errorf("GNOME did not open the restored dotoold keyboard within %s", dotoolDeviceReadyTimeout)
	}
	return nil
}

// InspectLayout reports source and daemon layout for the top-level status command.
func InspectLayout(ctx context.Context, d deps.Dependencies) LayoutStatus {
	typingMu.Lock()
	defer typingMu.Unlock()
	active, activeErr := detectActiveCached(ctx, d)
	daemon, ok := daemonSource(ctx, d)
	status := LayoutStatus{ActiveSource: formatSource(active), Dotoold: formatSource(daemon), Fallback: activeErr != nil}
	if activeErr != nil {
		status.Warning = fmt.Sprintf("active source detection failed: %v", activeErr)
	} else if !ok {
		status.Warning = "dotoold layout unavailable"
	}
	return status
}

// WatchInputSource prewarms dotoold when the active XKB source changes. It is
// intended to run for the lifetime of the agent; injection retains its own
// synchronization check as a safety net.
func WatchInputSource(ctx context.Context, d deps.Dependencies, interval time.Duration) {
	if d.Getenv != nil {
		desktop := strings.ToLower(d.Getenv("XDG_CURRENT_DESKTOP"))
		if desktop != "" && !strings.Contains(desktop, "gnome") {
			return
		}
	}
	if interval <= 0 {
		interval = 150 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var last inputsource.Source
	var lastValid bool
	lastFailed := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		typingMu.Lock()
		active, detectErr := inputsource.DetectActive(ctx, d)
		key := cacheKey(d)
		layoutCache.activeKey, layoutCache.active, layoutCache.activeErr, layoutCache.activeAt = key, active, detectErr, time.Now()
		changed := !lastValid || (detectErr == nil && active != last)
		if detectErr != nil && lastFailed {
			changed = false
		}
		if changed || (detectErr != nil && !lastFailed) {
			status, err := synchronizeDotooldToSource(ctx, d, active, detectErr)
			if status.Warning != "" && d.Stderr != nil {
				fmt.Fprintf(d.Stderr, "voxi: warning: %s\n", status.Warning)
			}
			if err != nil && d.Stderr != nil {
				fmt.Fprintf(d.Stderr, "voxi: warning: prewarm dotoold for %s: %v\n", formatSource(active), err)
			}
			if err == nil {
				last, lastValid, lastFailed = active, detectErr == nil, detectErr != nil
			}
		}
		typingMu.Unlock()
	}
}

func restartDotooldForSource(ctx context.Context, d deps.Dependencies, source inputsource.Source) error {
	if d.Getenv == nil || d.Getenv("HOME") == "" || d.MkdirAll == nil || d.WriteFile == nil || d.Run == nil {
		return fmt.Errorf("systemd layout dependencies are unavailable")
	}
	dropinDir := filepath.Join(d.Getenv("HOME"), ".config", "systemd", "user", "dotoold.service.d")
	if err := d.MkdirAll(dropinDir, 0755); err != nil {
		return err
	}
	variant := source.Variant
	data := fmt.Sprintf("[Service]\nEnvironment=DOTOOL_XKB_LAYOUT=%s\nEnvironment=DOTOOL_XKB_VARIANT=%s\n", source.Layout, variant)
	path := filepath.Join(dropinDir, "voxi-layout.conf")
	if err := d.WriteFile(path, []byte(data), 0644); err != nil {
		return err
	}
	baseline := snapshotDotoolKeyboardDevices()
	if err := d.Run(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	if err := d.Run(ctx, "systemctl", "--user", "restart", "dotoold.service"); err != nil {
		return err
	}
	if !waitForRestartReadyAfter(ctx, d, baseline, dotoolDeviceReadyTimeout) {
		return fmt.Errorf("GNOME did not open the new dotoold keyboard within %s", dotoolDeviceReadyTimeout)
	}
	return nil
}

func waitForRestartReadyAfter(ctx context.Context, d deps.Dependencies, baseline []string, timeout time.Duration) bool {
	if d.WaitInputDeviceReady != nil {
		return d.WaitInputDeviceReady(ctx, baseline, timeout) && waitForFIFO(ctx, d, timeout)
	}
	if !gnomeShellPresent() {
		return waitForFIFO(ctx, d, timeout)
	}
	return waitForGNOMEInputDevice(ctx, d, baseline, timeout)
}

func waitForFIFO(ctx context.Context, d deps.Dependencies, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if dotoolDaemonReady(dotoolPipePath(d.Getenv)) {
			return true
		}
		if ctx.Err() != nil || !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// snapshotDotoolKeyboardDevices records identities of existing dotool virtual keyboards.
func snapshotDotoolKeyboardDevices() []string {
	devices := dotoolKeyboardDevices()
	baseline := make([]string, 0, len(devices))
	for event, identity := range devices {
		baseline = append(baseline, event+"\x00"+identity)
	}
	sort.Strings(baseline)
	return baseline
}

func dotoolKeyboardDevices() map[string]string {
	devices := make(map[string]string)
	entries, err := os.ReadDir("/sys/class/input")
	if err != nil {
		return devices
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "event") {
			continue
		}
		base := filepath.Join("/sys/class/input", entry.Name())
		name, err := os.ReadFile(filepath.Join(base, "device", "name"))
		if err != nil || strings.TrimSpace(string(name)) != "dotool keyboard" {
			continue
		}
		deviceLink := filepath.Join(base, "device")
		identity, err := filepath.EvalSymlinks(deviceLink)
		if err != nil {
			identity, _ = os.Readlink(deviceLink)
		}
		devices[filepath.Join("/dev/input", entry.Name())] = identity
	}
	return devices
}

func waitForGNOMEInputDevice(ctx context.Context, d deps.Dependencies, baseline []string, timeout time.Duration) bool {
	old := make(map[string]string, len(baseline))
	for _, item := range baseline {
		parts := strings.SplitN(item, "\x00", 2)
		if len(parts) == 2 {
			old[parts[0]] = parts[1]
		}
	}
	deadline := time.Now().Add(timeout)
	for {
		current := dotoolKeyboardDevices()
		var added []string
		for event, identity := range current {
			if previous, exists := old[event]; !exists || previous != identity {
				added = append(added, event)
			}
		}
		if len(added) > 0 && gnomeShellHasOpened(added) && dotoolDaemonReady(dotoolPipePath(d.Getenv)) {
			return true
		}
		if ctx.Err() != nil || !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func gnomeShellPresent() bool {
	processes, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, process := range processes {
		if _, err := strconv.Atoi(process.Name()); err != nil {
			continue
		}
		comm, err := os.ReadFile(filepath.Join("/proc", process.Name(), "comm"))
		if err == nil && strings.TrimSpace(string(comm)) == "gnome-shell" {
			return true
		}
	}
	return false
}

func gnomeShellHasOpened(devices []string) bool {
	wanted := make(map[string]struct{}, len(devices))
	for _, device := range devices {
		wanted[device] = struct{}{}
	}
	processes, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, process := range processes {
		if _, err := strconv.Atoi(process.Name()); err != nil {
			continue
		}
		comm, err := os.ReadFile(filepath.Join("/proc", process.Name(), "comm"))
		if err != nil || strings.TrimSpace(string(comm)) != "gnome-shell" {
			continue
		}
		fds, err := os.ReadDir(filepath.Join("/proc", process.Name(), "fd"))
		if err != nil {
			continue
		}
		for _, fd := range fds {
			target, err := os.Readlink(filepath.Join("/proc", process.Name(), "fd", fd.Name()))
			if err == nil {
				if _, ok := wanted[target]; ok {
					return true
				}
			}
		}
	}
	return false
}

func runInjector(ctx context.Context, d deps.Dependencies, commands, name string, observer InjectionObserver) error {
	started := time.Now()
	if observer != nil {
		observer.Started(name, started)
	}
	pid := 0
	var err error
	if d.RunStdinProcess != nil {
		pid, err = d.RunStdinProcess(ctx, commands, name)
	} else if d.RunStdin != nil {
		err = d.RunStdin(ctx, commands, name)
	} else {
		err = fmt.Errorf("injector dependency is not configured")
	}
	ended := time.Now()
	if observer != nil {
		observer.Completed(InjectionAttempt{Path: name, PID: pid, StartedAt: started, EndedAt: ended, Err: err})
	}
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// CopyText copies text to the Wayland clipboard via wl-copy.
func CopyText(ctx context.Context, d deps.Dependencies, text string) error {
	if _, err := d.LookPath("wl-copy"); err != nil {
		return fmt.Errorf("wl-copy not found on PATH: %w", err)
	}
	if err := d.RunStdin(ctx, text, "wl-copy"); err != nil {
		return fmt.Errorf("wl-copy: %w", err)
	}
	return nil
}
