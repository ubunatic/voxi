# 093 — Collapse an immediately-repeated trailing sentence/clause in eager transcripts

**Status**: Closed — Implemented by codex (gpt-5.6-luna) — CollapseRepeatedTrailingClause in internal/asr/asr.go, wired into CleanWhisperTranscript; go vet/go test verified independently. Commit ba815ba
**Priority**: P3 (Low)
**Severity**: Bug (occasional hallucination artifact, not safety-critical)
**Category**: ASR Quality
**Related**: [internal/asr/asr.go](../internal/asr/asr.go) (`CleanWhisperTranscript`, `StripTrailingHallucinations`), [internal/asr/safety.go](../internal/asr/safety.go) (`CheckTranscriptSafety`, `repeatedUnit`), [spec/models.yaml](../spec/models.yaml) (`transcript_safety`, `stop_words`), [internal/feedback/feedback.go](../internal/feedback/feedback.go) (`IsSilenceArtifact` — the related-but-different mechanism for whole-utterance artifacts)

---

## 1. Problem

Reported by the user (2026-09-09) on the Cohere Transcribe (`crispasr`) default
backend: dictation occasionally produces a transcript where the last one or
two words of a sentence are repeated immediately after the sentence has
already ended with terminal punctuation — e.g. something shaped like
`"...let's get started. get started"` rather than the intended
`"...let's get started."`.

This is a distinct failure mode from two things the codebase already
handles:

- **Not a whole-utterance silence artifact** (`internal/feedback`'s
  `IsSilenceArtifact`/silence-artifact list) — that mechanism matches an
  *entire* utterance against a taught exact string (e.g. a spurious "And."
  from mic startup noise) and never touches text embedded in a longer,
  otherwise-correct transcript. It doesn't apply here because the repeated
  words are appended to real, correctly-recognized speech, not standing
  alone.
- **Not pathological repetition** (`internal/asr/safety.go`'s
  `CheckTranscriptSafety`/`repeatedUnit`, gated by `spec/models.yaml`'s
  `transcript_safety.min_repeat_count: 12`) — that check exists to reject
  runaway garbage loops (a single short unit repeated 12+ times) before
  they reach the injector, per issue 083. A single accidental duplication
  of the last clause is nowhere near that threshold and passes safety
  checks untouched, since the resulting text is short and well-formed.
- **Not a known stop-word/hallucination phrase**
  (`spec/models.yaml`'s `stop_words`, matched via
  `StripLeadingHallucinations`/`StripTrailingHallucinations` in
  `internal/asr/asr.go`) — those are fixed, known hallucinated phrases
  (e.g. "thank you for watching") shared across models, not a repeat of
  whatever the user actually just said, which is unpredictable content
  that can't be captured by a static pattern list.

There is currently no code path that detects "the tail of this transcript
duplicates the clause immediately before it."

## 2. Suggested Fix

Add a small, generic post-processing step — a natural sibling to
`StripLeadingHallucinations`/`StripTrailingHallucinations` in
`internal/asr/asr.go`, run in the same place inside `CleanWhisperTranscript`
(both call sites, `asr.go:147-149` and `asr.go:180-182`) — that:

1. Splits the cleaned candidate into sentences/clauses on terminal
   punctuation (`.`, `!`, `?`).
2. If the last clause's trailing word (or last two words) is identical
   (case-insensitive) to the same-length word run immediately preceding it
   across the punctuation boundary, drop the duplicated trailing copy.

Keep the match narrow and anchored specifically at a clause boundary right
after terminal punctuation — a generic "collapse any repeated word run
anywhere in the text" pass would risk eating intentional repetition for
emphasis ("very very good", "no no no"), which can appear mid-sentence
with no punctuation boundary and must not be touched. Anchoring the check to
"repeats immediately across a sentence-ending punctuation mark" keeps the
false-positive risk low: deliberately repeating a whole clause again
immediately after ending it with a period is not a natural dictation
pattern, unlike mid-sentence emphasis.

## 3. Verification

- Add unit tests in `internal/asr/asr_test.go` alongside the existing
  `TestStripLeadingHallucinations`/`TestStripLeadingDashFragment` tests:
  positive case (trailing clause duplicated after `.`), and at least one
  negative case confirming legitimate mid-sentence repetition (e.g. "no no
  no", "very very good") is left untouched.
- No live model access needed to verify the string-processing logic; if a
  repro sample is available (voxi's dev-sample recorder, issue 042), record
  one from the actual Cohere backend once it recurs and add it as a fixture,
  but don't block the fix on waiting for a fresh occurrence.
