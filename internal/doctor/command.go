package doctor

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/spec"
)

// ExitError carries a command result status through Cobra to main.
type ExitError struct{ Code int }

func (e ExitError) Error() string { return fmt.Sprintf("voxi doctor exited with status %d", e.Code) }

// CommandExitCode returns the process exit status requested by this command.
func (e ExitError) CommandExitCode() int { return e.Code }

// NewCommand creates the read-only `voxi doctor` command.
func NewCommand(out io.Writer, host Host) *cobra.Command {
	if host == nil {
		host = systemHost{}
	}
	var opts Options
	cmd := &cobra.Command{
		Use:           "doctor",
		Short:         "Diagnose audio stack faults without changing system state",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := spec.LoadDoctor()
			if err != nil {
				return ExitError{Code: 2}
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			report, err := (HostDoctor{Host: host, Spec: s}).Run(ctx, opts)
			if err != nil {
				return ExitError{Code: 2}
			}
			report.Print(out)
			if code := report.ExitCode(); code != 0 {
				return ExitError{Code: code}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.PWDump, "pw-dump", "", "read PipeWire state from a saved pw-dump JSON file")
	cmd.Flags().StringVar(&opts.Since, "since", "", "journal and core dump window (default: since last resume / last 7 days)")
	cmd.Flags().BoolVar(&opts.NoProbe, "no-probe", false, "skip the silent playback routing probe")
	return cmd
}
