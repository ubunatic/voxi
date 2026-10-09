// SPDX-FileCopyrightText: 2026 Uwe Jugel
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"sync"

	"ubunatic.com/loom"
	"ubunatic.com/voxi/internal/frontend"
	"ubunatic.com/voxi/internal/tts"
)

// voxiR2T2 is the only voxi-owned unit the Servers tab may start or stop.
const voxiR2T2 = "voxi-r2t2.service"

// ui is the loom widget for the three tabs. It holds the latest snapshot,
// turns key presses into actions, and draws the box plus a status line.
// Actions that change state run off the key loop and report back through note.
type ui struct {
	deps       frontend.Deps
	spawn      func(func()) // runs an action off the key loop; tests run it inline
	invalidate func()       // asks the renderer to redraw after a change

	mu   sync.Mutex
	tab  tab
	snap frontend.Snapshot
	note string
}

func newUI(deps frontend.Deps, invalidate func()) *ui {
	return &ui{
		deps:       deps,
		spawn:      func(f func()) { go f() },
		invalidate: invalidate,
	}
}

// refresh reads every status once and redraws. It runs off the key loop.
func (u *ui) refresh(ctx context.Context) {
	snap := frontend.Collect(ctx, u.deps)
	u.mu.Lock()
	u.snap = snap
	u.mu.Unlock()
	u.invalidate()
}

func (u *ui) setNote(text string) {
	u.mu.Lock()
	u.note = text
	u.mu.Unlock()
	u.invalidate()
}

// do runs one action off the key loop, then reports the outcome and refreshes.
func (u *ui) do(label string, act func(ctx context.Context) error) {
	u.setNote(label + "…")
	u.spawn(func() {
		ctx := context.Background()
		if err := act(ctx); err != nil {
			u.setNote(label + " failed: " + err.Error())
		} else {
			u.setNote(label + ": done")
		}
		u.refresh(ctx)
	})
}

func (u *ui) selectTab(t tab) {
	u.mu.Lock()
	u.tab = t
	u.mu.Unlock()
	u.invalidate()
}

// ConsumeKey maps keys to tabs and to each tab's actions. q and Ctrl-C quit.
// Ctrl-C arrives as a key because loom puts the terminal in raw mode; loom.Pane.Close restores it.
func (u *ui) ConsumeKey(e loom.KeyEvent) loom.EventResult {
	name := e.Name()
	switch name {
	case "q", "ctrl-c":
		return loom.Quit()
	case "1":
		u.selectTab(tabSpeech)
		return loom.Handled()
	case "2":
		u.selectTab(tabServers)
		return loom.Handled()
	case "3":
		u.selectTab(tabSettings)
		return loom.Handled()
	}

	u.mu.Lock()
	t := u.tab
	u.mu.Unlock()
	switch t {
	case tabSpeech:
		u.speechKey(name)
	case tabServers:
		u.serversKey(name)
	case tabSettings:
		u.settingsKey(name)
	}
	return loom.Handled()
}

// speechKey starts and stops microphone recording and TTS playback. Those
// calls change what the agent does, so they run only on these keys.
func (u *ui) speechKey(name string) {
	switch name {
	case "r":
		u.do("Start listening", func(ctx context.Context) error {
			_, err := u.deps.SetListening(ctx, true)
			return err
		})
	case "x":
		u.do("Stop listening", func(ctx context.Context) error {
			_, err := u.deps.SetListening(ctx, false)
			return err
		})
	case "p":
		u.do("Play / pause", func(ctx context.Context) error {
			return u.deps.SpeechControl(ctx, tts.ActionPlayPause)
		})
	case "s":
		u.do("Stop playback", func(ctx context.Context) error {
			return u.deps.SpeechControl(ctx, tts.ActionStop)
		})
	}
}

// serversKey starts and stops model servers through their owning tool: lmcoder
// for the ASR server, systemctl only for the voxi-owned voxi-r2t2 unit.
func (u *ui) serversKey(name string) {
	switch name {
	case "r":
		u.spawn(func() { u.refresh(context.Background()) })
	case "i":
		u.setNote("Install lmcoder so it is on PATH, then press r to refresh.")
	case "s":
		u.do("Start ASR via lmcoder", func(ctx context.Context) error {
			return u.deps.SetASRService(ctx, true)
		})
	case "x":
		u.do("Stop ASR via lmcoder", func(ctx context.Context) error {
			return u.deps.SetASRService(ctx, false)
		})
	case "a":
		u.do("Start "+voxiR2T2, func(ctx context.Context) error {
			return u.deps.SetVoxiUnit(ctx, voxiR2T2, true)
		})
	case "d":
		u.do("Stop "+voxiR2T2, func(ctx context.Context) error {
			return u.deps.SetVoxiUnit(ctx, voxiR2T2, false)
		})
	}
}

// settingsKey is a placeholder: the mockup's edit and restore actions have no
// backing in this build, so the keys say so instead of changing anything.
func (u *ui) settingsKey(name string) {
	switch name {
	case "e", "r":
		u.setNote("Editing settings is not part of this build yet (issue 186).")
	}
}

// Draw paints the tab's box and the status line. A terminal smaller than the
// box gets a short message instead of a cut-off box.
func (u *ui) Draw(c *loom.Canvas, r loom.Rect) {
	u.mu.Lock()
	t, snap, note := u.tab, u.snap, u.note
	u.mu.Unlock()

	needW, needH := t.width(), screenHeight+noteRows
	if r.W < needW || r.H < needH {
		lines := []string{
			"Terminal too small",
			fmt.Sprintf("need %dx%d, have %dx%d", needW, needH, r.W, r.H),
			"Resize, or press q to quit.",
		}
		for i, line := range lines {
			if i < r.H {
				c.Write(r.X, r.Y+i, clip(line, r.W), loom.Style{})
			}
		}
		return
	}
	for i, line := range screen(t, snap) {
		c.Write(r.X, r.Y+i, line, loom.Style{})
	}
	c.Write(r.X, r.Y+screenHeight, clip(note, r.W), loom.Style{})
}

// ConsumeMouse ignores the mouse; every action is a key.
func (u *ui) ConsumeMouse(loom.MouseEvent) loom.EventResult {
	return loom.Ignored()
}
