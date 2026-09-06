# 062: FluidVoice Automatic Vocabulary Training From User Corrections (Research)

**Status**: Research Complete — Adopt in reduced form (Voxi-owned history/feedback UI only, not system-wide)
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

## 5. Research Findings

Read in full: `AutomaticDictionaryCorrectionTracker.swift`,
`DictionaryTrainingEndpointDetector.swift`,
`DictionaryTrainingEndpointMonitor.swift`, `DictionaryTransferService.swift`,
`PronunciationDictionaryStore.swift`, plus Voxi's `internal/speechcontext`
and `internal/feedback` for comparison.

**1. Correction detection.** Immediately after FluidVoice types dictated
text, `AutomaticDictionaryCorrectionTracker.beginObservingInsertion` anchors
the exact `NSRange` it just inserted in the focused field (verified by
re-reading the field's value via the macOS Accessibility API and confirming
the substring matches). It then attaches an `AXObserver` to that field for
up to 30s, listening for `kAXValueChangedNotification`,
`kAXSelectedTextChangedNotification`, and
`kAXFocusedUIElementChangedNotification`. Every value change is diffed
against the last known value; if the changed span falls inside (or
immediately abuts) the tracked inserted range, it becomes a "pending
correction" that keeps absorbing further edits until a completion signal —
caret moves away, focus changes, ~1s of inactivity, or a 3s hard fallback —
fires evaluation. Crucially, this is not general typing surveillance: it
only ever watches the one field, for a bounded window, right after Voxi's
own dictation wrote into it — it does not scan arbitrary user typing.

**2. Mapping to source.** No fuzzy/phonetic alignment is used. Because the
tracker already knows precisely which `NSRange` it inserted, mapping is a
plain positional diff: common-prefix/common-suffix trimming between
before/after strings (`AutomaticDictionaryCorrectionDetector.textChange`)
isolates the changed span, which is then expanded outward to whitespace/
punctuation token boundaries so partial-word edits capture the whole word.
Candidates are filtered to ≤3 words, ≤40 chars each, alphanumeric, and
"semantically different" after stripping punctuation — cheap heuristics,
no NLP.

**3. Storage & application.** Nothing is auto-applied. A validated
candidate becomes a *suggestion* surfaced via a UI overlay
(`DictionaryCorrectionOverlayController`), gated by
`AutomaticDictionarySuggestionPolicy`: the same (heard, corrected) pair must
recur ≥2 times (configurable) within a rolling 7-day window before it is
even shown, and previously-dismissed pairs get a 7-day cooldown plus a
max-3-dismissals-per-session cap. Only on explicit user acceptance does it
get written to `SettingsStore.customDictionaryEntries` — used two ways: as
a literal post-processing find/replace list, and/or as weighted terms fed
into `ParakeetVocabularyStore` (decoder-level vocabulary boosting,
analogous to Voxi's initial-prompt biasing). `DictionaryTransferService`
only handles export/import of this already-accepted dictionary as JSON; it
plays no role in detection.

**4. False-positive guardrails.** Layered: (a) scope-limited to the exact
just-typed range, so unrelated edits elsewhere in the document never enter
the pipeline; (b) token/length/alphanumeric-content filters reject
punctuation-only or oversized edits; (c) repetition requirement (default 2
occurrences in 7 days) before a suggestion is ever shown, filtering one-off
edits that aren't a recognition error; (d) suppressed entirely while ASR is
actively recording, during onboarding, or if the trigger is already a saved
dictionary entry; (e) always ends in a human accept/dismiss decision — this
is a suggestion engine, not silent auto-training.

**5. Portability to Voxi.** Not portable system-wide. The entire mechanism
rests on macOS's Accessibility API letting one process passively observe
live text-buffer edits inside an arbitrary other application's focused
field — a capability with no Wayland equivalent. Wayland's `text-input-v3`/
`input-method-v2` protocols cover active IME composition, not passive
post-hoc observation of edits a user makes with an unrelated app after the
fact, and are not implemented by every toolkit. So there is no
Wayland-native way to watch "did the user just retype what Voxi typed into
some arbitrary GTK/Qt/Electron app." The underlying *algorithm*, however, is
fully portable: it's a generic diff-anchor-and-gate technique that needs no
macOS-specific capability, only both the "before" and "after" text under
the same process's control. Voxi already has exactly that inside its own
surfaces — `internal/feedback`'s stored dictation history / a future
`voxi monitor` or GNOME-companion edit view, where Voxi owns both what was
typed and what the user subsequently edits it to.

**Verdict: Adopt in reduced form**, scoped to Voxi's own
history/feedback/monitor UI, not as a system-wide interceptor. Concretely:

- `internal/feedback` already models one-shot corrections
  (`Add`/`AddSilenceArtifact`) manually invoked via `voxi feedback`. The
  gap is *detecting* a correction automatically when a user edits a stored
  transcript (e.g. in a future `voxi monitor` transcript-review pane or a
  GNOME-companion "fix last utterance" affordance), rather than requiring
  the user to type a feedback command from scratch.
- A small new helper (e.g. `internal/speechcontext/correction.go`) could
  port the diff-and-token-expand logic verbatim from
  `AutomaticDictionaryCorrectionDetector` (it's ~80 lines of
  string-range arithmetic, license-compatible under GPLv3→AGPLv3, and has
  no Swift/Foundation dependency beyond string handling) to turn
  (originalTranscript, editedTranscript) into a candidate (heard,
  corrected) pair.
- Gate acceptance the same way: require the pair to recur across N stored
  chunks/sessions (Voxi's `internal/history`/chunk ring buffer already
  retains recent transcripts — issue 053 — so occurrence counting is
  cheap) before proposing it, and always require explicit user
  confirmation (e.g. `voxi feedback vocabulary suggest` printing pending
  candidates) before it lands in `internal/speechcontext`'s vocabulary
  file — never silent auto-write.
- This is new feature scope, not a bug fix — worth its own follow-up
  ticket if pursued, scoped to "detect corrections within Voxi's own
  stored history/monitor UI," explicitly not "observe arbitrary
  third-party app text fields" (infeasible per above).

## 6. Background

Raised 2026-09-06 after the user asked to evaluate FluidVoice
(`https://github.com/altic-dev/FluidVoice`) as a source of adoptable ideas
for Voxi. Initial scan established: the app is 100% Swift on macOS-only
frameworks (Speech, Accessibility API, CoreAudio, CoreML) — the README's
"Windows... on the way" is a waitlist signup, not existing code, so there is
no cross-platform core to fork or clone. Licensing: FluidVoice is GPLv3,
Voxi is AGPLv3 — compatible if literal code were ever reused, though nothing
here calls for that given the disjoint stacks (Swift/macOS vs Go/Linux).
