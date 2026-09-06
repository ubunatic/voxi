# 062: FluidVoice Automatic Vocabulary Training From User Corrections (Research)

**Status**: In Progress — starting research per user request
**Priority**: P3 (Low)
**Severity**: Informational
**Category**: Research
**Related**: [032 Small.en project vocabulary biasing](032-small-en-project-vocabulary-biasing.md), [046 Speech-context default-on](046-speech-context-default-on.md), [033 User stop-word feedback](033-user-stop-word-feedback.md), [034 Isolated silence-artifact feedback](034-isolated-silence-artifact-feedback.md), [047 OSS voice-typing tool landscape research](047-oss-voice-typing-tool-landscape-research.md)

---

## 1. Problem & Motivation

`altic-dev/FluidVoice` (macOS, Swift, GPLv3, ~11k stars — a "Wispr Flow
alternative") ships a feature not present in Voxi: it automatically detects
when a user manually corrects a dictated word/phrase in the target
application and feeds that correction back into its custom-vocabulary
dictionary without any explicit "add to vocabulary" step from the user.
Relevant source: `Sources/Fluid/Services/AutomaticDictionaryCorrectionTracker.swift`,
`DictionaryTrainingEndpointDetector.swift`, `DictionaryTrainingEndpointMonitor.swift`,
`DictionaryTransferService.swift`, `PronunciationDictionaryStore.swift`.

Voxi's current vocabulary biasing (`internal/speechcontext`, issues 032/046)
is a static, manually curated project-vocabulary prompt. Voxi's feedback
mechanisms (`internal/feedback`, issues 033/034/042) capture stop-words and
silence artifacts, but require the user to explicitly run a `voxi feedback`
command — there is no closed loop where an in-app text correction
automatically strengthens future recognition of the same term.

This ticket is research-only: understand FluidVoice's mechanism well enough
to judge whether an equivalent is buildable in Voxi's Linux/Wayland
architecture, and at what cost.

## 2. Research Questions

1. **Correction detection**: How does `DictionaryTrainingEndpointDetector`/
   `DictionaryTrainingEndpointMonitor` decide a user has "corrected" text
   rather than just continued typing or editing unrelated content? What
   signal do they key off (macOS Accessibility API text-change diffs,
   cursor position, timing window after a dictation insert)?
2. **Mapping to source**: How is the corrected string mapped back to the
   original mis-transcribed word/phrase from the just-completed dictation,
   given the two may not be substring-aligned?
3. **Storage & application**: How does `PronunciationDictionaryStore` /
   `DictionaryTransferService` persist learned corrections, and how are they
   later applied — as decoder-level biasing (like Voxi's initial-prompt
   vocabulary), a post-processing find/replace pass, or both?
4. **False-positive guardrails**: What prevents a one-off manual edit
   (unrelated to STT error) from polluting the dictionary? Any confidence
   threshold, repetition requirement, or user confirmation step?
5. **Portability to Voxi**: Voxi injects via `dotool` and has no equivalent
   to macOS's Accessibility API text-change observation on arbitrary
   Wayland apps. What Linux/Wayland-side signal (if any) could stand in —
   e.g., watching Voxi's own typed-output buffer for an immediately
   following manual edit via a compositor text-input protocol, or is this
   simply not observable on Wayland without app cooperation? Is the
   feature feasible at all outside of Voxi's own recorder/history UI (i.e.
   only within `voxi monitor`/GNOME companion, not system-wide)?

## 3. Deliverables

- A `## Research Findings` section in this ticket answering the above,
  written from actually reading the four/five files listed above (not just
  the README/marketing copy).
- An explicit feasibility verdict for Voxi: adopt, adopt in reduced form
  (e.g. scoped to Voxi's own history/monitor UI where Voxi controls both
  ends of the text), or reject with reasons.
- If "adopt" or "reduced form": a rough sketch of what would live in
  `internal/speechcontext` / `internal/feedback` and what new signal source
  it would need — not a full implementation.

## 4. Non-Goals

- No code changes in this ticket.
- Not evaluating FluidVoice's other features (Command Mode, Rewrite Mode,
  multi-engine STT abstraction, local HTTP API) — those are separate,
  lower-priority feature ideas, not research gaps, and are not in scope
  here.

## 5. Background

Raised 2026-09-06 after the user asked to evaluate FluidVoice
(`https://github.com/altic-dev/FluidVoice`) as a source of adoptable ideas
for Voxi. Initial scan established: the app is 100% Swift on macOS-only
frameworks (Speech, Accessibility API, CoreAudio, CoreML) — the README's
"Windows... on the way" is a waitlist signup, not existing code, so there is
no cross-platform core to fork or clone. Licensing: FluidVoice is GPLv3,
Voxi is AGPLv3 — compatible if literal code were ever reused, though nothing
here calls for that given the disjoint stacks (Swift/macOS vs Go/Linux).
