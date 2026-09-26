package settings

import (
	"context"
	"fmt"

	"ubunatic.com/voxi/internal/config"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/install"
	"ubunatic.com/voxi/spec"
)

// restartServices applies settings saved by the interactive editor to the daemon
// and synchronizes any backend services (such as voxi-r2t2.service) required by the chosen model.
func restartServices(ctx context.Context, d deps.Dependencies, home string, s *config.UserSettings) error {
	if d.Run == nil {
		return fmt.Errorf("command runner is unavailable")
	}

	if s != nil {
		models, err := spec.LoadModels()
		if err == nil {
			modelName := s.ASRModel
			if modelName == "" {
				modelName = models.DefaultModel
			}

			if install.IsR2T2Active(modelName, models) {
				if err := d.Run(ctx, "systemctl", "--user", "enable", "--now", "voxi-r2t2.service"); err != nil {
					return fmt.Errorf("enable/start voxi-r2t2.service: %w", err)
				}
			} else {
				// Best effort disable & stop if not required
				_ = d.Run(ctx, "systemctl", "--user", "disable", "--now", "voxi-r2t2.service")
			}
		}
	}

	if err := d.Run(ctx, "systemctl", "--user", "try-restart", "voxi-agent.service"); err != nil {
		return fmt.Errorf("restart voxi-agent.service: %w", err)
	}
	return nil
}

// restartAgent applies settings saved by the interactive editor to the daemon.
func restartAgent(ctx context.Context, d deps.Dependencies) error {
	return restartServices(ctx, d, "", nil)
}

