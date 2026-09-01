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

- Go/Golang @docs/Go.md,
  Modern Go, avoid deps but use Cobra, add tests
- Make/Makefile @docs/Make.md,
  ⚙️ phony sentinel, self-doc help, build dependency pattern
- Markdown @docs/Markdown.md,
  PascalCase for evergreens, kebab-case for ephemeral docs
- Git @docs/Git.md,
  conventional commits, work on the default branch, don't push unless asked
- Canary-first development @docs/Canary.md,
  probe external mechanisms before building features on them
- Spec system @docs/Spec.md,
  YAML spec files as single source of truth; Go code must not duplicate spec values
- Agentic Loop Practices @docs/AgenticLoop.md,
  5-phase loop (Advisory -> Dev -> Review -> Hygiene -> Retro), zero zombie guarantee
- Issue Tracking Practices @docs/IssueTracking.md,
  P0-P3 priorities, metadata headers (Status, Priority, Severity, Category), tracker sync
<!-- harnez:end Language Conventions -->

## Workspace (uman)

Cross-project operations go through `uman`.
Project website publishes from `website/` via `uman website sync voxi`.
<!-- harnez:begin Repo Setup -->
## Repo Setup
- Solo/hobby repo — single default branch, no PR workflow.
- codeberg.org is primary; github.com (if present) is a synced mirror only.
<!-- harnez:end Repo Setup -->
