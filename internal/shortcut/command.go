package shortcut

import (
	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/deps"
)

// NewCommand creates the desktop shortcut management command.
func NewCommand(d deps.Dependencies) *cobra.Command {
	cmd := &cobra.Command{Use: "shortcut", Short: "Manage the standard desktop recording shortcut"}
	setupCmd := &cobra.Command{Use: "setup", Short: "Configure Super+X on GNOME", Args: cobra.NoArgs}
	setupCmd.RunE = func(cmd *cobra.Command, _ []string) error {
		force, _ := cmd.Flags().GetBool("force")
		return Setup(cmd.Context(), d, force)
	}
	setupCmd.Flags().BoolP("force", "f", false, "proceed despite an existing Super+X assignment")
	cmd.AddCommand(
		setupCmd,
		&cobra.Command{Use: "remove", Short: "Remove the Voxi-owned GNOME shortcut", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return Remove(cmd.Context(), d) }},
	)
	return cmd
}
