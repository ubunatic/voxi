package typing

import (
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/modifiers"
)

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
	if text == "" {
		return nil
	}

	// Gate on active physical modifier keys: wait up to 5s for user to release modifiers
	if reader := modifiers.NewModifierReader(""); reader != nil {
		_ = reader.WaitModifiersReleased(ctx, 5*time.Second)
	}

	typeDelayMs := 0
	if home := d.Getenv("HOME"); home != "" {
		if ms, ok, err := config.ReadTypeDelayMs(config.VoxtypeConfigPath(home)); err == nil && ok {
			typeDelayMs = ms
		}
	}
	commands := BuildDotoolCommands(text, typeDelayMs)

	if dotoolDaemonReady(dotoolPipePath(d.Getenv)) {
		if err := d.RunStdin(ctx, commands, "dotoolc"); err == nil {
			return nil
		}
	}
	if _, err := d.LookPath("dotool"); err != nil {
		return fmt.Errorf("dotool not found on PATH: %w", err)
	}
	if err := d.RunStdin(ctx, commands, "dotool"); err != nil {
		return fmt.Errorf("dotool: %w", err)
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
