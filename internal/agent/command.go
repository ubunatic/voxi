package agent

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/tts"
	"ubunatic.com/voxi/spec"
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
			executable, err := os.Executable()
			if err != nil {
				return err
			}
			home := os.Getenv("HOME")
			if d.Getenv != nil {
				home = d.Getenv("HOME")
			}
			settings, err := config.LoadUserSettings(home)
			if err != nil {
				return fmt.Errorf("load TTS settings: %w", err)
			}
			manager := tts.NewManagerWithEnabled(cmd.Context(), tts.NewEngine(d, executable), settings.TTSEnabled)
			defer manager.Close()
			server, err := tts.StartServer(cmd.Context(), "", manager)
			if err != nil {
				return err
			}
			defer server.Close()
			if settings.TTSEnabled {
				mpris, mprisErr := tts.StartMPRIS(cmd.Context(), manager)
				if mprisErr != nil {
					fmt.Fprintf(d.Stderr, "warning: MPRIS unavailable: %v\n", mprisErr)
				} else {
					defer mpris.Close()
				}
			}
			if err := a.Start(cmd.Context()); err != nil {
				fmt.Fprintf(d.Stderr, "warning: ASR backend failed to start; TTS remains available: %v\n", err)
			}
			go watchASRFallbackRevert(cmd.Context(), d)
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

func watchASRFallbackRevert(ctx context.Context, d deps.Dependencies) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			home := d.Getenv("HOME")
			if home == "" {
				home, _ = os.UserHomeDir()
			}
			settings, err := config.LoadUserSettings(home)
			if err != nil || settings == nil {
				continue
			}
			if settings.ASRModel == "r2t2-confucius4-voxi" {
				dial := d.DialTimeout
				if dial == nil {
					dial = net.DialTimeout
				}
				conn, err := dial("tcp", "127.0.0.1:8735", 500*time.Millisecond)
				if err == nil {
					conn.Close()
					modelSpec, specErr := spec.LoadModels()
					defaultModel := "r2t2-confucius4"
					if specErr == nil && modelSpec != nil && modelSpec.DefaultModel != "" {
						defaultModel = modelSpec.DefaultModel
					}
					settings.ASRModel = defaultModel
					_ = config.SaveUserSettings(home, settings)
					_ = exec.CommandContext(ctx, "systemctl", "--user", "stop", "voxi-r2t2.service").Run()
					fmt.Fprintf(d.Stderr, "voxi-agent: primary ASR endpoint (127.0.0.1:8735) recovered; reverted from fallback to %s\n", defaultModel)
				}
			}
		}
	}
}
