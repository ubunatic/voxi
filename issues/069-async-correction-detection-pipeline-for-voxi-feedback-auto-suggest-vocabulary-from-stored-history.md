# 069: Async Correction-Detection Pipeline for `voxi feedback` (Auto-Suggest Vocabulary From Stored History)

**Status**: Proposed / Feature
**Priority**: P3 (Low)
**Severity**: Enhancement
**Category**: Feature
**Related**: [062 FluidVoice automatic vocabulary training research](062-fluidvoice-automatic-vocabulary-training-from-user-corrections-research.md), [032 Small.en project vocabulary biasing](032-small-en-project-vocabulary-biasing.md), [046 Speech-context default-on](046-speech-context-default-on.md), [033 User stop-word feedback](033-user-stop-word-feedback.md), [053 Ring buffer of recent audio chunks and transcripts](053-ring-buffer-recent-audio-chunks-and-transcripts.md), [internal/feedback](../internal/feedback), [internal/speechcontext](../internal/speechcontext)

---

## 1. Problem & Motivation

`voxi feedback` today (`internal/feedback/command.go`) is entirely
**synchronous and user-initiated**: the user has to consciously remember
that Voxi mis-transcribed something, then explicitly run
`voxi feedback stop-word add "<phrase>"` (or the dev-sample recorder,
issue 042) right then, from scratch, with no help from what Voxi already
recorded. There is no path from "Voxi's stored chunk/transcript history
already contains evidence of a recurring mis-transcription" to "a
vocabulary/stop-word entry gets proposed automatically."

Issue 062's research into FluidVoice's automatic-dictionary-correction
feature found that its core *algorithm* — diff the original transcript
against a later edit, isolate the changed span, expand to word boundaries,
and require the same (heard, corrected) pair to recur before ever
proposing it — is fully portable Go logic with no macOS-specific
dependency. The one part of FluidVoice's design that doesn't port
(observing arbitrary third-party app text fields live via the Accessibility
API) simply doesn't apply here: this ticket only concerns **Voxi's own
stored history** (issue 053's chunk ring buffer / transcript store), which
Voxi already fully owns on both the "before" and "after" side — no new OS
capability is needed.

## 2. Scope

This is new feature work, not a bug fix. Two independent pieces:

1. **Detection**: given Voxi's stored recent chunks/transcripts (issue 053),
   find candidate (heard, corrected) pairs. The "corrected" side has to come
   from *somewhere* — for a first cut, this is most plausibly the existing
   `voxi feedback` / dev-sample-recorder flows where a user already supplies
   a corrected transcript (issue 042's "named utterance + corrected text"),
   not a live text-field watch (which Voxi has no mechanism for, and issue
   062 confirmed is infeasible system-wide on Wayland). Reframe as: mine
   *already-collected* correction pairs across history for recurrence,
   rather than trying to detect corrections happening in real time in an
   arbitrary app.
2. **Async surfacing**: a new `voxi feedback vocabulary suggest` (or similar)
   subcommand that lists pending candidates — pairs that have recurred ≥N
   times (configurable, default matching FluidVoice's 2-in-7-days as a
   starting point, tuned to Voxi's usage cadence) — for the user to review
   and explicitly accept/dismiss. Never silently auto-write to
   `internal/speechcontext`'s vocabulary file.

## 3. Design Sketch (Non-Binding — Refine at Implementation Time)

- New helper package or file (e.g. `internal/speechcontext/correction.go`)
  implementing the diff-and-token-expand candidate extraction — this is
  ~80 lines of string-range arithmetic per issue 062's read of FluidVoice's
  `AutomaticDictionaryCorrectionDetector`; License-compatible to reimplement
  cleanly in Go (do not port Swift code verbatim; the *algorithm* is what's
  being reused, write it fresh against Voxi's own data model).
- A recurrence-counting store keyed on the (heard, corrected) pair,
  scanning Voxi's existing chunk/history retention (issue 053) — avoid
  introducing a second persistence mechanism if `internal/history`/
  `internal/chunks` already has enough retained data to count occurrences.
- `voxi feedback vocabulary suggest` prints pending candidates with
  accept/dismiss subcommands (`voxi feedback vocabulary accept <id>`,
  `... dismiss <id>`) — accepted entries land in
  `internal/speechcontext`'s vocabulary file the same way a manually
  curated term would (issue 032/046), so no new consumption path is needed,
  only a new production path.
- Async framing: this command is meant to be run periodically (manually, or
  eventually via a `voxi monitor` notification/badge), not synchronously at
  the moment of a mistake — the whole point is decoupling "when the error
  happened" from "when the user gets around to reviewing it."

## 4. Acceptance Criteria

- `voxi feedback vocabulary suggest` lists candidates derived from real
  stored history/correction data, with a clear recurrence count and
  accept/dismiss workflow.
- Accepted suggestions are verified to actually affect subsequent
  transcription (i.e. they reach `internal/speechcontext`'s vocabulary
  correctly) via an integration test.
- No candidate is ever written to the vocabulary file without an explicit
  accept — covered by a test asserting `suggest` alone never mutates state.
- Table-driven unit tests for the diff/candidate-extraction logic itself,
  independent of the CLI plumbing.

## 5. Non-Goals

- No live/real-time observation of third-party app text fields — issue 062
  confirmed this is infeasible on Wayland and out of scope entirely, not
  just deferred.
- Not a redesign of `internal/feedback`'s existing stop-word or dev-sample
  subcommands — this is additive (`vocabulary suggest`/`accept`/`dismiss`),
  not a replacement.
- No decoder-level (initial-prompt) vs. post-processing find/replace design
  decision is made here — reuse whatever mechanism issue 032/046 already
  established for consuming vocabulary terms; this ticket only concerns how
  candidates get *proposed*.

## 6. Background

Raised 2026-09-06, following the FluidVoice evaluation sprint (issues
062-065) and specifically issue 062's "adopt in reduced form" verdict, at
the user's explicit request to turn that reduced-form sketch into a
tracked feature ticket for Voxi's own async, CLI-driven feedback workflow.
