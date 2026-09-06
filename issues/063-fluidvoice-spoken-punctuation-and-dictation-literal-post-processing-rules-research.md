# 063: FluidVoice Spoken-Punctuation and Dictation-Literal Post-Processing Rules (Research)

**Status**: In Progress — starting research per user request
**Priority**: P3 (Low)
**Severity**: Informational
**Category**: Research
**Related**: Recent fix: strip leading dash-fragment hallucination prefix (commit `604d0f9`), [054 Short-pause acoustic gating and context priming](054-short-pause-acoustic-gating-and-context-priming.md), [033 User stop-word feedback](033-user-stop-word-feedback.md), [062 FluidVoice automatic vocabulary training research](062-fluidvoice-automatic-vocabulary-training-from-user-corrections-research.md)

---

## 1. Problem & Motivation

`altic-dev/FluidVoice` (macOS, Swift, GPLv3) applies a dedicated
post-processing pass over raw STT output before typing it, split into two
files: `Sources/Fluid/Services/ASRService+SpokenPunctuationFormatting.swift`
(turning spoken words like "comma", "period", "new line" into literal
punctuation/formatting) and
`Sources/Fluid/Services/ASRService+DictationLiteralFormatting.swift`
(handling literal-mode dictation, e.g. numerals vs. spelled-out numbers,
capitalization rules).

Voxi has been iterating on its own transcript-cleanup rules recently — most
recently stripping a leading dash-fragment hallucination prefix (commit
`604d0f9`), plus the broader hallucination-gating work in issues 033/034/054.
This ticket asks whether FluidVoice's ruleset covers edge cases Voxi hasn't
hit yet, purely as a reference to sanity-check Voxi's own post-processing
coverage — not to import their code.

## 2. Research Questions

1. What is the exact set of spoken-punctuation triggers FluidVoice
   recognizes (comma, period, question mark, new line/paragraph, others?),
   and how do they disambiguate a spoken command word ("period") from the
   same word used literally in dictated content (e.g. dictating a sentence
   that legitimately contains the word "period")?
2. What literal-formatting rules does `ASRService+DictationLiteralFormatting.swift`
   apply (number formatting, capitalization at sentence boundaries, currency/
   date normalization, etc.), and are any of these rules ones Voxi's
   `internal/asr` / eager pipeline currently lacks?
3. Where in FluidVoice's pipeline does this post-processing run relative to
   AI-enhancement post-processing (`DictationPostProcessingService`,
   `DictationAIPostProcessingGate`) — is spoken-punctuation formatting a
   cheap deterministic pass that always runs, with AI enhancement layered
   optionally on top? How does that layering compare to Voxi's current
   deterministic-only pipeline?
4. Does anything in these two files overlap with or suggest a fix for
   hallucination-shaped artifacts Voxi has already fought (dash-fragment
   prefixes, isolated silence-artifact words per issue 034)?

## 3. Deliverables

- A `## Research Findings` section in this ticket, written from actually
  reading `ASRService.swift` and the two formatting extension files (not
  the README).
- A short table: FluidVoice rule vs. Voxi equivalent (present / absent /
  partially covered), for whichever rules surface during the read.
- A verdict on whether any specific gap is worth its own follow-up ticket
  (name the candidate rule and where it would land — likely `internal/asr`
  or `internal/eager`), or whether Voxi's existing coverage is already
  equivalent or superior.

## 4. Non-Goals

- No code changes in this ticket.
- Not a general STT-engine comparison (see issue 039) or full-tool
  architecture comparison (see issue 047) — scoped narrowly to the
  post-processing/formatting layer only.

## 5. Background

Raised 2026-09-06 alongside issue 062, from the same FluidVoice evaluation
prompted by the user
(`https://github.com/altic-dev/FluidVoice`). See issue 062 §5 for the
licensing/platform-portability context (GPLv3 vs Voxi's AGPLv3; Swift/macOS
stack with no cross-platform code to reuse — this is a read-and-compare
exercise, not a code port).
