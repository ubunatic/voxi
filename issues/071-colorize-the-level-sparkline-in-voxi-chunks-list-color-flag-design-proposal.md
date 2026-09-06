# 071 — Colorize the LEVEL Sparkline in `voxi chunks list` (`--color` flag, design proposal)

**Status**: Proposed
**Priority**: P3 (Low)
**Severity**: Enhancement
**Category**: Feature
**Related**: [070 Show Recorded Volume (RMS) and a Speech-Level Sparkline in voxi chunks list](070-show-recorded-volume-rms-column-in-voxi-chunks-list.md), [internal/chunks/command.go](../internal/chunks/command.go), [internal/audio/audio.go](../internal/audio/audio.go), [internal/monitor/render.go](../internal/monitor/render.go)

---

## 1. Problem & Motivation

Issue 070 added a `LEVEL` column to `voxi chunks list`: a fixed-width,
bracketed Braille sparkline (`[⣀⣤⣶⣿...]`) showing per-bucket speech volume
across a chunk's duration. It renders in plain, uncolored text. The user
asked: "think of how we can use `--color` in the sparkline (default: off,
default on when interactive terminal is detected)."

This is explicitly a **"think of how"** request — the user wants the
design space explored and posed as questions, not a single answer
prescribed and implemented. This ticket is a proposal/design document,
not an implementation ticket (see Non-Goals).

## 2. Investigation Findings

Grounding for the proposal below, gathered by reading the actual code
rather than assuming a pattern:

- **No existing color/TTY-detection abstraction in this repo.** There is
  no `internal/color` (or similarly named) package, and no
  `isatty`/`term.IsTerminal`-gated color-enable logic anywhere in
  `internal/`. Two different things exist that could be mistaken for
  this:
  - `internal/devsample/lineedit.go:145-156` has `stdinIsTerminal`,
    calling `golang.org/x/term.IsTerminal(int(f.Fd()))` — but this checks
    **stdin**, to decide whether interactive line-editing is available; it
    is not a stdout-color gate and isn't reused for output formatting
    anywhere.
  - `internal/asr/asr.go`'s `StripANSI`/`ansiEscapeRe` strips ANSI
    sequences out of *subprocess* (voxtype/whisper) output, and
    `internal/eager/eager.go:379` / `internal/devsample/transcribe.go:65`
    set `NO_COLOR=1` when *invoking* that subprocess — i.e. today color is
    only ever suppressed on inputs from elsewhere, never emitted or gated
    by voxi's own output.
