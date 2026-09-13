// SPDX-FileCopyrightText: 2026 Uwe Jugel
// SPDX-License-Identifier: AGPL-3.0-or-later

// Command miclevel demonstrates driving a loom TUI pane from voxi's
// audiolevel package against a real microphone: a scalar level meter and a
// scrolling Braille sparkline, both fed by one audiolevel.Manager.
package main

import (
	"context"
	"embed"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

//go:embed spec/*.yaml
var documents embed.FS

func main() {
	cmd := &cobra.Command{
		Use:           "miclevel",
		Short:         "Live microphone level meter and sparkline rendered via loom",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWatch(cmd.Context())
		},
	}
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
