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

<!-- harnez:begin Language Conventions -->
Adhere to the following conventions.

Docs in `./docs/` are managed by harnez. <!-- harnez:bundled -->

- Go/Golang @docs/Go.md,
  Modern Go, avoid deps but use Cobra, add tests
- Markdown @docs/Markdown.md,
  PascalCase for evergreens, kebab-case for ephemeral docs
- Git @docs/Git.md,
  conventional commits, work on the default branch, don't push unless asked
- Canary-first development @docs/Canary.md,
  probe external mechanisms before building features on them
- Spec system @docs/Spec.md,
  YAML spec files as single source of truth; Go code must not duplicate spec values
- Make/Makefile @docs/Make.md,
  ⚙️ phony sentinel, self-doc help, build dependency pattern
<!-- harnez:end Language Conventions -->

## Workspace (uman)

Cross-project operations go through `uman`.
Project website publishes from `website/` via `uman website sync voxi`.
<!-- harnez:begin Repo Setup -->
## Repo Setup
- Solo/hobby repo — single default branch, no PR workflow.
- codeberg.org is primary; github.com (if present) is a synced mirror only.
<!-- harnez:end Repo Setup -->
