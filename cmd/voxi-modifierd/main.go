package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/modifiers"
)

func main() {
	stateFile := flag.String("state-file", modifiers.DefaultModifierStatePath, "path to world-readable modifier state file")
	interval := flag.Duration("interval", 10*time.Millisecond, "polling interval for modifier checks")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	d := deps.DefaultDependencies(os.Stdin, os.Stdout)
	opts := modifiers.DaemonOptions{
		StateFile: *stateFile,
		Interval:  *interval,
	}

	if err := modifiers.RunModifierDaemon(ctx, d, opts); err != nil {
		fmt.Fprintf(os.Stderr, "voxi-modifierd error: %v\n", err)
		os.Exit(1)
	}
}
