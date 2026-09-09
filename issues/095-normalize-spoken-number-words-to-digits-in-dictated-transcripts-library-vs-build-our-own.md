# 095 — Normalize spoken number words to digits in dictated transcripts (library vs. build-our-own)

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Enhancement
**Category**: ASR Quality
**Related**: [internal/asr/asr.go](../internal/asr/asr.go) (`CleanWhisperTranscript` — the existing
deterministic post-processing chain this would join), [063 FluidVoice spoken-punctuation research](063-fluidvoice-spoken-punctuation-and-dictation-literal-post-processing-rules-research.md)
(adjacent "dictation-literal formatting" territory — that research found FluidVoice does *not*
actually do number formatting either, so this is a genuinely new gap, not something to copy from
there), [spec/models.yaml](../spec/models.yaml) (current default `cohere-transcribe-03-2026`)

---

## 1. Problem

Reported directly by the user while dictating this ticket (2026-09-09): spoken numbers
("two hundred eighty-eight", "seventy-six", "ninety-three", "five hundred seven") are sometimes
transcribed as spelled-out words rather than digits. The user's stated preference: numbers should
**always** come out as digits (`288`, `76`, `93`, `507`), not word form, when dictating.

This is inconsistent with plain digit output the ASR backend sometimes already produces
unprompted — this ticket is about making that behavior consistent and guaranteed, not building
number recognition from nothing. No existing code path in `internal/asr` normalizes number words
today; `CleanWhisperTranscript`'s chain (hallucination stripping, leading-dash-fragment stripping,
093/094's repeated-clause collapsing) has no number-formatting step.

## 2. Proposed Approaches

The user asked for exactly this framing: either adopt a library, or build a minimal one ourselves.
Both are viable; evaluate before committing.

### 2a. Adopt an existing library

A few real candidates exist in the Go ecosystem (found via search, not yet vetted for license,
maintenance activity, or correctness against Voxi's actual failure cases — do that before
depending on any of them):

- [`github.com/donna-legal/word2number`](https://github.com/donna-legal/word2number) — "three
  hundred thousand" → `300000`.
- [`github.com/aasmall/word2number`](https://github.com/aasmall/word2number) — appears to be a
  fork/variant of the above; compare which is better-maintained before picking one.
- [`github.com/pablodz/word2number`](https://github.com/pablodz/word2number) — claims multi-language
  support (English, Spanish, Portuguese) via a `Text2NumEN`-style API, which could matter if Voxi's
  Cohere backend's multilingual capability (per `spec/models.yaml`'s model label) is ever exercised
  for dictation in another language.

Per this repo's `docs/Go.md` convention (avoid unneeded deps), only add one of these if it actually
covers Voxi's real failure cases better than a small hand-rolled version would — check licensing,
whether it's actively maintained, and run it against a corpus of real spoken-number transcripts
(see §4) before deciding.

### 2b. Build a minimal converter

English cardinal number-word-to-digit conversion is a well-bounded, well-known algorithm (ones/
teens/tens/scale-word table — "hundred", "thousand", "million" — plus straightforward
concatenation rules) that doesn't obviously need an external dependency. A hand-rolled version can
be scoped exactly to what Voxi needs (cardinal numbers in ordinary dictated speech) rather than a
general-purpose library's broader surface (ordinals, fractions, multiple languages, currency
formats) that Voxi doesn't need yet.

## 3. Design Considerations (either approach)

- **Ambiguity with legitimate words**: "one" as a pronoun/determiner ("I want the blue one") vs. a
  numeral; small number words used idiomatically ("to be or not to be", not "2b or not 2b"). A
  naive global word-swap is unsafe — this needs to be scoped to number-word *sequences* (two or
  more consecutive number words, or a single number word in a clear numeric context) rather than
  swapping every lone occurrence of "one", "two", etc. Get this wrong and it actively damages
  transcripts, the same false-positive risk class as issue 094.
- **Where it runs**: a new step in `CleanWhisperTranscript`'s deterministic chain (alongside
  `StripLeadingHallucinations`/`CollapseRepeatedTrailingClause`), so it applies regardless of ASR
  backend (Whisper or Cohere/`crispasr`).
- **Scope for v1**: cardinal numbers only (matching the user's examples — no ordinals like
  "twenty-third", no currency/decimal handling) unless real dictation samples show those are common
  enough to matter.
- **Opt-out**: consider whether this should be unconditional or a toggle — the user's stated intent
  is "always," but confirm there's no existing per-user preference mechanism (`spec/models.yaml`,
  local feedback overrides) this should hook into for consistency with how other post-processing
  toggles work in this codebase (e.g. `--speech-context=false`).

## 4. Verification

- Table-driven Go unit tests covering the number word forms in the user's own report ("two hundred
  eighty-eight", "seventy-six", "ninety-three", "five hundred seven") plus common edge cases
  (teens, compound tens, "hundred"/"thousand" scale words, zero).
- At least one regression test confirming legitimate non-numeric use of small number words ("the
  blue one", "one moment please") is left untouched.
- If real dev samples of the spelled-out-number failure exist or recur, capture one via the
  dev-sample recorder (issue 042 / `voxi feedback sample save-chunk`) as a fixture, per this
  project's general preference for a real repro over a synthetic one alone.
