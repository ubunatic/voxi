# 063: FluidVoice Spoken-Punctuation and Dictation-Literal Post-Processing Rules (Research)

**Status**: Research Complete — Gap Confirmed: recommend follow-up implementation ticket for spoken-punctuation formatting
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

## 5. Research Findings (2026-09-06)

Read in full: `ASRService+SpokenPunctuationFormatting.swift` (1086 lines),
`ASRService+DictationLiteralFormatting.swift` (463 lines),
`DictationAIPostProcessingGate.swift` (118 lines); grepped the relevant call
sites in `ASRService.swift` (5753 lines, too large to read whole — targeted
reads around the formatting call sites instead, consistent with this repo's
own context-discipline convention).

**Q1 — disambiguating a spoken command word from literal content.**
FluidVoice does *not* solve this with any contextual/ML disambiguation. It
uses a simple, explicit, user-configurable **trigger prefix word**
(default: `"literal"`, `SettingsStore.defaultPunctuationDictionaryPrefix`).
A punctuation phrase (`"comma"`, `"period"`, `"question mark"`, `"open
paren"`, `"at sign"`, etc. — ~40 phrase rules covering all common symbols)
only converts to its symbol when it is immediately preceded by that
configured prefix word in the transcript, matched via
`matchPrefixedRule`/`indexAfterPrefix`. So "literal period" → `.`, but a
sentence that legitimately contains the word "period" is left untouched.
A handful of rules add extra gates on top of the prefix requirement
(`requiresDotContext`, `requiresSlashPathContext`, `requiresSymbolContext`,
`requiresAtSignPunctuationApp` — the last one restricts "at sign"/
"commercial at" to code/terminal/chat apps by app-name/bundle-ID/window-title
sniffing, since in prose "at sign" is more often literal). This is a cheap,
fully deterministic, easily portable mechanism — no model or heuristic
scoring involved.

**Q2 — the "literal formatting" file does not do what the ticket assumed.**
Correcting the ticket's premise: `ASRService+DictationLiteralFormatting.swift`
is not about numeral/currency/date formatting or capitalization. It handles
three narrower things, all keyed off spoken lead-in words rather than a
single global prefix:
- **Slash-command formatting**: turns spoken `"slash foo"` / `"forward
  slash foo"` (or an already-literal `"/ foo"` with a stray space) into a
  tight `/foo` token, gated by a reject-list of ~40 common words that must
  not be treated as command names (`"desktop"`, `"documents"`, `"tmp"`,
  `"the"`, etc.) and, for the spoken form, a required lead-in verb
  (`"run"`, `"open"`, `"type"`, `"send"`, …).
- **@mention formatting**: turns `"at Alice"` / `"tag Bob Smith"` /
  `"mention Carol"` into `@Alice` / `@Bob Smith` / `@Carol`, again gated by
  a reject-list (`"today"`, `"home"`, `"lunch"`, …) plus a relaxed mode
  (bare `"at Name"`, no lead-in required) restricted to apps recognized as
  chat/mention contexts.
- **Terminal-autocomplete spacing**: strips a trailing space FluidVoice
  would otherwise insert after a slash-command or mention token, specifically
  in terminal/chat apps where a trailing space would break shell tab-
  completion or a mention-autocomplete popup.
No numeral/date/capitalization rules exist in this file or its sibling; that
part of the ticket's Problem & Motivation section was speculative and is
now corrected here.

**Q3 — pipeline layering.** Confirmed the assumption was right, even though
the file split was different than expected. Every transcription result in
`ASRService.swift` runs a fixed, always-on, synchronous deterministic chain
before it is ever returned or typed:
`removeFillerWords → applyCustomDictionary → applySpokenPunctuationFormatting`
(see call sites around `ASRService.swift:2752` and `:2772`). AI enhancement
(`DictationAIPostProcessingGate.isConfigured`) is a fully separate, optional,
later stage gated on whether a prompt/provider is actually configured and
verified (local Fluid Intelligence, or a cloud provider with a verified
fingerprint) — it never blocks or is required for the deterministic pass to
run. This matches Voxi's own "deterministic pass always runs, nothing else
required" design, so no divergence to flag here — it's a confirmation, not a
gap.

**Q4 — overlap with hallucination-shaped artifacts.** None found within
these two files. FluidVoice's dash-fragment-equivalent hallucination
handling (if any) is not in the spoken-punctuation or dictation-literal
formatters — it would live elsewhere in the 5753-line `ASRService.swift`,
which is out of this ticket's scope per its Non-Goals. Voxi's dash-fragment
fix (commit `604d0f9`, `internal/asr/asr.go`) addresses a Whisper-model
decoding artifact (a garbled short token prepended before a real sentence)
that has no counterpart in what these two files do — they only ever
transform text the user is presumed to have deliberately spoken as spoken
punctuation or a command word; they don't attempt to detect or strip
garbled/hallucinated fragments. No overlap, no shared fix opportunity.

### Comparison Table

| FluidVoice rule | Voxi equivalent | Status |
|---|---|---|
| Spoken-punctuation-to-symbol with `"literal"` trigger prefix (~40 symbols) | none | **Absent** |
| Context gates on ambiguous symbol words (`requiresDotContext`, `requiresAtSignPunctuationApp`, etc.) | none | **Absent** (N/A without the base feature) |
| Spoken/literal slash-command formatting (`"slash foo"` → `/foo`) | none | **Absent** |
| Spoken @mention formatting (`"tag Bob"` → `@Bob`) | none | **Absent** |
| Terminal-autocomplete trailing-space suppression | none | **Absent** |
| Deterministic cleanup pass always runs; AI enhancement optional/gated on top | `CleanWhisperTranscript` (filler/hallucination stripping) always runs; no AI enhancement layer exists yet | **Equivalent in spirit** (Voxi has no AI-enhancement layer at all yet, so nothing to gate) |
| Dash-fragment/garbled-token hallucination stripping | `StripLeadingDashFragment` (604d0f9) | **Voxi-only** — no FluidVoice counterpart found in scope |

### Verdict

Voxi has **zero** spoken-punctuation-to-symbol conversion today — Whisper's
own model output is the only source of punctuation. This is a genuine,
previously-uncatalogued feature gap, not a nuance difference. FluidVoice's
solution is small, fully deterministic, and highly portable: a phrase-table
lookup keyed by a single configurable trigger word, no ML, no context beyond
optional app-name sniffing for a couple of ambiguous symbols.

**Recommend a follow-up implementation ticket** (not a research ticket) for:
"add opt-in spoken-punctuation formatting (`literal comma` → `,` etc.) to
`internal/asr`" — small, self-contained, testable via table-driven Go tests
mirroring FluidVoice's phrase-rule table, landing as a new pass in
`CleanWhisperTranscript` after hallucination stripping. Slash-command/mention
formatting is lower priority (niche, app-context-dependent) and can be a
separate, later ticket if ever wanted — not recommended as part of the same
follow-up to keep it small.

## 6. Background

Raised 2026-09-06 alongside issue 062, from the same FluidVoice evaluation
prompted by the user
(`https://github.com/altic-dev/FluidVoice`). See issue 062 §5 for the
licensing/platform-portability context (GPLv3 vs Voxi's AGPLv3; Swift/macOS
stack with no cross-platform code to reuse — this is a read-and-compare
exercise, not a code port).
