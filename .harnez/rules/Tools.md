# Harnez Tools

Managed by harnez — local edits here are overwritten on the next `harnez init`.
Put tool-neutral project rules in AGENTS.md and Harnez-only rules in .harnez/rules/.

## Tool Availability
If `harnez` is not installed or available in PATH, install it via:
```bash
go install ubunatic.com/harnez/cmd/harnez@latest
```

## Always `make install`
After every change to a project that has a `make install` target, run `make install` before
reporting or committing, so the user's installed binary always matches the code. This applies in
every repo to solo developers, host sessions that talk to a human, and orchestrators. Leaf developer
subagents in a sprint skip it unless their skill requires it at the end; the host installs after review.

## Editing Discipline
- Prefer structured patch tools (`apply_patch`) or whole-block replacements over
  narrow string substitution edits.
- When making multi-line edits, ensure sufficient surrounding context lines to
  avoid ambiguous pattern matches.
- **Reading & Context Discipline (Recommended for Large Files)**: Prefer
  `harnez read -L <range>` / `harnez read -n` for medium/large files (>100 lines)
  (`harnez read -I` is paused until issue 543, a memory blow-up, is fixed)
  to preserve token quota and prevent context fatigue. Native reads remain valid
  for targeted inspection; hook-level blocking is conditional on the active
  `reading_discipline.enforce` mode in `~/.harnez/config.yaml` (or
  `HARNEZ_READ_ENFORCE`).

## Issue Tracker Discovery (harnez find)
Applies when this project has an `issues/` tracker. To search existing issues,
compute the next ticket number, or allocate one, use `harnez find` / `harnez issues`
instead of `ls issues/`, `find`, or raw grep:
- `harnez find -d <repo> issues -a status:open` — list active open issues (use `-I` for visual overview PNG card)
- `harnez find -d <repo> issues "<query>"` — fuzzy search across titles and body text (use `-I` for visual overview)
- `harnez issues show -d <repo> <n> -I` — render single issue as styled visual PNG card (inspect via `view_file`)
- `harnez find -d <repo> issues next` — report the next free ticket number (read-only)
- `harnez issues new -d <repo> "<title>"` — run this right away; it reserves the number and creates the ticket skeleton.
- Open the printed file and fill it in, keeping its schema; do not search for a template first.
- Run `harnez index -d <repo>` and commit the ticket and index.
- `harnez issues <verb> -d <repo> <n> [reason]` — change a ticket's status, resync
  `issues/README.md`, and commit, in one call
- Commit documentation and `issues/*.md` changes immediately; don't batch them behind
  pending code work.

## Harnez Agent
- Prefer loaded `mcp__harnez__*` tools for lifecycle actions; otherwise use `harnez agent` via Bash (see the local Subagent Policy).
- A requested model such as `terra:low` or `luna` is a Harnez agent model (see `harnez agent models`); dispatch it with `harnez agent start --model <name>`, regardless of `subagent_mode`.
- Start (run it in a background shell, e.g. Claude `run_in_background`): `harnez agent start --name <name> --role <role> --model <model> -p <prompt>`.
- List: `harnez agent list`.
- Status: `harnez agent status --name <session>`.
- Wait: `harnez agent wait <session>` (session is positional).
- Resume: `harnez agent resume --name <session> <prompt>`.
- Stop: `harnez agent stop --name <session>`.
- When a `/goal` without an exit clause is set (e.g. typed by the user), say so in the first reply and offer `/goal ... or stop and report when blocked on a user decision or denied permission`; once blocked on the user, suggest `/goal clear` instead of repeating the wait message.

## External Skills
External skills (installed with `harnez skill install`) are explicit-only: never use one
unless the user names it (e.g. `/scroll-craft`, "use scroll-craft") or picks it after you ask.
- When a task could fit an external skill and the user named none, run
  `harnez skill search <topic>`. If anything matches, list the matches with their one-line
  descriptions and ask which to use, or none. Never pick one yourself.
- Agents without a native copy (Gemini, Prime) load the chosen skill with
  `harnez skill show <name>`.

## Code and Documentation Search
- Before broad shell searches, use `harnez find code|docs` or MCP `harnez_find`; see `@docs/Search.md`.

## Agentic Loop Invariants
Where `@docs/AgenticLoop.md` is present in this project, follow it rather than
restating it here — in particular Invariant 1 (Parallel Read, Sequential Write:
one writer per workspace), Invariant 3 (Zero Zombie Guarantee: track and terminate
every background task and subagent), and Invariant 6 (Context Discipline: no whole-file
reads of AGENTS.md/CLAUDE.md — grep or range-bounded reads).
