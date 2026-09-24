// Measures time until the first player process starts without emitting audio.
// Run with: go run ./scripts/canary_tts/first_audio
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/tts"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "__tts-supervise" {
		if err := tts.RunSupervisor(os.Args[3:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "FAIL:", err)
		os.Exit(1)
	}
}

func run() error {
	tempDir, err := os.MkdirTemp("", "voxi-tts-first-audio-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	playerPath := filepath.Join(tempDir, "pw-play")
	if err := os.WriteFile(playerPath, []byte("#!/bin/sh\nsleep 30\n"), 0700); err != nil {
		return err
	}
	if err := os.Setenv("PATH", tempDir+":"+os.Getenv("PATH")); err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	d := deps.DefaultDependencies(os.Stdin, os.Stdout)
	engine := tts.NewEngine(d, executable)
	reply := "The first paragraph introduces the report and its main result. It gives the listener useful context.\n\nThe second paragraph describes the implementation and its tradeoffs. It ends with a practical next step."
	chunks := tts.SplitText(reply)
	if len(chunks) < 2 {
		return fmt.Errorf("sample reply split into %d chunks, want multiple paragraphs", len(chunks))
	}
	started := time.Now()
	first, _, err := engine.Synthesize(context.Background(), chunks[0])
	if err != nil {
		return err
	}
	defer first.Close()
	player, err := engine.StartPlayback(context.Background(), first.Path())
	if err != nil {
		return err
	}
	defer player.Stop()
	timeToFirstPlayer := time.Since(started)
	nextDone := make(chan error, 1)
	go func() {
		next, _, synthErr := engine.Synthesize(context.Background(), chunks[1])
		if synthErr == nil {
			_ = next.Close()
		}
		nextDone <- synthErr
	}()
	select {
	case err := <-nextDone:
		if err != nil {
			return err
		}
	case <-time.After(20 * time.Second):
		return fmt.Errorf("second chunk synthesis timed out")
	}
	select {
	case err := <-player.Done():
		return fmt.Errorf("fake player ended before prefetch completed: %v", err)
	default:
	}
	fmt.Printf("time to first playable WAV and player start: %s\n", timeToFirstPlayer.Round(time.Millisecond))
	fmt.Println("next paragraph synthesized while the first player process was active; fake player emitted no audio")
	return nil
}
