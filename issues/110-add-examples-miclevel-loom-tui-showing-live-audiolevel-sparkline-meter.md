# 110 — Add `examples/miclevel`: loom TUI Showing Live `audiolevel` Sparkline/Meter

**Status**: Closed
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Feature
**Related**: `audiolevel/audiolevel.go`, `audiolevel/sparkline.go`, issue 109 (export streaming sparkline), `../loom` (`pane.go` `RunWatch`, `collector/`, `frame.go` `Box.SetRowsValues`), `../loom/examples/monitor`, `../loom/issues/015` (simulated Voxi panels — explicitly scoped to fake data, no real mic), `../loom/issues/039` (SubChar boundary-glyph seam, filed from this ticket's own verification), `../harnez/issues/330` (adopt this ticket's sparkline capability into harnez's own mic meter)

---

## Resolution (2026-09-13)

Shipped `examples/miclevel/` (`main.go`, `watch.go`, `spec/miclevel.yaml`,
`README.md`), wired via a real tagged dependency
(`codeberg.org/ubunatic/loom v0.2.1` in `go.mod`, no `replace`) plus a
tracked `go.work.example` (copy to untracked `go.work` for local
uncommitted-loom-checkout co-development — see
[docs/Go.md](../docs/Go.md) "Workspace Isolation").

Manually verified against a real microphone (not simulated): the level bar
and Braille sparkline both respond to ambient room noise/speech and decay
afterward; the no-backend degrade path (PATH stripped of `parec`/
`pw-record`) renders a clear message with no panic. `go build`/`go vet`/
`go test ./...` all green.

Two real bugs were found and fixed along the way (both root-caused before
being patched — see [docs/LiveMicMeter.md](../docs/LiveMicMeter.md) §12 for
the full writeup, kept there rather than only in this closed ticket since
the pitfalls are `loom`-API-wide, not specific to this one example):
1. A box with too little declared height for its padding silently dropped
   **all** child content (not just the dynamic parts) — a `loom` API
   footgun (`Box.Draw`'s `padding < (h-1)/2` guard), not anything specific
   to this ticket's YAML.
2. `SparklineStream.Sparkline()` returns a space-*padded* string (not `""`)
   before its buffer has real data — an easy off-by-assumption bug in any
   consumer, not just this one.

One cosmetic `loom` gap (`graph.RenderBar`'s `SubChar` boundary glyph shows
a terminal-font-rendering seam without `ANSI`+`BackgroundANSI` styling) was
filed upstream as `../loom` issue 039 rather than worked around locally —
per explicit user direction, `examples/miclevel` keeps `SubChar: true` to
match `examples/monitor`'s own established convention and lives with the
seam until loom addresses it.

Also filed `../harnez` issue 330: harnez already shares this same
`audiolevel.Meter`/`Manager` for its own scalar mic-level bar and is a
natural adopter of the new sparkline capability this ticket demonstrated —
blocked on a new voxi release tag past v0.1.7 (which predates the
sparkline export).

---

## 1. Problem & Motivation

`audiolevel` (issue 109) now exports `RenderSparkline`, `SparklineStream`, and
`Meter`/`Manager` sparkline integration as a public Go library — but nothing
in this repo demonstrates it end to end against a *real* microphone in a
terminal UI. The only consumers today are library-internal tests and
`voxi monitor` (which uses its own bespoke rendering, not `loom`).

Separately, `../loom` (Voxi's sibling app/TUI SDK project) has built exactly
the primitives this needs — `Pane.RunWatch` (independent collect/redraw
timers), the `collector` package (bounded, nonblocking periodic collection
into a sink), and `Box.SetRowsValues` (dynamic row content) — but loom's own
`examples/monitor` and its roadmap (`../loom/issues/015`) deliberately stay on
**simulated** data; 015's scope limits explicitly exclude "microphone" and
"changes to Voxi". There is currently no example anywhere that wires loom to
a real, live data source.

An `examples/` app in this repo that captures real mic audio via
`audiolevel.StartManager`/`Meter.EnableSparkline` and renders it live through
`loom` closes that gap: it proves the `audiolevel` public API is actually
consumable by an external Go program (not just internal Voxi code), and it
gives `loom` its first real (non-simulated) external-source integration test.

---

## 2. Scope

### In scope
- A new `examples/miclevel/` Go program (own `main.go`, matching the shape of
  `../loom/examples/monitor`) that:
  - Starts live capture via `audiolevel.StartManager` (or `RunCapture`
    directly) using `ParecCommand`/`PwRecordCommand`.
  - Enables the rolling sparkline (`Meter.EnableSparkline`) and reads
    `Manager.Snapshot()` / `Manager.Sparkline()` on each collection tick.
  - Renders a live-updating inline pane via `loom.Pane.RunWatch`: a scalar
    level meter/bar and the scrolling Braille sparkline, redrawn
    independently of the (slower) capture cadence per loom's collect/redraw
    split.
  - Handles the no-audio-backend case gracefully (loom's `StartManager`
    already degrades to `Reading{Available: false}` when no `parec`/
    `pw-record` is found — the example must show that state, not crash).
  - Quits cleanly on `q`/Ctrl-C, restoring the terminal (mirrors loom's
    `Close`-before-stdout convention).
- A short `examples/miclevel/README.md` (matching `../loom/examples/monitor/README.md`'s
  style): how to run it, what it shows, and a note on privacy (it opens a
  real mic capture stream — same `SuppressApplicationID` mic-indicator
  exemption `audiolevel` already uses elsewhere in this repo).
- Wiring `go.work`/`replace` (or the sibling-module convention this repo
  already uses, if any) so `examples/miclevel` can depend on `../loom` during
  local development. Check how `../loom`'s own `go.work.example` and this
  repo's existing module setup already handle sibling-project dependencies
  before inventing a new mechanism.

### Out of scope
- Any change to `voxi monitor`'s existing (non-loom) rendering — this is a
  standalone example, not a replacement.
- Any change to `loom`'s public API to accommodate this example, *unless* a
  real gap is found while building it (see §4).
- Publishing `examples/miclevel` as an installed subcommand of `voxi` — it's
  a library-usage demo, not a shipped feature.

---

## 3. Acceptance Criteria

- [x] `go run ./examples/miclevel` opens an inline loom pane showing a live
      scalar level and scrolling Braille sparkline that visibly respond to
      real microphone input (manually verified against a real mic, not just
      unit-tested).
- [x] Gracefully degrades (visible "unavailable" state, no panic) when no
      capture backend (`parec`/`pw-record`) is present.
- [x] `q`/Ctrl-C exits cleanly and restores the terminal.
- [x] `examples/miclevel/README.md` documents how to run it and what it
      demonstrates.
- [x] `go build ./...`, `go vet ./...`, `go test ./...` stay green with the
      new example in the tree.

---

## 4. Implementation Notes

- Read `../loom/examples/monitor` (`main.go`, `spec/watch.yaml`,
  `spec/monitor.yaml`) first for the established example shape and the
  `RunWatch`/`Cadence`/collect-callback pattern before designing this one —
  don't reinvent conventions loom's own example already established.
- If `loom`'s current API (`RunWatch`, `collector.Collector`,
  `Box.SetRowsValues`, or the sparkline/graph widget surface) turns out to be
  missing something this example genuinely needs (e.g. no widget suited to
  rendering a pre-rendered Unicode sparkline string, or `RunWatch`'s
  collect/redraw split not fitting a mic-cadence source cleanly), **file a
  loom issue** (`harnez issues new -d ../loom "<title>"`) describing the
  concrete gap rather than working around it silently in this example — per
  the original request driving this ticket.
- `audiolevel.SparklineOptions.SampleRate` must match whatever sample rate
  the capture command actually streams at (`ParecCommand`/`PwRecordCommand`'s
  `sampleRate` argument) — mismatched values silently skew the sliding
  window's time span.

## 5. Verification

Manual: run `go run ./examples/miclevel` with a real microphone, speak at
varying volumes, and confirm both the scalar meter and sparkline respond and
decay appropriately; also run with no mic/backend available to confirm the
degraded-state path. Automated: `go build`/`go vet`/`go test` across the
repo with the new example present.
