# 094 — `CollapseRepeatedTrailingClause` wrongly deletes a legitimate short answer that matches the question's last word

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Bug (false positive, deletes legitimate dictated text)
**Category**: ASR Quality
**Related**: [internal/asr/asr.go](../internal/asr/asr.go) (`CollapseRepeatedTrailingClause`, added for [093](093-collapse-an-immediately-repeated-trailing-sentence-clause-in-eager-transcripts.md)), [internal/asr/asr_test.go](../internal/asr/asr_test.go) (`TestCollapseRepeatedTrailingClause`)

---

## 1. Problem

Found by an independent review pass (`codex -m gpt-5.6-sol`, read-only review task, 2026-09-09) of
`CollapseRepeatedTrailingClause` — the function [093](093-collapse-an-immediately-repeated-trailing-sentence-clause-in-eager-transcripts.md)
added to collapse an accidentally-duplicated trailing clause, e.g. `"Let's get started. get
started"` → `"Let's get started."`.

The review found a genuine false positive that directly violates 093's own core requirement (never
delete legitimate, non-duplicate content):

```
Input:  "Was your answer no? No"
Output: "Was your answer no?"
```

The function collapses this because its 1-2-word suffix-match check only compares word content,
not whether the preceding text is actually the *same clause* being duplicated versus a *different*
independent sentence (here, a real one-word answer) that happens to share its last word with the
question's last word. `"No"` here is real, correctly-recognized speech — dropping it changes the
meaning of the transcript (a question with no visible answer at all) rather than fixing a
hallucination artifact.

This is a second, distinct issue from 093 itself: 093 asked for the collapse to be anchored to a
punctuation boundary (done, and it correctly avoids mid-sentence repetition like "very very good").
This bug shows the punctuation-boundary anchor alone is not sufficient — a real two-sentence
exchange where the second sentence happens to restate a content word from the first is a normal,
common dictation pattern (yes/no answers, short acknowledgements, one-word clarifications) and
should never be silently deleted.

## 2. Also found in the same review pass (lower severity)

- **Multiline call-site gap** (false negative, not a correctness risk): `CleanWhisperTranscript`'s
  Strategy 2 (line-by-line filtering) calls `CollapseRepeatedTrailingClause` on each line
  independently, before joining lines with spaces. A duplicate split across two lines (e.g. input
  lines `"Let's get started."` then `"get started"`) is never collapsed, since each line is too
  short on its own to trigger the match. Worth fixing alongside the false positive since both live
  in the same function/call sites, but lower priority — no data is lost, just an occasional
  uncollapsed duplicate (093's original, milder symptom).
- **Punctuation-cluster/Unicode edge cases** (minor, false negatives only): repeated `!`/`?`
  clusters (`"Really?! really"`, `"Go! go!!"`), no-space boundaries (`"Let's go.go"`), full-width
  Chinese terminal punctuation (`"再见。再见"`), and curly apostrophes (`"C'est bon. c'est bon"`) are
  not collapsed. None of these delete legitimate content — they're just missed positive cases.
  Optional follow-up only; not required for this ticket.

## 3. Suggested Fix

Tighten the match so it cannot fire on a plausible independent short reply. Options, in rough order
of simplicity:

- Require the suffix to be at least 2 words (drop the 1-word case entirely) — a one-word echo is
  exactly the shape of a legitimate short answer/acknowledgement, whereas an accidental ASR
  duplication of a longer clause is the pattern 093 was actually reported against
  (`"...let's get started. get started"`, a 2-word repeat). This is the cheapest fix and removes
  the false-positive class demonstrated above without losing 093's original motivating case.
- Alternatively (more conservative but more work): additionally require that the prefix clause
  itself end with the same word count of repeated content and that the preceding clause be long
  enough that a bare repeat is implausible as a standalone reply (e.g. don't fire when the entire
  prefix clause the match anchors against is itself very short, since a short prefix + short
  suffix pair is exactly the "two short independent sentences" shape).

Add a regression test asserting `"Was your answer no? No"` stays unchanged, alongside the existing
`TestCollapseRepeatedTrailingClause` cases.

## 4. Verification

- `go test ./internal/asr/...` after the fix, with the new regression case above added.
- Confirm 093's original motivating case (`"Let's get started. get started"` → `"Let's get
  started."`) still collapses correctly if the fix narrows the match to 2-word suffixes only.
