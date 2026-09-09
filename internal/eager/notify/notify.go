// Package notify plays voxi's fixed audio notification clips: non-injection
// feedback that never types into the focused window (issue 101). English
// only for now; multi-language support is deferred (issue 102).
package notify

import (
	"context"
	_ "embed"
	"fmt"
	"os"

	"ubunatic.com/voxi/internal/chunks"
	"ubunatic.com/voxi/internal/deps"
)

//go:embed typing-paused.wav
var typingPausedWAV []byte

// PlayTypingPaused plays a short pre-recorded clip ("Typing paused. Stop
// recording to finish.") via whatever local audio player is available (see
// chunks.PlayerCommand), so the user learns eager output is being buffered
// without anything being typed into an unknown-focus window.
func PlayTypingPaused(ctx context.Context, d deps.Dependencies) error {
	return playClip(ctx, d, typingPausedWAV)
}

func playClip(ctx context.Context, d deps.Dependencies, wav []byte) error {
	if d.Run == nil {
		return fmt.Errorf("no runner available to play notification clip")
	}
	tmp, err := os.CreateTemp("", "voxi-notify-*.wav")
	if err != nil {
		return fmt.Errorf("notify: create temp wav: %w", err)
	}
	path := tmp.Name()
	defer os.Remove(path)
	if _, err := tmp.Write(wav); err != nil {
		tmp.Close()
		return fmt.Errorf("notify: write temp wav: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("notify: close temp wav: %w", err)
	}

	playerName, playerArgs, err := chunks.PlayerCommand(d)
	if err != nil {
		return err
	}
	if err := d.Run(ctx, playerName, playerArgs(path)...); err != nil {
		return fmt.Errorf("notify: play clip with %s: %w", playerName, err)
	}
	return nil
}
