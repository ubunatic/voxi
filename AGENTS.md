<!-- harnez:begin Local Overlays -->
- **Before any work, read `AGENTS.local.md` if it exists** (@AGENTS.local.md). It holds this
  checkout's settings (subagent mode, output mode) and overrides this file where they differ.
<!-- harnez:end Local Overlays -->

Adhere to the following conventions.

<!-- harnez:begin Project Summary -->
## Project Summary

`voxi` is a standalone voice input, continuous eager sentence streaming, and
desktop typing engine for Linux/Wayland. It provides kernel evdev modifier gating
(`voxi-modifierd`), direct keystroke injection (`dotool`), a btop-style TUI
resource monitor (`voxi monitor`), and a GNOME Shell companion extension.
<!-- harnez:end Project Summary -->

## Development Scripts

Run from project root.

After making changes, always run `make install` so installed binaries are current.

`make install` does not affect the running `voxi-agent.service` (systemd --user) —
it keeps executing the old binary already loaded in memory until restarted.
If you touched code the live daemon actually runs (`internal/eager`, `internal/record`,
`internal/modifiers`, `cmd/voxi`'s `agent`/`eager` paths, etc.), run
`make restart-service` instead of `make install` — it rebuilds, installs, and
restarts `voxi-agent.service` so the change takes effect. Purely on-demand
subcommands invoked fresh each run (e.g. `voxi monitor`) don't need a restart.

<!-- harnez:begin Language Conventions -->
Adhere to the following conventions.

Docs in `./docs/` are managed by harnez. <!-- harnez:bundled -->

- Issue Tracking Practices @docs/IssueTracking.md,
  P0-P3 priorities, metadata headers (Status, Priority, Severity, Category), tracker sync
- Agentic Loop Practices @docs/AgenticLoop.md,
  5-phase loop (Advisory -> Dev -> Review -> Hygiene -> Retro), zero zombie guarantee
- Bash/Shell @docs/Bash.md,
  Read before multi-line shell: Make recipes, embedded scripts
  No ";", break before then/else/docs
  No "if [[]]", No "if []", Use "if test"
  smart indent!
  Use git -C/make -C, not cd
- Canary-first development @docs/Canary.md,
  probe external mechanisms before building features on them
- Git @docs/Git.md,
  conventional commits, work on the default branch, don't push unless asked
- Go/Golang @docs/Go.md,
  Modern Go, avoid deps but use Cobra, add tests; use runes and display width for terminal layout
- Release Pipeline @docs/GoRelease.md,
  harnez release, version.yaml spec, GoReleaser v2, non-interactive minisign (-W), Forgejo has_releases, language-agnostic (Go/Python/Zig/Rust/scripted)
- Make/Makefile @docs/Make.md,
  ⚙️ phony sentinel, self-doc help, build dependency pattern
- Man Pages for Go CLIs @docs/ManPages.md,
  cobra/doc GenManTree; <cmd> man / man --install; XDG ~/.local/share/man/man1
- Markdown @docs/Markdown.md,
  PascalCase for evergreens, kebab-case for ephemeral docs; ASCII art in chat, Mermaid only in docs/
- Spec system @docs/Spec.md,
  YAML spec files as single source of truth; Go code must not duplicate spec values
- Search Practices @docs/Search.md,
  harnez find code/docs, finder configuration, partial results, and rg fallback
<!-- harnez:end Language Conventions -->

## Workspace (uman)

Cross-project operations go through `uman`.
Project website publishes from `website/` via `uman website sync voxi`.

## Speech Samples

`~/.config/voxi/samples/` usually holds a developer machine's private `corpus.tsv`
(id, wav, expected transcript, keyterms) plus its WAVs — a local, uncommitted
labelled corpus for ASR accuracy checks.
<!-- harnez:begin Repo Setup -->
## Repo Setup
- Solo/hobby repo — single default branch, no PR workflow.
- codeberg.org is primary; github.com (if present) is a synced mirror only.
<!-- harnez:end Repo Setup -->
<!-- harnez:begin Harnez Managed Conventions -->
## Harnez Managed Conventions

Managed by harnez — local edits here are overwritten on the next `harnez init`.
Put project-specific rules outside this block.

### Tool Availability
If `harnez` is not installed or available in PATH, install it via:
```bash
go install ubunatic.com/harnez/cmd/harnez@latest
```

### Always `make install`
After every change to a project that has a `make install` target, run `make install` before
reporting or committing, so the user's installed binary always matches the code. This applies in
every repo to solo developers, host sessions that talk to a human, and orchestrators. Leaf developer
subagents in a sprint skip it unless their skill requires it at the end; the host installs after review.

### Editing Discipline
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

### Issue Tracker Discovery (harnez find)
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

### Harnez Agent
- Prefer loaded `mcp__harnez__*` tools for lifecycle actions; otherwise use `harnez agent` via Bash (see the local Subagent Policy).
- Start: `harnez agent start --detach --name <name> --role <role> --model <model> -p <prompt>`.
- List: `harnez agent list`.
- Status: `harnez agent status --name <session>`.
- Wait: `harnez agent wait <session>` (session is positional).
- Resume: `harnez agent resume --name <session> <prompt>`.
- Stop: `harnez agent stop --name <session>`.

### Code and Documentation Search
- Before broad shell searches, use `harnez find code|docs` or MCP `harnez_find`; see `@docs/Search.md`.

### Agentic Loop Invariants
Where `@docs/AgenticLoop.md` is present in this project, follow it rather than
restating it here — in particular Invariant 1 (Parallel Read, Sequential Write:
one writer per workspace), Invariant 3 (Zero Zombie Guarantee: track and terminate
every background task and subagent), Invariant 6 (Context Discipline: no whole-file
reads of AGENTS.md/CLAUDE.md — grep or range-bounded reads), and the Media & Demo
Verification Gate (explicit user confirmation before publishing
recordings or screenshots).
<!-- harnez:end Harnez Managed Conventions -->
<!-- harnez:begin Quota-1 Guardrails -->
## Quota-1 Guardrails

- **Single-Test Boundary**: Under Quota-1 rules, the agent may only run the test suite once per step/turn.
- **Code Modification Required**: If tests fail or complete, you MUST modify repository source files before running tests again. Repeated test runs without intermediate code modifications are blocked.
- **Clean Tree First**: Before the quota run, check `git status` for changes that are not yours. If there are any, wait or report; do not run.
- **Enforced Test Target**: Execute tests via `make test-q1` (or `harnez exec --quota-1 -- <test-cmd>`).
- **Unauthorized Bypass Forbidden**: Bypassing guardrails via `QUOTA_BYPASS=1` or `HARNEZ_QUOTA_BYPASS=1` is strictly reserved for human developers and CI environments. Agent loops must not set or pass bypass flags.
<!-- harnez:end Quota-1 Guardrails -->
