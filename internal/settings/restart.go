package settings

import (
	"context"
	"fmt"

	"ubunatic.com/voxi/internal/deps"
)

// restartAgent applies settings saved by the interactive editor to the daemon.
func restartAgent(ctx context.Context, d deps.Dependencies) error {
	if d.Run == nil {
		return fmt.Errorf("command runner is unavailable")
	}
	if err := d.Run(ctx, "systemctl", "--user", "try-restart", "voxi-agent.service"); err != nil {
		return fmt.Errorf("restart service: %w", err)
	}
	return nil
}
