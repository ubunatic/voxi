package record

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ubunatic.com/voxi/internal/agent"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/eager"
	"ubunatic.com/voxi/internal/mode"
)

// RecordAction represents a recording control verb (toggle, start, stop).
type RecordAction string

const (
	RecordActionToggle RecordAction = "toggle"
	RecordActionStart  RecordAction = "start"
	RecordActionStop   RecordAction = "stop"
)

// ControlRecording sends a recording control action to the active speech engine.
func ControlRecording(ctx context.Context, d deps.Dependencies, action RecordAction) error {
	if status, err := agent.DefaultClient().Record(ctx, agent.RecordAction(action)); err == nil {
		if d.Stdout != nil {
			fmt.Fprintln(d.Stdout, agent.RecordingMessage(status.Recording))
		}
		return nil
	} else if !errors.Is(err, agent.ErrUnavailable) {
		return fmt.Errorf("control recording through agent: %w", err)
	}

	m := mode.CurrentVoiceInputMode(ctx, d)
	if m == mode.ModeEager {
		return eager.ControlEagerDaemon(ctx, d, string(action))
	}

	if _, err := d.LookPath("voxtype"); err != nil {
		return fmt.Errorf("voxtype not found on PATH: %w", err)
	}

	switch action {
	case RecordActionToggle:
		if err := d.Run(ctx, "voxtype", "record", "toggle"); err != nil {
			return fmt.Errorf("voxtype record toggle: %w", err)
		}
	case RecordActionStart:
		if err := d.Run(ctx, "voxtype", "record", "start"); err != nil {
			return fmt.Errorf("voxtype record start: %w", err)
		}
	case RecordActionStop:
		if err := d.Run(ctx, "voxtype", "record", "stop"); err != nil {
			return fmt.Errorf("voxtype record stop: %w", err)
		}
	default:
		return fmt.Errorf("unknown record action %q (want toggle, start, or stop)", action)
	}
	return nil
}

// GetRecordingStatus queries the current recording/idle status of the speech engine.
func GetRecordingStatus(ctx context.Context, d deps.Dependencies) (string, error) {
	if status, err := agent.DefaultClient().Status(ctx); err == nil {
		return string(status.Recording), nil
	} else if !errors.Is(err, agent.ErrUnavailable) {
		return "inactive", fmt.Errorf("get recording status through agent: %w", err)
	}

	m := mode.CurrentVoiceInputMode(ctx, d)
	if m == mode.ModeEager {
		return eager.GetEagerRecordingStatus(ctx, d)
	}

	if _, err := d.LookPath("voxtype"); err != nil {
		return "inactive", fmt.Errorf("voxtype not found on PATH: %w", err)
	}
	if d.RunOutput != nil {
		out, err := d.RunOutput(ctx, "voxtype", "status")
		if err != nil {
			return "idle", nil
		}
		raw := strings.ToLower(out)
		if strings.Contains(raw, "recording") || strings.Contains(raw, "listening") {
			return "recording", nil
		}
		return "idle", nil
	}
	return "idle", nil
}
