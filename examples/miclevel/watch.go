// SPDX-FileCopyrightText: 2026 Uwe Jugel
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"codeberg.org/ubunatic/loom"
	"codeberg.org/ubunatic/loom/graph"
	"ubunatic.com/voxi/audiolevel"
)

const (
	// collectInterval doubles as the redraw interval: unlike loom's own
	// monitor example (slow collect, fast redraw), audiolevel already
	// smooths raw samples via ballistics, so a single fast tick both
	// advances the meter (Manager.Tick) and repaints it.
	collectInterval = 40 * time.Millisecond
	meterWindow     = 200 * time.Millisecond
	meterAttack     = 80 * time.Millisecond
	meterDecay      = 400 * time.Millisecond
	sparklineWidth  = 32
	barWidth        = 20
	micBoxID        = "mic"
)

// micSpec tunes the live mic-level meter: a short window smooths jitter
// between individual audio chunks without lagging noticeably behind speech
// onset, and attack/decay ease the displayed level toward each new target
// in both directions (see audiolevel.ApplyBallisticsEased).
func micSpec() (metric audiolevel.Metric, window, attack, decay time.Duration) {
	return audiolevel.MetricMax, meterWindow, meterAttack, meterDecay
}

// buildMicCaptureCmd picks parec (preferred: tags the stream so desktop
// mic-in-use indicators exempt it, see audiolevel.SuppressApplicationID) or
// falls back to pw-record, matching the backend selection voxi's own
// internal/monitor package uses. Returns nil if neither is on PATH, meaning
// live capture structurally can't work on this machine — audiolevel.StartManager
// degrades to a permanently unavailable Manager rather than erroring.
func buildMicCaptureCmd() func(context.Context) *exec.Cmd {
	if _, err := exec.LookPath("parec"); err == nil {
		return func(ctx context.Context) *exec.Cmd { return audiolevel.ParecCommand(ctx, 0) }
	}
	if _, err := exec.LookPath("pw-record"); err == nil {
		return func(ctx context.Context) *exec.Cmd { return audiolevel.PwRecordCommand(ctx, 0) }
	}
	return nil
}

func runWatch(ctx context.Context) error {
	data, err := documents.ReadFile("spec/miclevel.yaml")
	if err != nil {
		return err
	}
	root, cfg, err := loom.BuildWidget(bytes.NewReader(data))
	if err != nil {
		return err
	}
	frame, ok := root.(*loom.Frame)
	if !ok {
		return fmt.Errorf("miclevel: declared root must be a frame")
	}

	buildCmd := buildMicCaptureCmd()
	backendAvailable := buildCmd != nil
	if !backendAvailable {
		setFooter(frame, "(no mic backend found: need parec or pw-record)")
	}
	// audiolevel.DefaultSampleRate must match SparklineOptions.SampleRate
	// below — both describe the same ParecCommand/PwRecordCommand stream.
	mgr := audiolevel.StartManager(ctx, buildCmd, audiolevel.DefaultChunkBytes, audiolevel.DefaultMinDBFS, 0, micSpec, nil)
	defer mgr.Stop()
	mgr.EnableSparkline(audiolevel.SparklineOptions{
		Width:      sparklineWidth,
		SampleRate: audiolevel.DefaultSampleRate,
		Window:     3 * time.Second,
	})

	collect := func(now time.Time) error {
		reading := mgr.Tick(now, meterAttack, meterDecay)
		applyReading(frame, backendAvailable, reading, mgr.Sparkline())
		return nil
	}
	if err := collect(time.Now()); err != nil {
		return err
	}

	pane, err := loom.New(cfg.Height(0))
	if err != nil {
		return err
	}
	defer pane.Close()
	pane.MaxCols = cfg.MaxWidth()
	pane.Resizeable = true
	return pane.RunWatch(ctx, frame, loom.Cadence{Collect: collectInterval, Redraw: collectInterval}, collect)
}

func setFooter(frame *loom.Frame, text string) {
	for i := range frame.Boxes {
		if frame.Boxes[i].ID == micBoxID {
			frame.Boxes[i].Footer = text
		}
	}
}

func applyReading(frame *loom.Frame, backendAvailable bool, reading audiolevel.Reading, sparkline string) {
	for i := range frame.Boxes {
		box := &frame.Boxes[i]
		if box.ID != micBoxID {
			continue
		}
		values := box.Rows.GetValues()
		if len(values) > 0 && len(values[0]) > 1 {
			values[0][1] = levelBar(reading)
		}
		if len(values) > 1 && len(values[1]) > 1 {
			values[1][1] = waveline(backendAvailable, sparkline)
		}
		box.SetRowsValues(values)
	}
}

// SubChar's boundary glyph can show a cosmetic terminal-dependent seam
// without ANSI background styling (loom issue 039, filed from this
// example) — kept anyway to match examples/monitor's own SubChar usage
// rather than special-casing around a cosmetic, already-tracked loom gap.
func levelBar(reading audiolevel.Reading) string {
	if !reading.Available {
		return fmt.Sprintf("%s --%%", graph.RenderBar(0, graph.BarOptions{Width: barWidth}))
	}
	return fmt.Sprintf("%s %3.0f%%", graph.RenderBar(reading.Level, graph.BarOptions{Width: barWidth, SubChar: true}), reading.Level)
}

func waveline(backendAvailable bool, sparkline string) string {
	if !backendAvailable {
		return "(no backend)"
	}
	// SparklineStream.Sparkline returns a space-padded (not empty) string
	// until its rolling buffer has at least one real audio chunk.
	if strings.TrimSpace(sparkline) == "" {
		return "connecting"
	}
	return sparkline
}
