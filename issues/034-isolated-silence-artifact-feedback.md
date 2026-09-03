# 034: Isolated Silence-Artifact Feedback for Ambiguous Dictation Words

**Status**: Complete
**Priority**: P2 (Medium)  
**Severity**: Moderate  
**Category**: Feature  
**Related**: [033 User stop-word feedback](033-user-stop-word-feedback.md), [eager VAD pipeline](../internal/eager/eager.go), [ASR filtering](../internal/asr/asr.go)

---

## 1. Problem & Motivation

Some Whisper `small.en` hallucinations are ordinary, legitimate words. For
example, microphone noise or silence can occasionally become the complete
transcript `bye`. A normal stop-word rule is unsafe because it could remove a
genuine `bye` in "hello, bye for now" or in the middle of a longer sentence.

Voxi's eager pipeline already segments audio on VAD pauses but does not receive
per-word timestamps or decoder confidence from Voxtype. It must therefore not
pretend it can classify an individual word as silence-related. The initial,
safe behaviour is to reject only an exact configured artifact that is the
**entire normalized transcript of one VAD utterance**.

## 2. User Interface

```text
voxi feedback silence-artifact add "bye"
voxi feedback silence-artifact list
voxi feedback silence-artifact remove "bye"
```

The command should state that an artifact is discarded only when it is the
whole utterance, and show the reversal command after `add`.

## 3. Design & Constraints

- Extend the existing local, atomic, `0600` feedback store; no cloud, LLM, or
  model change.
- Entries are literal, case-insensitive phrases after trimming whitespace and
  terminal sentence punctuation. Reject empty/control-character/overlong and
  duplicate values.
- Evaluate the artifact decision after transcript extraction and existing
  built-in/user stop-word filtering, but before any text is typed or recorded
  in history.
- Match only whole normalized transcripts. Never strip an artifact from a
  leading, middle, or trailing position within a longer transcript.
- With no configured artifacts, existing transcript behaviour must be
  byte-for-byte unchanged.
- Do not use raw RMS alone as a gate: VAD has already admitted the segment and
  loud background noise can resemble speech. Future confidence/timestamp-aware
  refinement needs explicit backend support and is out of scope.

## 4. Verification

1. Unit-test persistence and CLI add/list/remove, including 0600 mode and
   malformed-store fallback.
2. Unit-test normalization/matching: `bye`, `Bye!`, and whitespace variants
   reject; `goodbye`, `hello bye`, `bye for now`, and `say goodbye` preserve.
3. Test eager/ASR integration so a matched isolated artifact is neither typed
   nor recorded, while a longer sentence is unchanged.
4. Run `go test ./...`, `make check`, and `make install`.

## 5. Acceptance Criteria

- `voxi feedback silence-artifact add "bye"` prevents only standalone `bye`
  utterances from reaching the focused app.
- A genuine phrase containing `bye` remains intact in every position.
- The feature is local, reversible, tested, and does not alter the active
  `small.en` model or its decoding settings.

## 6. Completion Notes

Implemented `voxi feedback silence-artifact add|list|remove` using the
existing atomic, local `0600` feedback store. Eager transcription now checks
configured artifacts after existing transcript filtering and before it can
type or append history. Matching is case-insensitive and whole-utterance only,
after whitespace and terminal `.`, `!`, or `?` normalization.

## 7. Bug Fix (2026-09-03): Asymmetric Punctuation Trim

**Reported by user**: after adding artifacts (e.g. `.com`), a hallucinated
`.` (and other near-variants) still typed through during trailing-silence
segments at the end of recording — "still see the dot popping in" despite
having "already added a stopwatch stop words and ignored phrases."

**Root cause**: `normalizeArtifact` (`internal/feedback/feedback.go`) only
trimmed trailing `.!?` (`strings.TrimRight`), not leading. A stored artifact
`.com` therefore normalized to itself (`.com`, leading dot untouched), while
Whisper's actual hallucinated output `com.` normalized to `com` (trailing dot
stripped) — two different strings, so `IsSilenceArtifact`'s exact-match never
matched them against each other even though they're the same underlying
hallucination with the punctuation on the opposite side.

**Fix**: changed the trim to `strings.Trim(phrase, ".!?")` (both ends), so
`.com`, `com.`, `Com!`, and whitespace variants all normalize to the same
`com` key and match each other, while `yamal.com` (punctuation embedded
mid-string, not at an edge) is correctly left as a distinct phrase and still
does not match. Added `TestIsSilenceArtifactMatchesEitherSideOfPunctuation`
in `internal/feedback/feedback_test.go` covering this. This is a symmetric
normalization fix, not a design change — Section 3's whole-utterance-only,
no-substring-match constraint is unchanged.

**Known remaining limitation (not fixed here, by design per Section 3)**:
this mechanism is still a finite literal-string allowlist. In the same
session, freshly observed hallucinations `"Mwah!"` and `"and all that."`
(distinct from the previously-added `"bye"` and `"and all of that"` entries)
were typed through because they're simply not yet in the list — expected
behavior for this design, not a bug, but worth the user knowing new
hallucinated variants will keep requiring `voxi feedback silence-artifact
add` one at a time rather than being caught automatically. No ticket filed
for this; Section 3's non-goals already documents the deliberate choice not
to use RMS/duration heuristics or fuzzy matching.

Verification: `go build ./...`, `go vet ./...`, `go test ./...`, `make
check` all pass. `make restart-service` run (this touches
`internal/feedback`, used live by `internal/eager`).
