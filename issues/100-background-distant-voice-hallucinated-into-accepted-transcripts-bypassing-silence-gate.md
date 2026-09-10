# 100 — Background/Distant Voice Hallucinated Into Accepted Transcripts, Bypassing Silence Gate

**Status**: In Progress
**Priority**: P1 (High)
**Severity**: Major
**Category**: Bug
**Related**: [083 reject runaway repeated dotool injection](083-prevent-runaway-repeated-dotool-desktop-injection.md), [093/094 CollapseRepeatedTrailingClause](094-collapserepeatedtrailingclause-wrongly-deletes-a-legitimate-short-answer-that-matches-the-question-s-last-word.md), [056 end-to-end stress testing](056-end-to-end-stress-session-testing-with-noise-and-load.md), [066 Cohere Transcribe canary](066-canary-cohere-transcribe-and-nemotron-3-5-streaming-as-alternative-asr-backends.md)

---

## 1. Problem

During a live session on 2026-09-09, two consecutive eager-pipeline chunks
were accepted and typed even though the user spoke none of that content. A
distant background voice (someone else talking in another room) was picked
up and hallucinated by the ASR (Cohere Transcribe backend) into plausible,
grammatically well-formed but entirely fabricated sentences.

Chunk diagnostics (`voxi chunks show <index>`):

- **#1532**, session `20260909T125556.419020286Z-000012`, chunk `/8`,
  2026-09-09 14:56:18, audio 8.00s, mean/peak RMS 200/8119,
  **voiced_ratio 0.367**, `probable_silence` **false**, `accepted` **true**.
  Transcript (raw==cleaned): `"I'm going to go to the hospital. I'm going to
  go to the hospital."` — a fully duplicated sentence *pair*, not a partial
  trailing-clause repeat.
- **#1533**, same session, chunk `/9`, 2026-09-09 14:56:22, audio 3.14s,
  mean/peak RMS 206/775, **voiced_ratio 0.497**, `probable_silence`
  **false**, `accepted` **true**. Transcript: `"Aye, you did that."`

Both chunks were saved via `voxi feedback sample save-chunk` and promoted
into `testdata/noise-samples/` as regression fixtures in commit `aa041b3`,
named `bg-voice-hospital-hallucination` and `bg-voice-aye-you-did-that`,
tagged `[background voice, distant/unintelligible]` in
`testdata/noise-samples/corpus.tsv`. These are real audio fixtures the
implementer can replay to reproduce.

## 2. Practical Implication

Voxi typed fabricated content into whatever the user's focused window was,
without the user having spoken. This is a false-positive accept — the
inverse failure mode of most existing hallucination/repetition work, which
targets malformed or pathologically-repetitive output from the user's *own*
speech. Here the ASR output is well-formed, non-repetitive-within-itself
(for #1533), and confident — it just isn't the user's dictation.

## 3. Why This Is a Distinct Gap

- **Not caught by the silence gate**: `probable_silence` was `false` for
  both chunks, and voiced_ratio (0.37–0.5) is high enough that a naive
  energy/VAD threshold won't reject it — the audio genuinely contains voiced
  content, just not the user's.
- **Not caught by `CollapseRepeatedTrailingClause`** (093/094, closed): that
  logic collapses a repeated *trailing clause fragment* within one
  transcript. #1532's failure mode differs — the ASR hallucinated two full,
  back-to-back, verbatim-identical sentences from a single background-noise
  stimulus. Distinct duplication shape, outside that fix's scope.
- **Related but distinct from 083** (reject pathological repetitive ASR
  output before injection; In Progress, P1/Critical): 083 is scoped around
  token/output repetition *limits* and a delivery ledger for at-most-once
  identity of accepted transcripts — bounding pathological output shape and
  duplicate delivery. This finding is upstream: the ASR produces a
  confident, well-formed, non-repetitive-within-itself hallucination from
  non-user audio. A repetition-limit or ledger fix would not catch #1533's
  `"Aye, you did that."` since it is neither repeated nor duplicated — it is
  simply wrong content accepted from audio that was never the user speaking.

## 4. Scope / Open Questions

Not prescribing an implementation. Directions worth investigating, listed as
open questions only:

- Could speaker-distance/loudness heuristics (e.g. RMS relative to the
  ambient noise floor, not just voiced_ratio) distinguish "the user, close to
  mic" from "someone else, distant" more reliably than the current gate?
- Is there a confidence/logprob signal from the Cohere Transcribe backend
  that correlates with this kind of hallucination, and that isn't currently
  surfaced or thresholded?
- Should duplicate-sentence-*pair* detection (like #1532) be a general
  acceptance-time check independent of the existing trailing-clause collapse
  (093/094), given it's a different structural pattern (whole-sentence
  duplication vs. trailing-clause repeat)?
- Relationship to 056 (end-to-end stress-session testing with noise and
  load): should this become a new stress-session assertion once a fix
  lands, using the two promoted noise-sample fixtures
  (`bg-voice-hospital-hallucination`, `bg-voice-aye-you-did-that`) as
  regression inputs?

## 5. Reproduction Assets

- `testdata/noise-samples/bg-voice-hospital-hallucination.*` (chunk #1532)
- `testdata/noise-samples/bg-voice-aye-you-did-that.*` (chunk #1533)
- Both listed in `testdata/noise-samples/corpus.tsv`, tagged
  `[background voice, distant/unintelligible]`, added in commit `aa041b3`.

## 6. Next Steps

- [ ] Decide which acceptance-time signal(s) (loudness/distance heuristic,
  ASR confidence, duplicate-sentence-pair check) to pursue, informed by the
  two fixture samples above.
- [ ] Prototype the chosen signal against the fixtures and confirm it
  rejects both #1532 and #1533 without regressing legitimate close-mic
  dictation in the existing noise-sample corpus.
- [ ] Land the accept/reject change in `internal/eager` alongside the
  existing `probable_silence`/voiced_ratio gating.
- [ ] Consider a 056-style stress-session assertion once a fix lands.

## 7. Canary and partial safety slice (2026-09-11)

The promoted fixtures were inspected with a non-injecting corpus canary. Their
energy overlaps ordinary background/noise recordings, so an unsupported RMS or
voiced-ratio threshold would risk rejecting legitimate quiet speech. Adjacent
duplicated complete sentence pairs are now rejected before typing, covering the
hospital-hallucination transcript shape, with unit regression tests and an
explicit `repeated_sentence_pair` rejection reason. The non-repetitive
`Aye, you did that.` fixture still requires a speaker/confidence signal that is
not currently available; this issue remains In Progress pending a justified
canary-driven classifier. The complete-sentence detector compares adjacent
sentences anywhere in a cleaned transcript and intentionally requires at least
three words per sentence to avoid rejecting short answers.
