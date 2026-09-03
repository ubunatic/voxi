# 043: Combined Assistive-Feedback Config Summary Command

**Status**: Implemented
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Feature
**Related**: [038 vocabulary feedback command](038-vocabulary-feedback-command.md), [032 small.en vocabulary biasing](032-small-en-project-vocabulary-biasing.md)

---

## 1. Problem & Motivation

Voxi's local assistive-feedback state is split across several independent
subcommands with no combined view:

- `voxi feedback stop-word list`
- `voxi feedback vocabulary list`
- `voxi feedback silence-artifact list`
- `voxi config get` (only covers `type_delay_ms`, unrelated to feedback state)

There is no single command to see, at a glance, what stop-words, vocabulary
terms, and silence-artifact rules are currently active, or whether
speech-context is enabled. A user (or an agent) auditing their local setup
has to run three separate `list` commands and cross-reference manually.

## 2. Desired Design

Add a read-only summary command, e.g.:

```text
voxi feedback status
```

or a top-level alias if that fits Voxi's existing command taxonomy better —
evaluate against `voxi status`/`voxi info`-style precedent already in the CLI
before deciding placement. Output should be a compact, human-readable summary
covering, at minimum:

- Stop-word count and list (or a truncated preview with a count).
- Vocabulary term count and list/preview.
- Silence-artifact rule count and list/preview.
- Whether `--speech-context` is the effective default and where its
  vocabulary sources currently resolve from (explicit file, static spec
  terms, repository-derived).

This is read-only and additive: it must not introduce a new persistence
format, and it must reuse each existing subcommand's underlying read/list
logic rather than re-implementing file parsing.

## 3. Implementation Plan

1. Identify each existing `list`-style read path (`internal/eager` or
   wherever stop-word/vocabulary/silence-artifact storage lives) and extract
   a shared read function per category if not already factored that way.
2. Add the new command composing those reads into one formatted summary
   (plain text; consider a `--json` flag for scripting, matching whatever
   precedent other Voxi commands set for structured output, if any).
3. Unit tests for the aggregation/formatting logic (empty state, populated
   state, partial state).

## 4. Acceptance Criteria

- One command shows counts and previews for stop-words, vocabulary terms,
  and silence-artifact rules without requiring three separate invocations.
- No new file formats or persistence changes; purely reads existing state.
- `go test ./...`, `make check`, `make install` pass.

## 5. Non-goals

- No mutation commands here — this ticket is read-only summary output only;
  add/remove stays in the existing per-category subcommands.
- No change to what data is tracked — only how it's surfaced.

## 6. Implemented Behavior

Added `voxi feedback status`, nested under the existing `voxi feedback`
subcommand tree (the top-level `voxi status`-style precedent in this CLI is
`voxi record status`, scoped to recording state only, so `feedback status`
follows the ticket's own suggested placement and existing taxonomy). No
`--json` flag was added: none of the sibling `feedback` subcommands emit
structured output, so there was no existing convention to match (`voxi
history list --format json` is the CLI's only structured-output precedent,
and it belongs to an unrelated command family).

```sh
voxi feedback status
```

```text
Stop-words: 22 active (22 built-in, 0 user), 0 built-in disabled
  built-in	thanks-watching	thank you for watching
  built-in	thanks-for-watching	thanks for watching
  built-in	thank-you	thank you
  built-in	thanks-listening	thanks for listening
  built-in	subscribe	please subscribe.*
  ... and 17 more; see: voxi feedback stop-word list

Silence artifacts: 0

Vocabulary terms: 0

Speech-context (--speech-context): off by default (opt-in per eager invocation)
  vocabulary resolves from, in priority order:
    1. explicit file (~/.config/voxi/vocabulary.txt): absent, 0 term(s)
    2. static spec terms (spec/models.yaml speech_context.terms): 11 term(s) available
    3. repository-derived (git repo/file basenames): computed live at eager runtime, not evaluated here
```

Reuse rather than reimplementation:

- Stop-words and silence artifacts: reused `feedback.Load` unchanged. The
  built-in/disabled/user row computation that `stop-word list` had inlined in
  its Cobra `RunE` closure was extracted into a new standalone
  `feedback.StopWordRows(Overrides, []spec.StopWord) []StopWordRow`, now
  called by both `stop-word list` and `status`.
- Vocabulary: reused `speechcontext.LoadVocabulary` unchanged.
- Speech-context resolution: reports the static shipped term count from
  `spec/models.yaml`'s `speech_context.terms` (already loaded once in
  `cmd/voxi/main.go` and threaded into `feedback.NewCommand`) and whether the
  explicit `~/.config/voxi/vocabulary.txt` file exists, matching the
  `Explicit > Static > Repository` priority order documented in
  `internal/speechcontext.Build`. `--speech-context` itself is a per-invocation
  `eager` flag with no persisted override, so its "default" is always
  reported as off. Repository-derived terms are runtime/cwd-dependent
  (`speechcontext.DiscoverRepositoryTerms`) and are described but not
  evaluated by this read-only command.
- New `internal/feedback/summary.go` holds `BuildSummary` (pure aggregation,
  no I/O) and `FormatSummary` (plain-text rendering with a 5-item preview per
  category plus a pointer to the full `list` subcommand), covered by
  `internal/feedback/summary_test.go` for empty, populated, and partial
  state (e.g. vocabulary populated with no stop-words), plus preview
  truncation and row ordering.

Verified with `go build ./...`, `go vet ./...`, `go test ./...`, `make
check`, and `make install` on 2026-09-02. Confirmed `make restart-service`
is unnecessary: `internal/feedback` is a standalone CLI/read package not
imported by the live `voxi-agent.service` daemon paths
(`internal/eager`, `internal/record`, `internal/modifiers`, or the agent's
`eager` runtime).
