// SPDX-FileCopyrightText: 2026 Uwe Jugel
// SPDX-License-Identifier: AGPL-3.0-or-later

// Command voxi-tui is a terminal front end for voxi: speech input and output,
// model servers (lmcoder and voxi-owned units) and settings, drawn with loom.
// It reads the voxi agent, TTS, settings and lmcoder through internal/frontend,
// and runs a state-changing action only on an explicit key press.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ubunatic.com/loom"
	"ubunatic.com/voxi/internal/frontend"
)

// refreshEvery is how often the status is re-read while the TUI is open.
const refreshEvery = 5 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "voxi-tui:", err)
		os.Exit(1)
	}
}

func run() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pane, err := loom.New(screenHeight + noteRows)
	if err != nil {
		return fmt.Errorf("needs an interactive terminal: %w", err)
	}
	defer pane.Close()
	pane.SetScreenMode(loom.ScreenAlt)
	pane.Resizeable = true
	// loom caps the canvas at its default MaxCols (50); the tabs are up to 78 wide.
	pane.MaxCols = 0

	app := newUI(frontend.NewDeps(home), pane.Invalidate)
	app.refresh(ctx)
	go refreshLoop(ctx, app)

	err = pane.RunWatch(ctx, app, loom.Cadence{Collect: time.Second, Redraw: time.Second}, func(time.Time) error {
		return nil
	})
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// refreshLoop re-reads the status every refreshEvery until ctx ends.
func refreshLoop(ctx context.Context, app *ui) {
	ticker := time.NewTicker(refreshEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			app.refresh(ctx)
		}
	}
}
