# 058: Configurable Scope for Hallucination Fixes: All Chunks vs. First-Chunk-Only

**Status**: Proposed
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Feature
**Related**: [033 user stop-word feedback](033-user-stop-word-feedback.md), [034 isolated silence-artifact feedback](034-isolated-silence-artifact-feedback.md), [049 leading hallucination prefix](049-leading-hallucination-subs-byuk.md), [internal/asr/asr.go](../internal/asr/asr.go), [internal/eager/eager.go](../internal/eager/eager.go), [Roadmap](../docs/Roadmap.md)

---

## 1. Problem & Motivation

User-stated target workflow for `voxi` (2026-09-05), given verbatim as the
reference spec this ticket narrows down to its one actionable gap:

1. Press Super+X.
2. Voxi records and does not lose the first word as soon as the OS mic icon
   appears.
3. Voxi chunks what is said.
4. Each chunk is typed as it's processed (not batched at the end).
5. **Fixes (hallucination/stop-word filtering) should be toggleable — for
   all chunks, or for the first chunk only.**

Points 1-4 already match the current `voxi eager`/`voxi agent --daemon`
architecture and needed no new ticket:

- Point 2: the 500ms circular pre-roll buffer in `internal/audio/audio.go`
  (`PreRollMs`/`preRollBuffer`) already prepends buffered frames to the
  detected speech segment specifically to "retain initial consonants" — see
  the comment at `internal/audio/audio.go:156`. (Whether this is currently
  *reliable* in practice is exactly what the still-open
  [057 recording start/stop race](057-recording-start-stop-race-delayed-hallucinated-typing-after-stop-cannot-restart-recording.md)
  bug threatens — that's a correctness bug in the existing mechanism, not a
  missing feature, and is tracked separately.)
- Points 3-4: VAD-driven chunking and per-chunk typing already exist —
  `runEagerCaptureSession` (`internal/eager/eager.go`) dispatches one
  transcription per detected utterance and calls `typing.TypeText` on each
  result as it's produced (`internal/eager/eager.go:376`), not after the
  whole session ends.

**Point 5 is the actual gap.** Today, `asr.IsSafeToType`,
`feedback.IsSilenceArtifact`, and `asr.StripLeadingHallucinations` apply
uniformly to every chunk in a session with no notion of chunk position — a
mid-session artifact utterance is filtered exactly the same way as the very
first utterance after the mic opens. There is no toggle to scope hallucination
filtering to "first chunk only" (where pre-roll/cold-start artifacts are most
likely) vs. "every chunk" (the current, only behavior).

## 2. Desired Design (Proposed — Not Yet Scoped In Detail)

Add an opt-in scope setting for existing hallucination-filtering mechanisms
(033 stop-words, 034 silence-artifacts, 049 leading-hallucination strip),
with at least these values:

- `all` (default, current, unchanged behavior — every chunk in the session
  is filtered identically).
- `first-only` — apply the filters only to the first chunk of a recording
  session; subsequent chunks in the same session bypass them.

Open questions to resolve before implementation (canary-first per
`docs/Canary.md`):

- Is "first chunk only" actually the right dimension, or does the user's
  underlying motivation (cold-start/pre-roll artifacts specifically) map
  better to "first N milliseconds of the session" or "first chunk after
  mic-open, regardless of session length"? Needs a clarifying pass with the
  user before committing to the exact toggle semantics implied by the note.
- Where does chunk position need to be threaded through — currently no
  chunk-index/session-position concept exists in `runEagerCaptureSession`'s
  dispatch path; determine the least invasive way to add it (e.g. a simple
  counter reset per `startRecording` call) without conflicting with the
  work already planned for
  [057](057-recording-start-stop-race-delayed-hallucinated-typing-after-stop-cannot-restart-recording.md)'s
  fix to that same start/stop state machine — sequence 058 after 057, not
  concurrently, since both touch the same recording-session lifecycle state.
- CLI surface: likely a new `voxi feedback` scope flag or config value
  alongside the existing `stop-word`/`silence-artifact`/`vocabulary`
  subcommands, rather than a new top-level command.

## 3. Implementation Plan

1. Confirm with the user the precise intended semantics of "first chunk
   only" (see open questions above) before writing code.
2. Land [057](057-recording-start-stop-race-delayed-hallucinated-typing-after-stop-cannot-restart-recording.md)
   first — both tickets touch the same recording-session state machine in
   `internal/eager/eager.go`, and building on unstable start/stop state
   would only complicate both.
3. Add a chunk-position counter scoped to one recording session.
4. Thread a scope setting through to the filtering call sites in
   `internal/eager/eager.go` (~line 512, ~line 523/526) so `first-only`
   skips `IsSafeToType`/`IsSilenceArtifact`/`StripLeadingHallucinations` for
   chunks after the first.
5. Unit tests: default (`all`) behavior byte-for-byte unchanged; `first-only`
   correctly exempts the second-and-later chunk of a multi-utterance session
   from filtering while still filtering the first.

## 4. Acceptance Criteria

- Default behavior (scope unset / `all`) is unchanged from today.
- `first-only` scope is selectable, documented, and verified to exempt only
  the first chunk of a session, not the whole session or arbitrary chunks.
- `go test ./...`, `make check` pass; `make restart-service` run (touches
  `internal/eager`, used live by the daemon).

## 5. Non-Goals

- Not a redesign of the filtering mechanisms themselves (033/034/049 stand
  as-is) — this only adds a scope dimension to when they're applied.
- Not a fix for [057](057-recording-start-stop-race-delayed-hallucinated-typing-after-stop-cannot-restart-recording.md) —
  that bug must be resolved first since it affects the same session-lifecycle
  state this feature needs to build on.
