**Before any work, read all Harnez rules in one call: `harnez read .harnez/rules/Tools.md .harnez/rules/Issues.md .harnez/rules/Quota.md .harnez/rules/Subagents.md .harnez/rules/Output.md .harnez/rules/Local.md`. This follows Index.md order; Local.md overrides the other rules.**


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

Before removing or changing a file format (e.g. `corpus.tsv`), search the repo for every reader of
it — code, scripts, benches, testdata READMEs — and convert or ticket each one in the same change.

<!-- harnez:begin Language Conventions -->
Adhere to the following conventions.

Docs in `./docs/` are managed by harnez. <!-- harnez:bundled -->

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
- Issue Tracking Practices @docs/IssueTracking.md,
  P0-P3 priorities, metadata headers (Status, Priority, Severity, Category), tracker sync
- Make/Makefile @docs/Make.md,
  ⚙️ phony sentinel, self-doc help, build dependency pattern
- Man Pages for Go CLIs @docs/ManPages.md,
  cobra/doc GenManTree; <cmd> man / man --install; XDG ~/.local/share/man/man1
- Markdown @docs/Markdown.md,
  PascalCase for evergreens, kebab-case for ephemeral docs; ASCII art in chat, Mermaid only in docs/
- Search Practices @docs/Search.md,
  harnez find code/docs, finder configuration, partial results, and rg fallback
- Spec system @docs/Spec.md,
  YAML spec files as single source of truth; Go code must not duplicate spec values
<!-- harnez:end Language Conventions -->

## Workspace (uman)

Cross-project operations go through `uman`.
Project website publishes from `website/` via `uman website sync voxi`.

## Speech Samples

`~/.local/share/voxi/samples/{dictation,noise,voice}/` holds a developer machine's private
samples: one WAV plus one JSON sidecar (id, transcript, keyterms, source, consent) each, used
for ASR accuracy checks (`dictation`), no-speech checks (`noise`) and voice cloning (`voice`).
Use `voxi sample` to manage them and `internal/sample` to read them; never write them from
tests (use `t.TempDir()`). Terms and rules: `docs/SampleStore.md`. The legacy
`~/.config/voxi/samples/` (corpus.tsv) is read by nothing and must not be modified.
<!-- harnez:begin Repo Setup -->
## Repo Setup
- Solo/hobby repo — single default branch, no PR workflow.
- codeberg.org is primary; github.com (if present) is a synced mirror only.
<!-- harnez:end Repo Setup -->
