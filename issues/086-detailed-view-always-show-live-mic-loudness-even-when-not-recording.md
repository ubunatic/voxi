# 086 — Detailed view: always show live mic loudness, even when not recording

**Status**: Closed — resolved via spec/monitor.yaml status_icon.always_show_loudness
**Priority**: P3 (Low)
**Severity**: Enhancement
**Category**: Enhancement
**Related**: [084 Add Live Mic Input-Level Meter and Volume Display to `voxi monitor`](084-add-live-mic-input-level-meter-and-volume-display-to-voxi-monitor.md), [internal/monitor/render.go](../internal/monitor/render.go), [audiolevel/audiolevel.go](../audiolevel/audiolevel.go)

---

## 1. Background

084 shipped a live mic-loudness meter (`audiolevel` package + an
`audiolevel.Manager` kept running for the life of `voxi monitor -w`).
During that work the loudness bar was folded directly into the `status:`
line's icon slot: while `RecordStatus == "recording"`,
`formatRecordState` (`internal/monitor/render.go`) swaps the `●` glyph for
`RenderLevelGauge(micLevel)`; while idle it still shows the static `○
idle`. A separate always-visible `level:` row was deliberately removed at
the user's request (see conversation on 2026-09-08) to keep the compact
view uncluttered.

## 2. Request

Add a detailed view mode that always shows the live loudness bar —
including while idle/not recording — instead of only while
`RecordStatus == "recording"`. This lets a user check "is my mic even
picking up sound" (levels, background noise, gain) without having to
start dictating first.

## 3. Open Questions (not decided here)

- New `ResourceSections` toggle (a "detailed" mode key, e.g. `l` for
  level) vs. an always-on second row that only appears in an expanded
  box height?
- Should idle still show `○` at rest and only switch to the bar once
  level crosses some noise-floor threshold, or should the bar itself
  replace `○` at 0% when idle (i.e. same code path as recording, just
  driven by `RecordStatus` no longer gating it)?
- Interaction with the one-shot (non-`--watch`) path, which currently
  never starts `audiolevel.Manager` (`cmd/voxi/main.go` passes
  `audiolevel.Reading{}`) — should a detailed one-shot view spin up a
  brief capture window instead, or is this feature `--watch`-only?

## 4. Non-Goals (for now)

- Not committing to a specific UI/toggle design — see Open Questions.
- Not reintroducing the removed compact-view `level:` row; this is about
  an opt-in detailed view, not changing the default compact status line
  shipped in 084/085.

## 5. Resolution — 2026-09-08

Resolved the Open Questions' second bullet directly rather than a separate
"detailed view" section: `formatRecordState`
(`internal/monitor/render.go`) now checks
`spec/monitor.yaml`'s new `status_icon.always_show_loudness` — when true
(the shipped default), the idle case renders `RenderLevelChar(micLevel) +
"idle"` instead of the static `○ idle`, i.e. exactly "same code path as
recording, just no longer gated by `RecordStatus`," reusing the existing
icon slot rather than adding a new row or `ResourceSections` key.

Left as-is / not addressed by this resolution:
- **One-shot path** (bullet 3): still unaffected — `cmd/voxi/main.go`'s
  one-shot invocation still passes `audiolevel.Reading{}`, so
  `always_show_loudness` has no visible effect outside `--watch`. Spinning
  up a brief capture window for one-shot mode remains unimplemented; file a
  follow-up if that's wanted.
- No noise-floor threshold gating was added (bullet 2's other half) — the
  glyph reflects the raw ballistics-smoothed level at all times, including
  near-zero ambient noise, rather than snapping back to `○` below a
  cutoff.

Toggle lives in `spec/monitor.yaml` (schema:
`spec/schemas/monitor.schema.json`), loader: `spec.LoadMonitor()` /
`spec.MonitorSpec.StatusIcon`. Verified live: `voxi monitor -w` shows a
colored loudness glyph in place of `○` while genuinely idle. `go test
./...` and `make check` pass.
