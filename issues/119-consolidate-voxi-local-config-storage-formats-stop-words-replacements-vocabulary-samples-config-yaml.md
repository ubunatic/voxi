# 119 — Consolidate voxi local config storage formats (stop-words, replacements, vocabulary, samples, config.yaml)

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Refactor
**Related**: [118 voxi config import command](118-add-voxi-config-import-command-for-cross-machine-local-state-sync.md), [117 sample import](117-add-voxi-feedback-sample-import-command-for-cross-machine-sample-transfer.md), [099 replace corpus.tsv sample manifest with a more robust storage format](099-replace-corpus-tsv-sample-manifest-with-a-more-robust-storage-format.md)

---

## 1. Problem & Motivation

`~/.config/voxi/` currently holds local state in at least five different,
independently-implemented formats, each with its own load/save/validate
logic:

- `config.yaml` — main config (YAML), `internal/config/config.go`
- `env` — environment overrides (shell-style key=value), `internal/config/config.go`
- `stop-words.json` — JSON object with three string lists, `internal/feedback/feedback.go`
- `replacements.json` — JSON list of structs, `internal/feedback/replacement.go`
- `vocabulary.txt` — line-delimited plain text, `internal/speechcontext/context.go`
- `samples/corpus.tsv` + `<name>.wav` — TSV manifest + sidecar binaries, `internal/devsample/sample.go` (itself flagged for a format change in issue 099)

Each format was added independently as its feature landed, which is
reasonable in isolation, but it means every cross-cutting concern —
collision-safe merging (issue 118), atomic writes, backup/restore, file
permissions, validation-before-accept — has to be reimplemented per format
instead of written once. Issue 118 (config import) already anticipates
having to write a separate merge strategy per file for exactly this reason.

## 2. Desired Design (to be worked out during implementation/design pass)

Evaluate consolidating the non-binary local state (stop-words, silence
artifacts, replacements, vocabulary, and possibly config.yaml/env) behind
one storage convention — e.g. a shared package providing atomic
read/write/merge primitives, and/or a single structured file (YAML or
JSON) with named sections instead of five separate files. Open questions
to resolve in design, not here:

- Does `samples/` (binary WAV + manifest) join this consolidation, or stay
  a deliberately separate case given it pairs binary data with metadata
  (issue 099 already tracks its manifest format separately)?
- Single combined file vs. several files sharing one loader/writer
  package — a combined file simplifies import/export and atomic
  multi-field writes, but loses the "just `cat`/`grep` the one file you
  care about" simplicity the current per-concern files have.
- Migration path for existing installs: must read old-format files and
  either transparently upgrade them or provide a `voxi config migrate`
  step — this cannot be a breaking change for current users without a
  migration story.
- Whether this motivates using a spec-driven approach for these files too
  (see docs/Spec.md's YAML-single-source-of-truth pattern already used
  elsewhere in the project).

## 3. Why This Matters for Issue 118

Issue 118 (cross-machine `voxi config import`) can be built against the
current five formats independently, but every one of its per-file merge
functions is throwaway work if this consolidation lands afterward and
changes the underlying storage shape. If both are picked up, do 119's
design pass first, or explicitly accept 118's per-format merge code as
short-lived.

## 4. Out of Scope

- Implementing the import command itself (issue 118).
- The samples manifest format specifically (issue 099) — reference it, but
  let it stay its own decision unless the design pass concludes samples
  should be folded in too.
