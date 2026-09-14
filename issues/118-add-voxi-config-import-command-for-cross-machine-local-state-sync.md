# 118 — Add voxi config import command for cross-machine local state sync

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Feature
**Related**: [117 sample import](117-add-voxi-feedback-sample-import-command-for-cross-machine-sample-transfer.md), [119 consolidate voxi local config storage formats](119-consolidate-voxi-local-config-storage-formats-stop-words-replacements-vocabulary-samples-config-yaml.md)

---

## 1. Problem & Motivation

Issue 117 added `voxi feedback sample import <path>` for one of the several
independent local-state files under `~/.config/voxi/`. The same
cross-machine-sync need applies to the rest of that directory, which today
has no import path at all:

- `stop-words.json` (`internal/feedback/feedback.go`) — user stop-word
  overrides, disabled builtins, silence artifacts (JSON lists).
- `replacements.json` (`internal/feedback/replacement.go`) — heard-form to
  written-form corrections (JSON list of `{from, to, fixed_case}`).
- `vocabulary.txt` (`internal/speechcontext/context.go`) — persistent
  speech-context terms (line-delimited text).
- `config.yaml` / `env` (`internal/config/config.go`) — main Voxi config and
  environment overrides.
- `samples/` — already covered by issue 117.

A developer moving to, or working across, a second machine currently has no
supported way to bring any of this over except manually copying files and
hand-merging JSON/text by hand.

## 2. Desired Design

Add a `voxi config import <dir>` (name TBD during implementation — could
also live under `voxi feedback import <dir>` for symmetry with the sample
command) that imports a snapshot of another machine's `~/.config/voxi/`
directory (or a subset of it), dispatching per-file to the right merge
strategy:

- `stop-words.json`, `replacements.json`: list merge, keyed by the natural
  identity field (stop-word text, `from` for replacements), skip collisions
  by default with an `--overwrite` escape hatch — same policy issue 117
  established for samples.
- `vocabulary.txt`: line-set merge (dedupe).
- `config.yaml` / `env`: likely exclude from a first version, or require
  explicit `--include-config` — these are machine-specific (paths, device
  IDs) in ways the other files aren't, so a blind merge risks importing
  settings that don't apply to the target machine. Decide scope during
  implementation.
- `samples/`: reuse `internal/devsample.Import` from issue 117 rather than
  reimplementing.

Support importing a single area (`--only stop-words`, `--only vocabulary`,
etc.) as well as "everything importable" as the default, so a developer can
selectively sync.

## 3. Relationship to Issue 119

This ticket can proceed independently, dispatching per-file with one merge
function per format as it exists today. If issue 119 (config storage
consolidation) lands first, prefer building this against the unified
storage layer instead of the current five separate formats — check 119's
status before starting implementation.

## 4. Out of Scope

- Any network/cloud transfer mechanism — transport-agnostic, same as 117.
- Redesigning the underlying file formats (tracked in issue 119).
