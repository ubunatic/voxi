package tts

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/deps"
)

// NewSayCommand creates the monitor-gated voxi say CLI command.
func NewSayCommand(d deps.Dependencies) *cobra.Command {
	interrupt := false
	from := ""
	cmd := &cobra.Command{
		Use:   "say [text...]",
		Short: "Read text aloud through an open voxi monitor",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var text string
			if from != "" {
				if len(args) > 0 {
					return fmt.Errorf("--from cannot be combined with text arguments")
				}
				if from != "primary" && from != "clipboard" {
					return fmt.Errorf("--from must be primary or clipboard")
				}
				pasteArgs := []string{}
				if from == "primary" {
					pasteArgs = append(pasteArgs, "--primary")
				}
				if d.LookPath != nil {
					if _, err := d.LookPath("wl-paste"); err != nil {
						return fmt.Errorf("read %s selection: wl-paste is required: %w", from, err)
					}
				}
				var err error
				if d.RunOutput != nil {
					text, err = d.RunOutput(cmd.Context(), "wl-paste", pasteArgs...)
				} else {
					var output []byte
					output, err = exec.CommandContext(cmd.Context(), "wl-paste", pasteArgs...).Output()
					text = string(output)
				}
				if err != nil {
					return fmt.Errorf("read %s selection: %w", from, err)
				}
				if strings.TrimSpace(text) == "" {
					return fmt.Errorf("%s selection is empty", from)
				}
			} else if len(args) > 0 {
				text = strings.Join(args, " ")
			} else {
				data, err := io.ReadAll(io.LimitReader(d.Stdin, maxTextBytes+1))
				if err != nil {
					return fmt.Errorf("read text from stdin: %w", err)
				}
				if len(data) > maxTextBytes {
					return fmt.Errorf("text exceeds %d byte limit", maxTextBytes)
				}
				text = string(data)
			}
			runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
			if d.Getenv != nil {
				runtimeDir = d.Getenv("XDG_RUNTIME_DIR")
			}
			client := Client{SocketPath: SocketPath(runtimeDir, os.Getuid())}
			var count int
			var err error
			if interrupt {
				count, err = client.Replace(cmd.Context(), text)
			} else {
				count, err = client.Enqueue(cmd.Context(), text)
			}
			if err != nil {
				return err
			}
			fmt.Fprintf(d.Stdout, "queued %d TTS chunk(s)\n", count)
			return nil
		},
	}
	cmd.Flags().BoolVar(&interrupt, "interrupt", false, "stop current TTS and replace it with this text")
	cmd.Flags().StringVar(&from, "from", "", "read from Wayland primary selection or clipboard")
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	return cmd
}

// ReportCommandError prints errors to stderr and also notifies from terminal-free shortcuts.
func ReportCommandError(d deps.Dependencies, err error) {
	if err == nil {
		return
	}
	if d.Stderr != nil {
		fmt.Fprintln(d.Stderr, err)
	}
	if d.LookPath == nil {
		return
	}
	notify, lookErr := d.LookPath("notify-send")
	if lookErr != nil {
		return
	}
	run := d.Run
	if run == nil {
		run = func(ctx context.Context, name string, args ...string) error {
			return exec.CommandContext(ctx, name, args...).Run()
		}
	}
	_ = run(context.Background(), notify, "Voxi", err.Error())
}