- **Raw ANSI escapes are already used extensively, unconditionally, with
  no TTY gating at all.** `internal/monitor/render.go` and
  `internal/monitor/collector.go` hardcode `\x1b[32m`, `\x1b[33;1m`,
  `\x1b[90m`, etc. throughout the `voxi monitor` TUI (which assumes an
  interactive terminal by nature — it's a full-screen btop-style
  display, using `\033[2J`/`\033[H` cursor control in
  `internal/monitor/monitor.go:124-136`). `internal/eager/eager.go:447,600`
  and `internal/debug/debug.go:178-183` also print hardcoded
  `\x1b[32;1m`/`\x1b[90m`/`\x1b[33m` sequences unconditionally to
  `d.Stdout` in normal (non-monitor) command output, with **no
  isatty/NO_COLOR check before emitting them** — these are the closest
  existing "precedent" for coloring stdout, but they're not a template
  worth reusing as-is, since none of them gate on terminal detection
  either. `internal/monitor/render.go:184` (`StringDisplayWidth`) already
  has to strip ANSI codes via `asr.StripANSI` to compute visual column
  widths correctly — i.e. this repo has already hit (and solved, for the
  monitor's own box-drawing) the "raw escapes break width math" problem,
  which the `chunks list` table's fixed-width columns would hit too if
  color codes were inserted into `LEVEL` without similar care.
  So: **there is prior art for hand-rolled ANSI, but no prior art for
  conditional/gated color** — this ticket would be establishing that
  pattern for the first time, not following one.
- **Dependency policy**: `go.mod` currently has three direct
  dependencies: `github.com/spf13/cobra`, `golang.org/x/term`, and
  `gopkg.in/yaml.v3`. `docs/Go.md` states "Standard library first; every
  dependency needs a reason and approval" and "No Heavy Frameworks."
  `golang.org/x/term` — already a direct dependency, imported today only
  for stdin's `IsTerminal` in `devsample/lineedit.go` — already provides
  `term.IsTerminal(int(fd))` for **stdout** as well (it takes any fd), so
  detecting "is stdout an interactive terminal" needs **zero new
  dependencies**: `term.IsTerminal(int(os.Stdout.Fd()))`. Color output
  itself needs no library either — the reprieve of adding `fatih/color`
  or similar would go against the "avoid deps" convention when a handful
  of raw `\x1b[3xm...\x1b[0m` constants (consistent with the
  `internal/monitor` and `internal/eager` precedent above) would suffice
  and keep the whole feature within existing dependency policy.
- **`voxi chunks show`'s text format has no sparkline display at all
  today.** `FormatChunkDetails` (`internal/chunks/command.go:122-144`)
  prints `Mean / Peak RMS: %d / %d` but does **not** print
  `c.VolumeSparkline` in any form — issue 070 explicitly scoped that as
  a non-required "natural small extension" it left undone. So there is
  currently no second call site to keep in sync; if `show` never grows a
  sparkline line, this ticket's color logic only needs one call site
  (`list`'s row-printing loop, `internal/chunks/command.go:69`). If a
  future ticket adds the sparkline to `show`, whatever color-rendering
  helper this ticket produces should be reusable there too.

## 3. Scope (proposal only — see Non-Goals)

Explore, as open design questions for a future implementation ticket:

1. **Flag semantics** — what should the flag look like?
   - A tri-state `--color=auto|always|never` string flag, matching
     `git`/`ls`/`grep`'s convention exactly (`auto` = TTY-detect,
     default).
   - A simpler `--color` bool flag that defaults to auto-detecting via
     `term.IsTerminal(int(os.Stdout.Fd()))` when unset, with the bool
     acting as a forced override only when explicitly passed (harder to
     represent "unset vs. explicitly false" with `cobra`'s plain
     `BoolVar`, which needs `Changed` inspection on the flag to
     distinguish "user didn't pass it" from "user passed `--color=false`").
   - Whether `NO_COLOR` (already a convention this codebase itself sets
     for subprocesses, see Investigation) should also be honored as an
     environment-level override for voxi's own output, for consistency.
   - Whether the flag lives on `chunks` (`cmd.PersistentFlags()`, shared
     by `list`/`show`/`play`) or narrowly on `listCmd` alone
     (`internal/chunks/command.go:75`) — given Investigation found `show`
     doesn't render a sparkline yet, a persistent flag would be forward
     -looking but currently only affect `list`.
2. **What gets colored, and by what scheme** — genuinely open, per the
   user's "think of how":
   - **Per-glyph height coloring**: color each Braille glyph in the
     `LEVEL` sparkline by its own quantized level
     (`sparklineMinLevel..sparklineLevels`, `internal/audio/audio.go`) —
     e.g. dim/gray for the minimum `⣀`, ramping to a brighter color or
     different hue for `⣤`/`⣶`/`⣿`. Mirrors the existing height-based
     glyph selection one-for-one; requires inserting a color code before
     each glyph and a reset after, which increases the byte length of
     the `LEVEL` string but must not affect its rendered visual width
     (see `StringDisplayWidth`/`TruncateLineANSI`-style handling in
     Investigation — the fixed-width `%-10s`/`%-12s` printf formatting
     used today, `internal/chunks/command.go:69`, does not account for
     invisible ANSI bytes, so column alignment would break without care).
   - **Whole-sparkline coloring by accept/reject outcome**: color the
     entire bracketed `LEVEL` string (or just its brackets) based on
     `c.Accepted`/`c.RejectionReason` — e.g. red/dim for
     `rej:low_energy_transient` rows, normal or green-tinted for accepted
     ones. Orthogonal to per-glyph coloring; the two could combine (outcome
     sets a base hue, height sets brightness/intensity) or compete for
     the same terminal attribute.
   - Whether `STATUS` itself (`accepted` vs. `rej:...`,
     `internal/chunks/command.go:48-55`) should also get colored for
     consistency — arguably a more obviously useful target for
     accept/reject coloring than the sparkline, since it's the actual
     outcome column. Worth deciding whether this ticket's future
     implementation should stay narrowly scoped to `LEVEL` as literally
     asked, or extend to `STATUS` — posed here as an open question, not
     decided.
3. **Mechanism** — assuming no new dependency (see Investigation),
   sketch a small `internal/chunks` or `internal/audio` helper (e.g.
   `colorize(s string, code string) string` wrapping
   `"\x1b[" + code + "m" + s + "\x1b[0m"`, only invoked when the resolved
   color mode says on) and a
   `shouldUseColor(mode string, out *os.File) bool` resolver
   implementing the `auto`/`always`/`never` decision, callable from
   `internal/chunks/command.go`.

## 4. Acceptance Criteria (for a future implementation ticket, not this one)

This ticket has no acceptance criteria of its own beyond being read and
used to scope that follow-up ticket — see Non-Goals. A follow-up
implementation ticket should decide and state:

- The final flag shape and default (auto/always/never vs. bool+auto).
- Whether `NO_COLOR` is honored.
- The exact color scheme (per-glyph height, outcome-based, both, or
  neither) and which column(s) it applies to.
- How fixed-width column alignment is preserved once ANSI bytes are
  inserted into `LEVEL` (and `STATUS`, if in scope).
- Test coverage proving color codes appear/disappear correctly under
  forced `always`/`never` and are visually neutral (assert on the
  ANSI-stripped content, per `asr.StripANSI`, so no test becomes
  brittle against exact color codes without reason).

## 5. Non-Goals

- **No implementation in this ticket.** The user asked to "think of how,"
  not to build it — this is a design/proposal ticket. A follow-up
  ticket should pick one of the flag-semantics and color-scheme options
  above and implement it.
- **No decision on a specific color scheme.** Per-glyph height coloring,
  outcome-based coloring, and coloring `STATUS` are all posed as open
  questions above, not committed to.
- **Not expanding scope to recolor the entire `chunks list` table.**
  Investigation did surface that `STATUS` coloring is arguably more
  useful than `LEVEL` coloring, but that is noted only as an open
  question in Section 3, not a scope expansion — a future ticket should
  make that call explicitly rather than inheriting it as a foregone
  conclusion from this one.
- **No new third-party dependency.** Investigation confirms
  `golang.org/x/term` (already a direct dependency) covers TTY detection
  and hand-rolled ANSI escapes (consistent with existing
  `internal/monitor`/`internal/eager` precedent) cover coloring; adding
  `fatih/color` or similar would go against `docs/Go.md`'s
  "avoid deps" convention for a need this small.

## 6. Background

Raised 2026-09-06, immediately after issue 070 (RMS column + LEVEL
sparkline) was implemented and iteratively fixed through several
real-usage calibration rounds in the same session. The user's own
framing — "think of how" — signals this is meant as an open design
exploration to seed a future ticket, not a request to ship a specific
color scheme now.
