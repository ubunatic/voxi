//go:build !debug

package main

import (
	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/deps"
)

func addDebugCommands(root *cobra.Command, d deps.Dependencies) {
	// Debug canary and VAD probe excluded in release builds.
}
