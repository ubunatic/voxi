# 084: Add Live Mic Input-Level Meter and Volume Display to `voxi monitor`

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Enhancement
**Category**: Enhancement
**Related**: [070 RMS column and speech-level sparkline in `voxi chunks list`](070-show-recorded-volume-rms-column-in-voxi-chunks-list.md), [071 Colorize the LEVEL sparkline](071-colorize-the-level-sparkline-in-voxi-chunks-list-color-flag-design-proposal.md), [072 2x Braille resolution packed dual-column glyphs](072-2x-braille-resolution-for-the-level-sparkline-in-voxi-chunks-list-packed-dual-column-glyphs.md), [025 Voice Input: real-time audio input volume / VU meter animation in recording indicator](025-voice-input-volume-animation.md), [internal/monitor/monitor.go](../internal/monitor/monitor.go), [internal/audio/audio.go](../internal/audio/audio.go)

---

## 1. Problem & Motivation

`voxi monitor` (`internal/monitor/monitor.go`) is voxi's btop-style TUI
resource monitor, with four boxes today (`ResourceSections`: Speed,
Hardware, Transcript, Daemons). Confirmed by grep
(`grep -niE 'level|volume|rms|mic' internal/monitor/monitor.go` — no
matches): there is currently no mic/volume box at all in this file. A user
watching `voxi monitor` while dictating has no way to see, at a glance,
whether the mic is actually picking up their voice right now, or what the
system's configured input volume/gain is set to.

