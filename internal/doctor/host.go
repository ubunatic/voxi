package doctor

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"ubunatic.com/voxi/spec"
)

// Host supplies read-only system state to checks. ProbeRouting may play silence.
type Host interface {
	Run(context.Context, string, ...string) ([]byte, error)
	Apply(context.Context, string) error
	ProbeRouting(context.Context, spec.DoctorRouting) (string, error)
	Wait(context.Context, time.Duration) error
}

type systemHost struct{}

func (systemHost) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func (systemHost) Wait(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (systemHost) Apply(ctx context.Context, command string) error {
	args, err := commandArgs(command)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("doctor: empty repair command")
	}
	return exec.CommandContext(ctx, args[0], args[1:]...).Run()
}

func commandArgs(command string) ([]string, error) {
	var args []string
	var word strings.Builder
	quote := rune(0)
	escaped, started := false, false
	for _, r := range command {
		if escaped {
			word.WriteRune(r)
			escaped, started = false, true
			continue
		}
		if quote == '\'' {
			if r == '\'' {
				quote = 0
			} else {
				word.WriteRune(r)
			}
			started = true
			continue
		}
		if quote == '"' {
			switch r {
			case '"':
				quote = 0
			case '\\':
				escaped = true
			default:
				word.WriteRune(r)
			}
			started = true
			continue
		}
		switch {
		case r == '\'' || r == '"':
			quote, started = r, true
		case r == '\\':
			escaped, started = true, true
		case r == ' ' || r == '\t':
			if started {
				args = append(args, word.String())
				word.Reset()
				started = false
			}
		case r == '\n' || r == ';' || r == '|' || r == '&' || r == '`' || r == '$':
			return nil, fmt.Errorf("doctor: unsafe character in repair command")
		default:
			word.WriteRune(r)
			started = true
		}
	}
	if quote != 0 || escaped {
		return nil, fmt.Errorf("doctor: incomplete quoting in repair command")
	}
	if started {
		args = append(args, word.String())
	}
	return args, nil
}

func (systemHost) ProbeRouting(ctx context.Context, cfg spec.DoctorRouting) (string, error) {
	for _, name := range []string{"pw-play", "pw-link"} {
		if _, err := exec.LookPath(name); err != nil {
			return "", fmt.Errorf("%s: %w", name, err)
		}
	}
	script, err := spec.DoctorScript(cfg.Script)
	if err != nil {
		return "", err
	}
	tmpDir, err := os.MkdirTemp("", "voxi-doctor-")
	if err != nil {
		return "", fmt.Errorf("create routing probe directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	wavPath := filepath.Join(tmpDir, "silence.wav")
	if err := writeSilenceWAV(wavPath, cfg); err != nil {
		return "", err
	}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, "bash", "-c", string(script), "voxi-doctor", cfg.ProbeNode, wavPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if probeCtx.Err() != nil {
			return "", fmt.Errorf("routing probe timed out: %w", probeCtx.Err())
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func writeSilenceWAV(path string, cfg spec.DoctorRouting) error {
	if cfg.SampleRate < 1 || cfg.Channels < 1 || cfg.DurationMS < 1 {
		return fmt.Errorf("doctor: invalid routing probe audio settings")
	}
	dataBytes := cfg.SampleRate * cfg.Channels * 2 * cfg.DurationMS / 1000
	data := make([]byte, 44+dataBytes)
	copy(data[0:4], "RIFF")
	binary.LittleEndian.PutUint32(data[4:8], uint32(36+dataBytes))
	copy(data[8:16], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:20], 16)
	binary.LittleEndian.PutUint16(data[20:22], 1)
	binary.LittleEndian.PutUint16(data[22:24], uint16(cfg.Channels))
	binary.LittleEndian.PutUint32(data[24:28], uint32(cfg.SampleRate))
	binary.LittleEndian.PutUint32(data[28:32], uint32(cfg.SampleRate*cfg.Channels*2))
	binary.LittleEndian.PutUint16(data[32:34], uint16(cfg.Channels*2))
	binary.LittleEndian.PutUint16(data[34:36], 16)
	copy(data[36:40], "data")
	binary.LittleEndian.PutUint32(data[40:44], uint32(dataBytes))
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write routing probe silence: %w", err)
	}
	return nil
}
