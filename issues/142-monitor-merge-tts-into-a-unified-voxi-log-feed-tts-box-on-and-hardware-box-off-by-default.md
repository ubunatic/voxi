# 142 — Monitor: merge TTS into a unified voxi log feed, TTS box on and hardware box off by default

**Status**: Open
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

## 3. Notes / Uncertainties

- Open questions: how input and output are marked in the log (prefix, colour
  or icon); whether played, skipped and stopped TTS chunks stay in the log; and
  how much text a long TTS chunk shows. Settle them with the user if they are
  not obvious.
- Keep the 141 behaviour: the monitor keys, MPRIS, quit-stops-everything, and
  lines that clear to the end on redraw.
- Panel toggles and their defaults live in `spec/actions.yaml`. Change the
  defaults there rather than in Go (see `@docs/Spec.md`).
- Before starting, re-check the live code, since the monitor changes often.
