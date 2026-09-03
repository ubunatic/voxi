package chunks

import (
	"context"
	"fmt"
	"os"

	"ubunatic.com/voxi/internal/deps"
)

// PlayerCommand picks a standard local audio player.
func PlayerCommand(d deps.Dependencies) (name string, args func(wavPath string) []string, err error) {
	for _, candidate := range []struct {
		name string
		args func(string) []string
	}{
		{"pw-play", func(p string) []string { return []string{p} }},
		{"paplay", func(p string) []string { return []string{p} }},
		{"aplay", func(p string) []string { return []string{p} }},
		{"ffplay", func(p string) []string { return []string{"-nodisp", "-autoexit", "-loglevel", "quiet", p} }},
		{"play", func(p string) []string { return []string{"-q", p} }}, // sox
	} {
		if _, lookErr := d.LookPath(candidate.name); lookErr == nil {
			return candidate.name, candidate.args, nil
		}
	}
	return "", nil, fmt.Errorf("no local audio player found (looked for pw-play, paplay, aplay, ffplay, play)")
}

// Play plays the chunk's WAV audio using an available player tool.
func (b *Buffer) Play(ctx context.Context, d deps.Dependencies, selector string) error {
	chunk, err := b.Get(selector)
	if err != nil {
		return err
	}
	wavPath := b.WAVPath(chunk)
	if _, err := os.Stat(wavPath); err != nil {
		return fmt.Errorf("chunk audio file missing: %w", err)
	}

	playerName, playerArgs, err := PlayerCommand(d)
	if err != nil {
		return err
	}
	if d.Run == nil {
		return fmt.Errorf("no runner available to play %s", wavPath)
	}
	if err := d.Run(ctx, playerName, playerArgs(wavPath)...); err != nil {
		return fmt.Errorf("play chunk with %s: %w", playerName, err)
	}
	return nil
}
