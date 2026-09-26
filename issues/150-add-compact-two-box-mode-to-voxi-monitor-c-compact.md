# 150 — Add compact two-box mode to voxi monitor (-c, --compact)

**Status**: In Progress
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Feature
**Related**: 086 (detailed view mic meter), 142 (unified feed), `spec/actions.yaml`, `docs/data/monitor-compact-design-008.ansi`

---

## 1. Problem & Motivation

The full `voxi monitor` layout displays multiple vertically stacked boxes (Voice & Speed, Hardware Load, Daemons & Health, Transcript Feed, TTS Controls), which can require significant vertical terminal space. In a compact terminal or tiling window manager pane, users want a high-density, two-box dashboard that skips redundant CPU/GPU metrics and focuses directly on speech streaming, chunk timelines, and dictation feed history.

## 2. Technical Specification / Findings

Following the Harnez-style compact TUI design established in `docs/data/monitor-compact-design-008.ansi`:
- **Left Box (`¹ Live Voice Stream`)**:
  - Live recording/idle state badge, active ASR model, single-character rainbow animated mic level sparkline (`RenderLevelChar`).
  - Modifier gating status and real-time factor (RTF) transcription speed + latency.
  - Live chunk timeline formatted consistently with `voxi chunks list` (`#INDEX`, timestamp, audio duration, level sparkline, status badge, transcript/rejection reason).
- **Right Box (`² Dictation In/Out & Health`)**:
  - ASR backend socket status, RAM & GPU VRAM allocation.
  - Daemon health indicators (`voxi-agent`, `voxi-r2t2`, `dotoold`).
  - Recent dictation stream entries (`[IN]`, `[OUT]`, and `[TTS]`).
- **Interaction & Flags**:
  - CLI flag `-c` / `--compact` on `voxi monitor`.
  - Interactive key toggle `c` / `C` in `voxi monitor -w` to switch between full and compact views.

## 3. Implementation & Verification Plan

1. Update `spec/actions.yaml` to add the `compact` action (`c`, `C`) and adjust `hardware` hotkeys.
2. Implement `PrintCompactTwoBox` in `internal/monitor/render.go`.
3. Wire `-c` / `--compact` in `cmd/voxi/main.go` and `internal/monitor/monitor.go`.
4. Add unit tests in `internal/monitor/` verifying 2-box rendering, alignment, and toggle key dispatch.
5. Verify live via `voxi monitor -c` and `voxi monitor -w`.

