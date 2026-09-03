# 049: Leading Hallucination Prefix ("Subs byuk" / subtitle-credit artifacts) Not Stripped

**Status**: Implemented
**Priority**: P1 (High)
**Severity**: Major
**Category**: Bug
**Related**: [internal/asr hallucination filtering](../internal/asr/asr.go), [spec/models.yaml stop-word list](../spec/models.yaml)

---

## 1. Problem & Motivation

The user reports that dictated transcripts now consistently start with
"Subs byuk" (a phonetically garbled Whisper hallucination, almost certainly
the model's well-known "subtitles by [name/organization]" training-data
artifact — the same family of hallucination `spec/models.yaml`'s
`subtitles-by` stop-word entry (`pattern: "subtitles by.*"`) was written to
catch) prepended before the genuine transcribed speech, on every or nearly
every utterance.

The existing hallucination-filtering machinery in `internal/asr/asr.go`
only handles two shapes:

1. `IsSafeToType` + `hallucinationRegexp` reject a candidate **only when the
   entire trimmed line matches** a stop-word pattern anchored `^...$`
   (`asr.go:18-23`) — a real, non-empty utterance with a genuine sentence
   after the hallucinated prefix never matches this, so it isn't rejected.
2. `StripTrailingHallucinations` (`asr.go:64-71`) strips a stop-word match
   **only from the end** of the text (its regex has no anchor forcing
   end-of-string, but it's only ever invoked once per candidate/line, and
   the existing stop-word list and naming ("Trailing") were designed for
   outro-style artifacts like "thanks for watching" and "subtitles by ...").

There is currently **no leading-hallucination stripping** at all. A prefix
hallucination like "Subs byuk, hello team, let's start the standup." would
pass `IsSafeToType` (not a whole-line match) and pass through
`CleanWhisperTranscript` with "Subs byuk" still attached to the front of
the genuine sentence, and it would get typed verbatim into whatever app has
focus — a real accuracy/annoyance regression the user is hitting on what
sounds like every utterance right now.

Also worth checking: "Subs byuk" doesn't literally match the existing
`subtitles-by` pattern's text (`"subtitles by.*"`) since Whisper is
hallucinating a garbled phonetic rendering, not the literal string
"subtitles by" — so even a leading-strip mechanism reusing the existing
stop-word list verbatim might not catch this specific garbled variant
without an additional stop-word entry for it.

## 2. Reproduction / Evidence Needed

**Confirmed by the user (2026-09-02)**: the literal string is exactly
`"Subs byuk"` — not a guessed rendering, this is what actually appears at
the start of the typed output. That literal string (case-insensitive) is
usable directly as a new `stop_words` entry.

Still open, not required to ship the fix but worth tracking separately:

- Whether this started recently (e.g. correlated with issue 046 flipping
  `--speech-context` to default-on — worth checking whether the
  speech-context initial prompt itself is somehow priming this
  hallucination, versus it being pre-existing and only now noticed). If the
  user notices it stops or changes after this ticket's fix ships, that's
  independent of the root-cause question and doesn't block closing this
  ticket.
- A `voxi feedback sample record` capture reproducing it would still help
  validate the fix end-to-end against real audio, but is no longer a
  blocker for implementation now that the literal string is confirmed.

## 3. Desired Design

1. Add a `StripLeadingHallucinations` function to `internal/asr/asr.go`,
   mirroring `StripTrailingHallucinations` but anchored to the start of the
   text instead of matching anywhere, applied before/alongside the existing
   trailing strip in `CleanWhisperTranscript`'s both strategies (the
   quote-extract path at `asr.go:81-87` and the line-by-line path at
   `asr.go:92-118`).
2. Add the actual literal hallucinated string(s) the user's audio produces
   (from Section 2's captured samples) as new `stop_words` entries in
   `spec/models.yaml`, alongside or replacing the existing `subtitles-by`
   entry if it's the same underlying artifact family.
3. Regression-test against the existing stop-word list to confirm no
   existing trailing-strip behavior changes, plus new tests for the leading
   case using the captured real strings.

## 4. Acceptance Criteria

- A captured real utterance that previously produced "Subs byuk, <genuine
  speech>" produces just "<genuine speech>" after `CleanWhisperTranscript`.
- Existing trailing-hallucination stripping and whole-line rejection remain
  unchanged (unit tests for both pass unmodified).
- New unit tests cover the leading-hallucination case using the real
  captured string(s), not just a guessed pattern.
- `go test ./...`, `make check`, `make restart-service` (this touches the
  live eager transcription cleanup path) pass.

## 5. Non-goals

- No general NLP-based hallucination detection — pattern/stop-word based,
  consistent with the existing mechanism.
- No change to the speech-context prompting mechanism itself (issue
  032/046) unless Section 2's investigation actually implicates it as the
  cause, in which case that finding gets recorded here first before any
  code change there.

## 6. Implemented Behavior

Implemented in `internal/asr/asr.go`:

- Added `StripLeadingHallucinations(text string, stopWords []string) string`,
  mirroring `StripTrailingHallucinations`'s structure but with its regex
  anchored to the start of the (case-insensitive) text (`^\s*<pattern>\s*[.!,]*\s*`)
  instead of matching anywhere, so a stop-word-like substring occurring
  mid-sentence is left untouched — only a true leading match is removed.
- Wired `StripLeadingHallucinations` into both `CleanWhisperTranscript`
  strategies (the quote-extract path and the line-by-line fallback path),
  applied before `StripTrailingHallucinations` on each candidate/line. Order
  doesn't change which hallucinations are caught (each strip is anchored to
  its own end of the string), but stripping the leading prefix first keeps
  the candidate readable at every intermediate step.
- Added a new shared stop-word entry to `spec/models.yaml`'s
  `common_stop_words` anchor list: `{ id: subs-byuk, pattern: "subs byuk" }`,
  placed next to the existing `subtitles-by` entry, catching the confirmed
  literal garbled hallucination string case-insensitively.
- Added unit tests in `internal/asr/asr_test.go`:
  - `TestStripLeadingHallucinations` — strips a leading "Subs byuk, ..."
    prefix down to the genuine sentence, and confirms a stop-word-like
    substring occurring mid-sentence is left unchanged.
  - `TestCleanWhisperTranscriptStripsLeadingHallucination` — end-to-end
    test through `CleanWhisperTranscript` using real voxtype-style output
    with a "Subs byuk" prefix and the real `spec/models.yaml`-derived
    stop-word list.
  - All pre-existing tests (trailing-hallucination stripping, whole-line
    rejection, user stop-word boundary handling) pass unmodified.

Verification: `go build ./...`, `go vet ./...`, `go test ./...`, and
`make check` all pass. `make install` and `make restart-service` were run
so the live `voxi-agent.service` daemon picks up the fix (this path is used
by `internal/eager`).
