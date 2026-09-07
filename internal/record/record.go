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
	"ubunatic.com/voxi/spec"
)

// RecordAction represents a recording control verb (toggle, start, stop).
type RecordAction string

const (
	RecordActionToggle RecordAction = "toggle"
	RecordActionStart  RecordAction = "start"
	RecordActionStop   RecordAction = "stop"
)

// defaultEngineNeedsVoxtype reports whether the model spec's currently
// configured default model resolves to the "whisper" engine -- the only
// engine voxtype's own "record"/"status" subcommands can serve. See issue
// 077: before this, ModeNeither (no voice-input systemd unit active, the
// common state before any dictation session has been started, or the
// steady state when the default engine's own session lives behind the
// voxi-agent RPC rather than a tracked systemd unit) unconditionally
// required voxtype even when the default model is cohere-transcribe, which
// never touches voxtype. If the spec cannot be loaded at all, this
// preserves the pre-issue-077 behavior (unconditional voxtype requirement)
// rather than silently routing around a broken install.
func defaultEngineNeedsVoxtype() bool {
	modelSpec, err := spec.LoadModels()
	if err != nil {
		return true
	}
	return modelSpec.IsWhisperEngine(modelSpec.DefaultModel)
}

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
	if m == mode.ModeEager || (m == mode.ModeNeither && !defaultEngineNeedsVoxtype()) {
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
	if m == mode.ModeEager || (m == mode.ModeNeither && !defaultEngineNeedsVoxtype()) {
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
