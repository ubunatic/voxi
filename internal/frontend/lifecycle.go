// SPDX-FileCopyrightText: 2026 Uwe Jugel
// SPDX-License-Identifier: AGPL-3.0-or-later

package frontend

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"ubunatic.com/voxi/internal/agent"
	"ubunatic.com/voxi/internal/tts"
)

// lifecycleTimeout bounds one start or stop. A model server can take a while
// to load, so this is much longer than the status probeTimeout.
const lifecycleTimeout = 90 * time.Second

// ErrLmcoderMissing means a lmcoder lifecycle command cannot run because lmcoder
// is not installed.
var ErrLmcoderMissing = errors.New("lmcoder is not installed; install it first")

// ErrNotVoxiOwned means the front ends may not start or stop the named unit.
var ErrNotVoxiOwned = errors.New("unit is not voxi-owned")

// voxiOwnedUnits are the only systemd user units the front ends may start or
// stop. lmcoder-owned servers are never listed here: they go through lmcoder.
var voxiOwnedUnits = []string{"voxi-r2t2.service"}

// SetASRService starts or stops the ASR model server through lmcoder's
// supported service command. It never calls systemctl for an lmcoder unit.
func (d Deps) SetASRService(ctx context.Context, start bool) error {
	verb := "stop"
	if start {
		verb = "start"
	}
	ctx, cancel := context.WithTimeout(ctx, lifecycleTimeout)
	defer cancel()
	if _, err := d.Run(ctx, "lmcoder", "version"); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return ErrLmcoderMissing
		}
		return fmt.Errorf("%w: %v", ErrLmcoderMissing, err)
	}
	out, err := d.Run(ctx, "lmcoder", "service", verb, "--asr")
	if err != nil {
		return fmt.Errorf("lmcoder service %s --asr: %w: %s", verb, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// SetVoxiUnit starts or stops a voxi-owned user unit through systemctl --user.
func (d Deps) SetVoxiUnit(ctx context.Context, unit string, start bool) error {
	if !isVoxiOwned(unit) {
		return fmt.Errorf("%s: %w", unit, ErrNotVoxiOwned)
	}
	verb := "stop"
	if start {
		verb = "start"
	}
	ctx, cancel := context.WithTimeout(ctx, lifecycleTimeout)
	defer cancel()
	out, err := d.Run(ctx, "systemctl", "--user", verb, unit)
	if err != nil {
		return fmt.Errorf("systemctl --user %s %s: %w: %s", verb, unit, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// SetListening starts or stops microphone recording in the voxi agent.
// Starting recording opens the microphone, so front ends call this only on an
// explicit user action.
func (d Deps) SetListening(ctx context.Context, on bool) (agent.Status, error) {
	action := agent.RecordStop
	if on {
		action = agent.RecordStart
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	return d.Agent.Record(ctx, action)
}

// SpeechControl sends one playback action to the voxi TTS server.
func (d Deps) SpeechControl(ctx context.Context, action tts.Action) error {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	return d.TTS.Control(ctx, action)
}

func isVoxiOwned(unit string) bool {
	for _, owned := range voxiOwnedUnits {
		if unit == owned {
			return true
		}
	}
	return false
}
