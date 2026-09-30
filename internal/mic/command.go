package mic

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/deps"
)

// NewCommand creates `voxi mic`: show the mic setup and, with --fix, repair it
// the same way a recording start does (issue 177).
func NewCommand(d deps.Dependencies, settle time.Duration) *cobra.Command {
	var fix bool
	cmd := &cobra.Command{
		Use:   "mic",
		Short: "Check the PipeWire mic setup and optionally repair it",
		Long: "Show the default mic and any problems with it: no usable default, a saved default that is not connected, " +
			"or a Bluetooth headphone in playback-only mode (A2DP) with no mic.\n\n" +
			"With --fix, clear the stale saved default, switch connected Bluetooth headphones to headset mode, " +
			"and fall back to a built-in mic. Voxi runs the same check alongside every recording.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			doc := NewDoctor(d, settle)
			st, err := doc.Probe(ctx)
			if err != nil {
				return err
			}
			fmt.Fprintf(d.Stdout, "Default mic: %s\n", describe(st, st.DefaultSource))
			if st.ConfiguredSource != "" && st.ConfiguredSource != st.DefaultSource {
				fmt.Fprintf(d.Stdout, "Saved mic:   %s\n", describe(st, st.ConfiguredSource))
			}
			problems := Diagnose(st)
			if len(problems) == 0 {
				fmt.Fprintln(d.Stdout, "No problems found.")
				return nil
			}
			for _, p := range problems {
				fmt.Fprintf(d.Stdout, "Problem:     %s\n", p.Detail)
			}
			if !fix {
				for _, a := range Plan(st, problems) {
					fmt.Fprintf(d.Stdout, "Would fix:   %s\n", a.Summary)
				}
				return nil
			}
			rep, err := doc.Check(ctx)
			for _, a := range rep.Applied {
				fmt.Fprintf(d.Stdout, "Fixed:       %s\n", a.Summary)
			}
			for _, p := range rep.Remaining {
				fmt.Fprintf(d.Stdout, "Remaining:   %s\n", p.Detail)
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "repair the problems found")
	return cmd
}

func describe(st State, name string) string {
	if name == "" {
		return "(none)"
	}
	src, ok := st.Source(name)
	if !ok {
		return name + " (not connected)"
	}
	if dev, ok := st.Device(src.DeviceID); ok && dev.Bluetooth {
		return fmt.Sprintf("%s (Bluetooth, %s)", src.Description, dev.Active.Name)
	}
	return src.Description
}
