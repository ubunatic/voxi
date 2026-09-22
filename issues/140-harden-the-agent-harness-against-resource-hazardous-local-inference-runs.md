# 140 — Harden the agent harness against resource-hazardous local inference runs

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Major
**Category**: Process
**Related**: 134 §5 (the VRAM trap), 136 (the invisible status line), `@docs/Canary.md`, `@docs/AgenticLoop.md`, `@docs/ASREngines.md`

---

## 1. Problem

During the R2T2 integration (131/134/136) an agent-run `llama-server` was started
without a context cap. On this AMD Cezanne iGPU VRAM *is* system RAM, so it
inflated to **26.5 GiB VRAM / 99% RAM** and made the machine unusable; two
parallel `-j 12` llama.cpp builds separately pushed desktop apps into swap.

The knowledge to prevent this now exists in `@docs/ASREngines.md` and 134 §5, but
it lives only in prose an agent may not read. Four related weaknesses showed up in
the same session:

1. **No enforced cap.** The safe command is documented; nothing stops an agent
   typing the unsafe one.
2. **Canary discipline covers function, not resources.** `@docs/Canary.md` says
   probe a mechanism before building on it. The R2T2 canary proved transcription
   worked and never asked what it consumes unbounded. A single lucky run (8 GiB)
   was generalised as safe.
3. **UI changes were verified by assertion, not by looking.** 136 M2 shipped a
   status line that passed every test and was truncated out of view at a normal
   terminal width (fixed in M3).
4. **Ad-hoc agent briefs.** Resource limits, abort thresholds, one-process-at-a-
   time and kill-before-report had to be restated by hand per dispatch, and were
   forgotten exactly once — which was enough.

## 2. /goal

An agent cannot casually hang this machine with a local inference run, and a
visual change cannot ship unseen. Concretely, all four hold:

- The R2T2/llama-server backend is started through one repo-local entry point
  that carries the memory cap, so the unsafe invocation is not something an agent
  has to remember to avoid. (Note the overlap with issue **139**'s systemd unit —
  if both land, one of them owns the canonical command and the other references
  it; do not create two sources of truth.)
- `@docs/Canary.md` states a resource-envelope rule: before running an unfamiliar
  local model server, establish its memory ceiling and check consumption *after*
  startup, not only before.
- `@docs/AgenticLoop.md` gains a verification rule for TUI/UI output: rendered at
  realistic width and actually looked at, the sibling of the existing Media &
  Demo Verification Gate.
- A reusable agent-brief preamble for local-inference work exists (one process at
  a time, explicit caps, abort thresholds, kill-before-report, never a `pkill`
  pattern that can match the agent's own shell).

## 3. Notes / Uncertainties

- **Where the preamble lives is open**: `AGENTS.md`, a `docs/` evergreen, or a
  harnez-level skill so other projects inherit it. This hazard is not
  voxi-specific — any project running local models on this laptop has it.
- Keep the docs edits inside the intended blocks: much of `AGENTS.md` is
  harnez-managed and overwritten on `harnez init`.
- `docs/ModelRoles.md` is still untracked in this repo although it governed how
  this session was run. Decide whether it becomes a tracked evergreen; a fresh
  agent cannot follow a doc that is not there. Owner's call — it is not mine to
  commit.
- Prefer mechanisms over prose wherever a mechanism is cheap: a script that
  cannot be invoked wrongly beats a sentence an agent may skip.
