package settings

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/deps"
)

// RunInteractive launches the raw terminal interactive settings TUI.
func RunInteractive(ctx context.Context, d deps.Dependencies, home string, asrModels []string) error {
	var inReader io.Reader = d.Stdin
	var fd int

	if f, ok := d.Stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fd = int(f.Fd())
		inReader = f
	} else {
		// Fallback to /dev/tty if available
		tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if err != nil {
			return fmt.Errorf("open /dev/tty for interactive mode: %w", err)
		}
		defer tty.Close()
		fd = int(tty.Fd())
		inReader = tty
	}

	initialSettings, err := config.LoadUserSettings(home)
	if err != nil {
		return fmt.Errorf("load initial settings: %w", err)
	}

	model := NewMenuModel(initialSettings, asrModels)

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("enter raw terminal mode: %w", err)
	}
	defer func() {
		_ = term.Restore(fd, oldState)
		fmt.Fprint(d.Stdout, "\033[?25h\033[?1049l")
	}()

	// Switch to alternate screen buffer, hide cursor, and clear screen
	fmt.Fprint(d.Stdout, "\033[?1049h\033[?25l\033[2J\033[H")

	getTermWidth := func() int {
		if outF, ok := d.Stdout.(*os.File); ok {
			if w, _, err := term.GetSize(int(outF.Fd())); err == nil && w > 0 {
				return w
			}
		}
		return 72
	}

	render := func() {
		width := getTermWidth()
		frame := RenderMenu(model, width)
		crlfFrame := strings.ReplaceAll(strings.ReplaceAll(frame, "\r\n", "\n"), "\n", "\r\n")
		fmt.Fprint(d.Stdout, "\033[H\033[2J"+crlfFrame)
	}

	render()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		key, err := ReadKey(inReader)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read input: %w", err)
		}

		model.HandleKey(key)

		if model.Saved {
			updated := model.ToUserSettings(initialSettings)
			// Restore terminal before printing success message
			_ = term.Restore(fd, oldState)
			fmt.Fprint(d.Stdout, "\033[?25h\033[?1049l")

			if err := config.SaveUserSettings(home, updated); err != nil {
				return fmt.Errorf("save user settings: %w", err)
			}
			fmt.Fprint(d.Stdout, RenderSaveSuccess(home))
			return nil
		}

		if model.Closed {
			_ = term.Restore(fd, oldState)
			fmt.Fprint(d.Stdout, "\033[?25h\033[?1049l")
			return nil
		}

		render()
	}
}
