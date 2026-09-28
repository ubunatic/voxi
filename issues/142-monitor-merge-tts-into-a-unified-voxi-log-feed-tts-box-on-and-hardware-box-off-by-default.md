# 142 — Monitor: merge TTS into a unified voxi log feed, TTS box on and hardware box off by default

**Status**: Closed — done per user review 2026-09-28
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: UX / Monitor
**Related**: 141 (TTS MVP: the monitor-gated queue and the current TTS panel), `spec/actions.yaml` (`hardware`, `transcript` panel toggles)

---

## 1. Problem

Since 141, `voxi monitor -w` shows TTS in its own panel at the bottom:
state, the `Now:` line, the `Queue:` line, first-audio time and a key legend.
That panel is clunky, and it sits apart from the transcript feed, which
already shows what voxi is doing with speech *input*. The user has to look in
two places to see what voxi is processing.

## 2. /goal

`voxi monitor` has one clean UI:

- **One voxi log.** The transcript feed becomes a combined feed that shows
  speech input (dictated chunks) *and* speech output (TTS chunks as they are
  synthesized and played), in order, with the two directions easy to tell
  apart. The chunk currently being processed is visible there.
- **A compact TTS box** replaces the bottom TTS panel with its `Now:`/`Queue:`
  lines. It holds the TTS state, the queue count and the controls, and it is
  **shown by default**.
- **The hardware box is hidden by default** and can still be switched on with
  its existing toggle.

Done when the old bottom TTS panel is gone and the defaults match the above.
There are render tests for the combined feed and the new defaults, and the user
confirms the layout live.

## 3. UI Decisions

- Feed rows use explicit `[IN]` and `[OUT]` prefixes, cyan for input and green
  for output. The text labels keep direction clear without relying on colour.
- Each output chunk has one row, created when synthesis starts and updated in
  place through `synthesizing`, `playing`, then `played`, `skipped` or `stopped`.
  Terminal outcome rows remain visible in the feed for the monitor session.
- Keep the latest 10 output rows, matching the eager transcript history cap.
  Merge them with the recent input rows and order by each row's creation time.
- Long rows are truncated to the available terminal width with `...`.
- The compact TTS box shows playback state, queued chunk count, current `Now:`
  text, backend/first-audio details when available, and existing controls. It
  does not preview each queued chunk.
- Defaults are sourced from `spec/actions.yaml`: speed, transcript feed,
  daemons, and TTS box on; hardware box off. The existing `h` toggle remains,
  and `y` toggles the TTS box.

## 4. Preserved behavior

- Keep the 141 behaviour: the monitor keys, MPRIS, quit-stops-everything, and
  lines that clear to the end on redraw.
- Panel toggles and their defaults live in `spec/actions.yaml`. Change the
  defaults there rather than in Go (see `@docs/Spec.md`).
- Before starting, re-check the live code, since the monitor changes often.

## 5. User live check (2026-09-24): required follow-ups
- **The `[t]` transcript feed shows 4 lines again**, as before 142. The merged
  IN/OUT feed must not grow taller. The 10-row OUT cap is about history, not
  visible height.
- **The TTS box takes the hardware box's old position** in the layout.
  Hardware stays hidden by default; when toggled on, it goes where the TTS box
  was.
