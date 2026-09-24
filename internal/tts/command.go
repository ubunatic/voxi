package tts

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/deps"
)

// NewSayCommand creates the monitor-gated voxi say CLI command.
func NewSayCommand(d deps.Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "say [text...]",
		Short: "Read text aloud through an open voxi monitor",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var text string
			if len(args) > 0 {
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
			count, err := client.Enqueue(cmd.Context(), text)
			if err != nil {
				return err
			}
			fmt.Fprintf(d.Stdout, "queued %d TTS chunk(s)\n", count)
			return nil
		},
	}
	cmd.SilenceUsage = true
	return cmd
}
