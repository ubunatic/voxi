package mode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"ubunatic.com/voxi/internal/deps"
)

// Voice-input streaming/batch/eager modes run as mutually exclusive systemd user services.
const (
	BatchService       = "voxtype.service"
	StreamingService   = "voxtype-streaming.service"
	EagerService       = "voxi-eager.service"
	LegacyEagerService = "harnez-voice-eager.service"

	streamingConfigRelPath = ".config/voxtype/config-streaming.toml"
	streamingModelRelPath  = ".local/share/voxtype/models/parakeet-unified-en-0.6b"
)

// VoiceInputMode is a stable identifier for the active voice-input mode.
type VoiceInputMode string

const (
	ModeBatch        VoiceInputMode = "batch"
	ModeStreaming    VoiceInputMode = "streaming"
	ModeEager        VoiceInputMode = "eager"
	ModeNeither      VoiceInputMode = "neither"
	ModeInconsistent VoiceInputMode = "inconsistent"
)

func serviceActive(ctx context.Context, d deps.Dependencies, service string) bool {
	if d.Run == nil {
		return false
	}
	return d.Run(ctx, "systemctl", "--user", "is-active", "--quiet", service) == nil
}

// CurrentVoiceInputMode reports which voice-input service, if any, is currently active.
func CurrentVoiceInputMode(ctx context.Context, d deps.Dependencies) VoiceInputMode {
	batch := serviceActive(ctx, d, BatchService)
	streaming := serviceActive(ctx, d, StreamingService)
	eager := serviceActive(ctx, d, EagerService) || serviceActive(ctx, d, LegacyEagerService)

	activeCount := 0
	for _, b := range []bool{batch, streaming, eager} {
		if b {
			activeCount++
		}
	}
	if activeCount > 1 {
		return ModeInconsistent
	}
	switch {
	case batch:
		return ModeBatch
	case streaming:
		return ModeStreaming
	case eager:
		return ModeEager
	default:
		return ModeNeither
	}
}

// DescribeVoiceInputMode renders CurrentVoiceInputMode for terminal output.
func DescribeVoiceInputMode(m VoiceInputMode) string {
	switch m {
	case ModeBatch:
		return "batch (voxtype.service active, base.en whisper, typed at end of utterance)"
	case ModeStreaming:
		return "streaming (voxtype-streaming.service active, local Parakeet, typed incrementally)"
	case ModeEager:
		return "eager (voxi-eager.service active, continuous sentence streaming via local whisper)"
	case ModeNeither:
		return "neither (all services stopped)"
	case ModeInconsistent:
		return "inconsistent (multiple voice-input services active; stop others manually)"
	default:
		return string(m)
	}
}

func checkStreamingPreconditions(d deps.Dependencies) error {
	home := d.Getenv("HOME")
	if home == "" {
		return fmt.Errorf("cannot resolve $HOME to locate voice-input streaming config")
	}
	configPath := filepath.Join(home, streamingConfigRelPath)
	if _, err := d.Stat(configPath); err != nil {
		return fmt.Errorf("streaming config missing at %s: %w", configPath, err)
	}
	modelDir := filepath.Join(home, streamingModelRelPath)
	info, err := d.Stat(modelDir)
	if err != nil {
		return fmt.Errorf("streaming model missing at %s; run: voxtype setup --download --model parakeet-unified-en-0.6b --quiet: %w", modelDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s exists but is not a directory", modelDir)
	}
	return nil
}

func ensureEagerServiceUnit(ctx context.Context, d deps.Dependencies) error {
	home := d.Getenv("HOME")
	if home == "" {
		return fmt.Errorf("cannot resolve $HOME")
	}
	unitDir := filepath.Join(home, ".config/systemd/user")
	unitPath := filepath.Join(unitDir, EagerService)

	unitContent := `[Unit]
Description=Voxi Continuous Eager Sentence Streaming Dictation
Documentation=https://github.com/ubunatic/voxi
PartOf=graphical-session.target
After=graphical-session.target

[Service]
Type=simple
ExecStart=%h/go/bin/voxi eager --daemon
Restart=on-failure
RestartSec=3

Environment=XDG_RUNTIME_DIR=%t
Environment=PATH=%h/go/bin:%h/.local/bin:/usr/local/bin:/usr/bin:/bin

[Install]
WantedBy=graphical-session.target
`
	if _, err := d.Stat(unitPath); err != nil {
		_ = os.MkdirAll(unitDir, 0755)
		if err := os.WriteFile(unitPath, []byte(unitContent), 0644); err != nil {
			return fmt.Errorf("create %s: %w", unitPath, err)
		}
		_ = d.Run(ctx, "systemctl", "--user", "daemon-reload")
	}
	return nil
}

func waitActive(ctx context.Context, d deps.Dependencies, service string) bool {
	const attempts = 20
	for i := 0; i < attempts; i++ {
		if serviceActive(ctx, d, service) {
			return true
		}
		d.Sleep(500 * time.Millisecond)
	}
	return false
}

// SwitchVoiceInputMode switches between batch, streaming, and eager voice-input services.
func SwitchVoiceInputMode(ctx context.Context, d deps.Dependencies, target VoiceInputMode) error {
	var wanted string
	var others []string

	switch target {
	case ModeStreaming:
		if err := checkStreamingPreconditions(d); err != nil {
			return err
		}
		wanted = StreamingService
		others = []string{BatchService, EagerService, LegacyEagerService}
	case ModeBatch:
		wanted = BatchService
		others = []string{StreamingService, EagerService, LegacyEagerService}
	case ModeEager:
		if err := ensureEagerServiceUnit(ctx, d); err != nil {
			return err
		}
		wanted = EagerService
		others = []string{BatchService, StreamingService, LegacyEagerService}
	default:
		return fmt.Errorf("invalid voice-input mode %q (want batch, streaming, or eager)", target)
	}

	if serviceActive(ctx, d, wanted) {
		fmt.Fprintf(d.Stdout, "%s already active; nothing to do\n", wanted)
		for _, other := range others {
			if serviceActive(ctx, d, other) {
				return fmt.Errorf("%s is also active; stop it manually (systemctl --user stop %s)", other, other)
			}
		}
		return nil
	}

	var activeOthers []string
	for _, other := range others {
		if serviceActive(ctx, d, other) {
			activeOthers = append(activeOthers, other)
			if err := d.Run(ctx, "systemctl", "--user", "stop", other); err != nil {
				return fmt.Errorf("stop %s: %w", other, err)
			}
		}
	}

	if err := d.Run(ctx, "systemctl", "--user", "start", wanted); err != nil {
		for _, other := range activeOthers {
			_ = d.Run(ctx, "systemctl", "--user", "start", other)
		}
		return fmt.Errorf("start %s: %w (restored %v)", wanted, err, activeOthers)
	}

	if !waitActive(ctx, d, wanted) {
		restoreErr := ""
		for _, other := range activeOthers {
			if err := d.Run(ctx, "systemctl", "--user", "start", other); err != nil {
				restoreErr = fmt.Sprintf("; failed to restore %s: %v", other, err)
			} else {
				restoreErr = fmt.Sprintf("; restored %s as a fallback", other)
			}
		}
		return fmt.Errorf("%s did not become active within 10s%s", wanted, restoreErr)
	}

	fmt.Fprintf(d.Stdout, "switched voice input to %s (%s active)\n", target, wanted)
	return nil
}