This is a forward-looking feature request raised by the user pointing at
the sibling project `harnez` (`../harnez` relative to this repo), which
already ships a comparable feature in its own TUI (`harnez usage --watch`)
and has documented the approach in `harnez`'s
`docs/MicIndicators.md` §15 ("Live Input Meter Ballistics & Decoupled TUI
Architecture") and implemented it in `harnez`'s
`internal/usage/miclive.go`. This ticket is scoped as a **new mic/volume
box in `voxi monitor`**, distinct from and complementary to issues 070/071/072,
which are about the `RMS`/`LEVEL` columns in `voxi chunks list` (a
per-chunk historical view of already-recorded audio), not a live TUI meter
box.

## 2. What Already Exists in voxi (Reuse Candidate)

voxi already has logarithmic RMS-scaling and Braille-sparkline rendering
code in `internal/audio/audio.go`, built for issues 070-072:

- `ComputeAudioRMS(frame []byte) int` — raw linear RMS from a PCM frame.
- `RenderVolumeSparkline` / `sparklineLevel` — logarithmic quantization of
  RMS into levels, using `sparklineFloorRMS = 80` and
  `sparklineCeilingRMS = 8192` (calibrated against voxi's own acoustic-gate
  thresholds per issue 070's post-implementation fixes), rendered as
  Braille glyphs.
- `RenderAudioLevelMeter(rms int, threshold int) string` — an existing
  single-value `■`/`·` live bar (10-char), already used during recording
  in `internal/eager/eager.go`.

None of this currently powers a `voxi monitor` box, and none of it opens a
long-lived background capture stream the way `harnez`'s `miclive.go` does
— it operates on frames voxi is already capturing during an active
recording, not a standalone always-on meter.

## 3. What harnez Does (Pointer, Not a Locked Spec)

Read directly from `../harnez` for this ticket (not re-derived/invented):

- **Capture**: `parec --raw --format=s16le --rate=8000 --channels=1
  --latency-msec=20 --process-time-msec=20 -d @DEFAULT_SOURCE@` (or a
  `pw-record`-based PipeWire-native fallback), held open in a background
  goroutine for as long as the mic box is visible, reading ~50ms chunks
  and computing RMS in memory. No audio is ever persisted.
- **Logarithmic dBFS scaling**: `-60 dBFS -> 0%`, `0 dBFS -> 100%`
  (`micLiveMinDBFS = -60.0` in `miclive.go`), i.e.
  `level = (dBFS - (-60)) / (0 - (-60)) * 100`, clamped to `[0, 100]`.
  This differs from voxi's existing `sparklineFloorRMS`/`sparklineCeilingRMS`
  scale, which is calibrated in raw linear RMS units against voxi's own
  acoustic-gate thresholds (120/150), not dBFS.
- **VU-meter ballistics** (`micLiveApplyBallistics` in `miclive.go`):
  instant peak attack (displayed level jumps immediately when the target
  level rises), exponential decay on falloff with a ~150ms time constant
  (`decayed = prev * exp(-dt/tau)`), and a clean floor snap to 0 below a
  0.5% cutoff — avoids the strobe-like jitter of a raw max-per-window
  metric collapsing abruptly to zero between syllables.
- **Two distinct values shown side by side** in harnez's mic box: the live
  real-time input signal level (actual audio energy right now, from the
  `parec` stream above) *and* the separately-configured system input
  volume/gain (i.e. what `pactl`/`wpctl` reports as the input device's set
  volume, independent of whether anything is currently making sound). The
  new voxi box should show both, matching this two-value design.
- **Decoupled redraw architecture**: harnez's TUI dynamically bumps redraw
  cadence from ~4 FPS to ~20 FPS during voice activity, and paces hardware
  load sampling independently of the meter's redraw rate, so a fast-moving
  meter doesn't drag other TUI panels into a higher, wasteful refresh rate.
  `voxi monitor`'s existing redraw/refresh model should be checked against
  this before assuming the same decoupling is or isn't already needed.
- **Privacy-indicator suppression** (`docs/MicIndicators.md` §2-§13):
  harnez tags its `parec` capture stream with
  `--property=application.id=org.gnome.VolumeControl` (confirmed by live
  canary to suppress GNOME Shell's mic-in-use indicator) and
  `--property=node.virtual=true` (inferred, not yet live-canaried, to
  suppress KDE Plasma's indicator) so a passive meter stream doesn't
  falsely trigger the OS's "microphone is recording" UI. If voxi's meter
  uses a similar always-open `parec`/`pactl`-based capture stream, this is
  directly relevant and worth reading before implementation — noted here
  as a pointer, not necessarily in scope for a first cut of this feature.

This document is dated 2026-09-07 and is itself a live-evolving design
doc in a sibling project, not a frozen spec — read it fresh at
implementation time rather than trusting this summary to still be current.

## 4. Proposed Approach — Two Options (Not Decided)

**Option A — Reuse/adapt voxi's existing sparkline/RMS code.**
Extend `internal/audio/audio.go`'s existing `ComputeAudioRMS` and
logarithmic quantization machinery to drive a live meter, keeping voxi's
own raw-RMS calibration (`sparklineFloorRMS`/`sparklineCeilingRMS`, already
tuned against voxi's acoustic gate) rather than introducing a second,
dBFS-based scale. Would need: (1) a new standalone background capture
loop (voxi's existing RMS code runs on frames from an active recording,
not a free-standing `parec` stream), and (2) porting/adapting harnez's
ballistics (attack/decay/floor-snap) on top of it.

**Option B — Port harnez's `miclive.go` approach directly.**
Bring over harnez's dBFS scaling (`-60..0 dBFS -> 0..100%`) and ballistics
essentially as documented, as a new package/file in voxi (e.g.
`internal/audio` or a new `internal/miclive`), independent of the existing
sparkline code. Keeps voxi's implementation calibration-compatible with
harnez's if the two projects' mic-meter behavior should stay consistent
for the same person using both tools.

Whoever implements this should decide between A and B (or a hybrid) as
part of implementation — this ticket intentionally does not decide it.

## 5. Open Questions

- **Scale unit**: should voxi's live meter reuse its own linear-RMS/
  logarithmic-quantization calibration (already tuned against its own
  acoustic-gate thresholds, issue 070) or adopt harnez's dBFS-based
  `-60..0` scale? These are calibrated against different reference points
  and are not drop-in equivalent.
- **Capture backend**: does voxi already have a `pactl`/`parec` capability
  probe or backend-selection pattern (pactl vs. PipeWire-native vs.
  ALSA-only) to reuse, or does this need one built from scratch the way
  harnez's `mic.go`/`resolveMicBackend` does?
- **Configured volume source**: which command/API should voxi read the
  "configured" system input volume from (`pactl get-source-volume
  @DEFAULT_SOURCE@`, `wpctl get-volume`, or something else) — not yet
  investigated in this ticket.
- **Redraw cadence**: does `voxi monitor`'s existing refresh loop need the
  same normal/high-FPS decoupling harnez uses, or is voxi's existing
  refresh interval already fine for a meter box given voxi's simpler
  box set?
- **Privacy-indicator suppression**: in scope for a first implementation,
  or deferred? If deferred, should be called out explicitly rather than
  silently left as a known gap (an always-open, unlabeled `parec` stream
  could trigger a false "recording" indicator on GNOME/KDE desktops while
  `voxi monitor` is simply open, not while voxi is actually dictating).
- **Box placement/toggle**: should this be a new `ResourceSections` flag
  (a 5th box alongside Speed/Hardware/Transcript/Daemons) or folded into
  an existing box (e.g. Speed)? Not decided here.

## 6. Non-Goals (for now)

- Not scoped to change `voxi chunks list`'s existing RMS/LEVEL columns
  (issues 070-072) — those remain a separate, already-implemented,
  per-chunk historical feature.
- Not committing to porting harnez's privacy-indicator suppression tags in
  the first cut (see Open Questions).
- Not committing to a specific scale/unit or capture backend — see
  Proposed Approach and Open Questions above.

## 7. Background

Raised 2026-09-08 by the user as a forward-looking pointer ("see ../harnez
for how we aim to capture the audio level — volume slider and actual
real-time level"), not yet scoped into concrete acceptance criteria. Filed
as a feature request with open questions rather than a locked spec,
consistent with how the referenced harnez design doc itself is still
evolving (last updated 2026-09-07, per its own footer).
