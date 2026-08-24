package agent

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/deps"
)

// NewCommand returns the `voxi agent` command tree.
func NewCommand(d deps.Dependencies) *cobra.Command {
	var daemon bool
	var socketPath string
	var statePath string

	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Run or control the unified Voxi voice-input agent",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !daemon {
				return fmt.Errorf("specify --daemon or an agent subcommand")
			}
			a, err := New(Options{StatePath: statePath, Backends: RuntimeBackends(d)})
			if err != nil {
				return err
			}
			if err := a.Start(cmd.Context()); err != nil {
				return err
			}
			defer a.Stop(context.Background())
			return Serve(cmd.Context(), socketPath, a)
		},
	}
	cmd.PersistentFlags().BoolVar(&daemon, "daemon", false, "run the agent daemon and control socket")
	cmd.PersistentFlags().StringVar(&socketPath, "socket", "", "Unix socket path (default: XDG runtime dir)")
	cmd.PersistentFlags().StringVar(&statePath, "state", "", "persistent mode-state path")

	clientFor := func() Client { return Client{SocketPath: socketPath} }
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Print the agent's selected mode and recording state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := clientFor().Status(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Fprintf(d.Stdout, "%s (%s)\n", status.Mode, status.Recording)
			return nil
		},
	}
	setModeCmd := &cobra.Command{
		Use:   "set-mode [batch|streaming|eager]",
		Short: "Set the agent's selected voice-input backend",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := clientFor().SetMode(cmd.Context(), Mode(args[0]))
			if err != nil {
				return err
			}
			fmt.Fprintf(d.Stdout, "agent mode set to %s (%s)\n", status.Mode, status.Recording)
			return nil
		},
	}
	recordCmd := &cobra.Command{
		Use:   "record [toggle|start|stop]",
		Short: "Control recording through the agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := clientFor().Record(cmd.Context(), RecordAction(args[0]))
			if err != nil {
				return err
			}
			fmt.Fprintln(d.Stdout, recordingMessage(status.Recording))
			return nil
		},
	}
	cmd.AddCommand(statusCmd, setModeCmd, recordCmd)
	return cmd
}

// RecordingMessage renders the recording state for terminal output.
func RecordingMessage(state RecordingState) string {
	return recordingMessage(state)
}

func recordingMessage(state RecordingState) string {
	if state == RecordingActive {
		return "Recording started"
	}
	return "Recording stopped"
}
