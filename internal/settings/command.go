package settings

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/deps"
)

// NewCommand creates the `voxi settings` command.
func NewCommand(d deps.Dependencies, availableASRModels []string) *cobra.Command {
	var dump bool
	var asJSON bool
	var format string

	cmd := &cobra.Command{
		Use:   "settings",
		Short: "Interactive TUI for Voxi feature toggles and configuration",
		Long: "Interactive terminal menu to view and toggle core Voxi settings:\n" +
			"  • LLM Post-Processing Cleaner\n" +
			"  • Cleanup Model Selection\n" +
			"  • ASR Model Selection\n" +
			"  • Keystroke Delay (type_delay_ms)\n" +
			"  • Dictation History\n" +
			"  • Modifier Key Gating (voxi-modifierd)\n\n" +
			"When run without a terminal (e.g. piped or in scripts), settings are printed\n" +
			"as formatted text or JSON.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			home := d.Getenv("HOME")
			if home == "" {
				if h, err := os.UserHomeDir(); err == nil {
					home = h
				}
			}

			isTTY := isOutputTerminal(d)

			if dump || asJSON || format == "json" || !isTTY {
				s, err := config.LoadUserSettings(home)
				if err != nil {
					return fmt.Errorf("load settings: %w", err)
				}
				if asJSON || format == "json" {
					out, err := RenderJSON(s)
					if err != nil {
						return fmt.Errorf("encode json: %w", err)
					}
					fmt.Fprintln(d.Stdout, out)
					return nil
				}
				fmt.Fprint(d.Stdout, RenderSummary(s, home))
				return nil
			}

			return RunInteractive(cmd.Context(), d, home, availableASRModels)
		},
	}

	cmd.Flags().BoolVarP(&dump, "dump", "d", false, "print current settings non-interactively")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output current settings as JSON")
	cmd.Flags().StringVar(&format, "format", "text", "output format for non-interactive dump: text or json")

	return cmd
}

func isOutputTerminal(d deps.Dependencies) bool {
	if f, ok := d.Stdout.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}
