# 068: Opt-In Spoken-Punctuation Formatting (`literal comma` → `,`) — Gated on User Demand

**Status**: Proposed — Deferred, no current demand
**Priority**: P3 (Low) — explicitly not a P2/P1; see Section 2 gate
**Severity**: Enhancement
**Category**: Feature
**Related**: [063 FluidVoice spoken-punctuation research](063-fluidvoice-spoken-punctuation-and-dictation-literal-post-processing-rules-research.md), [internal/asr](../internal/asr)

---

## 1. Problem & Motivation

Issue 063's research (reading FluidVoice's
`ASRService+SpokenPunctuationFormatting.swift`) confirmed Voxi has **zero**
spoken-punctuation-to-symbol conversion today — there is no way to dictate
"literal comma" and have Voxi type `,`. Whisper's raw model output is the
only source of punctuation Voxi ever produces. FluidVoice's mechanism for
this is small, fully deterministic, and cheaply portable: a phrase table
(~40 entries) that only converts when immediately preceded by a single
configurable trigger word (default `"literal"`), with a couple of optional
app-name-gated exceptions for inherently ambiguous symbols (e.g. "at sign").

This ticket exists so the design isn't lost, but is filed as **explicitly
deferred**, not queued for near-term work.

## 2. Explicit Gate — Do Not Start Implementation Until

**There is currently zero user demand for this feature.** Nobody has asked
for spoken-punctuation dictation in Voxi's own usage or issue history. Do
not implement this speculatively. Start only when one of the following
happens:
- The user (or a real usage session) actually hits a case where they want
  to dictate punctuation/symbols by voice and current Whisper output can't
  produce it reliably, **or**
- The user explicitly asks to build it.

If this ticket is revisited purely because it's "easy" or "already
designed," that is not sufficient reason on its own — re-confirm demand
first.

## 3. Desired Design (When/If Implemented)

Mirrors FluidVoice's approach, adapted to Voxi's Go pipeline:

1. A static phrase table (Go map or table-driven struct) mapping spoken
   phrases (`"comma"`, `"period"`, `"question mark"`, `"open paren"`,
   `"at sign"`, etc.) to literal symbols — start with FluidVoice's ~40-entry
   set as a reference, trim to what's actually useful for Voxi's use cases
   (dictation into code/terminal contexts especially).
2. A single configurable trigger word (default e.g. `"literal"`), matched
   immediately before a phrase-table entry, so "literal comma" → `,` but a
   sentence that legitimately contains the word "comma" is untouched. No ML
   or contextual disambiguation needed — this is the same reasoning
   FluidVoice used and it's sufficient.
3. Land as a new deterministic pass in `internal/asr`'s `CleanWhisperTranscript`
   pipeline, running **after** existing hallucination-stripping (dash-fragment
   fix, filler-word removal) — order matters, since punctuation-phrase
   matching should happen on already-cleaned text.
4. Opt-in via a config flag (off by default, unlike issue 046's speech-context
   biasing which shipped default-on only after its own measurement gate) —
   this is a behavior change to what gets typed, not a recognition-quality
   improvement, so it should not surprise existing users.
5. Skip FluidVoice's app-name-sniffing gates (`requiresAtSignPunctuationApp`
   and similar) for a first cut — Voxi doesn't currently have an
   active-app-detection mechanism wired into `internal/asr` the way
   `internal/speechcontext`/`ActiveAppMonitor`-equivalent context does (check
   `internal/speechcontext` for reusable app-context signal before deciding
   whether to add this in v1 or defer it).
6. Table-driven Go tests mirroring FluidVoice's phrase-rule table, covering:
   trigger-prefixed conversion, untriggered literal words left alone, and at
   least one ambiguous-symbol edge case if the app-context gate is included.

## 4. Non-Goals

- Slash-command formatting (`"slash foo"` → `/foo`) and @mention formatting
  (`"tag Bob"` → `@Bob`) — issue 063 identified these as a separate, lower-
  priority, more niche/app-context-dependent feature. Do not bundle them
  into this ticket if it's ever implemented; file separately if wanted.
- No implementation, code, or canary work is authorized by filing this
  ticket — see Section 2.

## 5. Background

Raised 2026-09-06, following issue 063's research verdict recommending a
follow-up implementation ticket. Filed at the user's explicit request to
capture the design, with the user's own instruction to keep it low-priority
and demand-gated rather than scheduled.
