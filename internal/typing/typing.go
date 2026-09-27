package typing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	if p := getenv("DOTOOL_PIPE"); p != "" {
		return p
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
		if active, err := inputsource.DetectActive(ctx, d); err == nil {
			if current, ok := daemonSource(ctx, d); ok && current != active {
				if err := restartDotooldForSource(ctx, d, active); err != nil {
					fmt.Fprintf(d.Stderr, "voxi: warning: update dotoold layout to %s: %v; continuing with current layout %s\n", formatSource(active), err, formatSource(current))
				}
			}
		} else {
			current, _ := daemonSource(ctx, d)
			fmt.Fprintf(d.Stderr, "voxi: warning: detect active keyboard layout: %v; continuing with dotoold layout %s\n", err, formatSource(current))
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
	if err := d.Run(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	return d.Run(ctx, "systemctl", "--user", "restart", "dotoold.service")
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
