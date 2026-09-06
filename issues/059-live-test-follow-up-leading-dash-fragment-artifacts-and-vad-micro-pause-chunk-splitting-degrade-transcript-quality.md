# 059: Live Test Follow-Up: Leading Dash-Fragment Artifacts and VAD Micro-Pause Chunk Splitting Degrade Transcript Quality

**Status**: Closed — implemented in 604d0f9
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Bug
**Related**: [054 short-pause acoustic gating](054-short-pause-acoustic-gating-and-context-priming.md), [049 leading hallucination subs byuk](049-leading-hallucination-subs-byuk.md), [058 configurable scope for hallucination fixes](058-configurable-scope-for-hallucination-fixes-all-chunks-vs-first-chunk-only.md), [057 recording start/stop race](057-recording-start-stop-race-delayed-hallucinated-typing-after-stop-cannot-restart-recording.md), [internal/asr/asr.go](../internal/asr/asr.go), [internal/audio/audio.go](../internal/audio/audio.go), [internal/eager/eager.go](../internal/eager/eager.go)

---

## 1. Problem & Motivation

User-reported, live manual test of the dictation pipeline (2026-09-05),
performed immediately after [057](057-recording-start-stop-race-delayed-hallucinated-typing-after-stop-cannot-restart-recording.md)
was fixed and committed, specifically to re-verify that fix. The 057
re-verification itself passed (recorded as a live-verification note on 057).
This ticket covers two separate, unrelated-to-057 transcript-quality
problems the same test surfaced, given in the user's own words:

### 1.1 Leading dash-fragment artifact typed before the real sentence

The user said: *"I will now check if the transcribe immediately starts
after I close the session with a Super X."* What was actually typed began
with a stray, unrelated fragment prefix:

```
-Transcribe. I will now check if the transcribe immediately starts after I close the session with a Super X.
```

The user's own framing: *"Argh! we got a '-Transcribe' sneak in, lets
remove and leading '-<word>'."*

### 1.2 VAD micro-pause splits one continuous utterance into two chunks

The user said, as one continuous utterance: *"And this should be the 3rd
chunk."* It was typed as two separately-transcribed chunks:

```
and this should be done. Third chunk.
```

i.e. the VAD segmenter cut the utterance at an internal micro-pause,
Whisper transcribed each half independently with no shared context, and
the first half was completed with a plausible-sounding but wrong ending
("should be done") instead of the intended "the third chunk" — the words
"third chunk" only survive, disconnected, as their own following chunk.

(For contrast/completeness: the user's second sub-utterance, "This should
be the 2nd chunk", came out as "This should be the second chunk" — digits
spelled as words is expected Whisper behavior, not a bug, and is **not**
part of this ticket. The user's first sub-utterance, "This should be the
1st chunk", came out as "Let's show Peter Frost's chunk." — a full-sentence
misrecognition; investigated below but not conclusively root-caused, see
§2.3.)

## 2. Investigation

### 2.1 Leading dash-fragment: no mechanism catches this pattern

`internal/asr/asr.go`'s `IsSafeToType`, `StripLeadingHallucinations`, and
`StripTrailingHallucinations` all operate on the model's known `stop_words`
list (`spec/models.yaml`) — literal/regex phrases like "thanks for
watching" or "subtitles by.*". None of them match a generic leading
`-<fragment>` punctuation pattern; a `"-Transcribe."`-shaped prefix is not
a known stop-word and is not rejected by any existing filter, so it passes
`IsSafeToType` and gets typed like any genuine chunk.

This is **not a fluke specific to this test** — this exact class of
artifact is already independently documented:
- [issue 054 §1](054-short-pause-acoustic-gating-and-context-priming.md)
  lists observed short-pause hallucinations including `"-Trap."` and
  `"-D, etc."` — the same leading-dash shape.
- The current live chunk ring buffer (`voxi chunks list`, captured during
  this investigation, unrelated recording session from ~11:36–11:38) shows
  chunk `#186`, **accepted** and typed: `"-H. Also file a follow-up ticket
  that..."` — a leading dash-fragment glued onto the front of an otherwise
  genuine, correctly-transcribed sentence, in exactly the shape the user
  hit.

**Root cause, per `runEagerCaptureSession` (`internal/eager/eager.go`
~L459-495):** each VAD-detected utterance is dispatched as its own
independent `TranscribeJob` (`internal/audio/audio.go`'s
`AudioSegmenter.ProcessFrame`/`Flush`), transcribed, and (if accepted)
typed immediately — there is no mechanism to detect or merge a spurious
short garbled lead-in with the "real" sentence that follows it, whether
the dash-fragment lands as its own separate chunk or, as chunk #186 shows,
as a prefix fused onto the front of one larger transcript. This is
distinct from [054](054-short-pause-acoustic-gating-and-context-priming.md)'s
acoustic gate (`MinVoicedFrames`/`MinVoicedRunFrames`/`MinMeanRMS`), which
only rejects pure noise/silence transients before they reach `voxtype` —
chunk #186 and the user's `"-Transcribe."` case both contain genuine voiced
speech energy (they were accepted, not `rej:low_energy_transient`), so
054's acoustic gate does not and should not reject them; the problem is
purely in Whisper's own decoding of a boundary-clipped or otherwise
ambiguous lead-in into a garbled, punctuation-prefixed token run that no
downstream text filter catches.

Also distinct from [058](058-configurable-scope-for-hallucination-fixes-all-chunks-vs-first-chunk-only.md):
058 is about *when* existing filters apply (first-chunk-only vs. every
chunk); this ticket is about a *missing filter* — no existing mechanism,
at any scope, recognizes a generic leading `-<word>` fragment shape at
all. Chunk #186 above additionally shows this artifact is not confined to
the first chunk of a session, so a first-chunk-only scope toggle (058)
would not fully address it.

### 2.2 VAD micro-pause chunk splitting: known, tunable, currently un-mitigated tradeoff

`internal/audio/audio.go`'s `DefaultSegmenterOptions` sets `SilenceMs: 800`
— any pause of ≥800ms inside continuous speech is treated as an utterance
boundary and forces a chunk split (`AudioSegmenter.ProcessFrame`,
`consecutiveSilence >= s.silenceFramesNeeded`). A natural micro-pause
before "3rd" (e.g. a breath or brief hesitation) longer than 800ms is
enough to trigger this. Each resulting chunk is transcribed independently
by `voxtype` with no shared context between them (no carry-over of the
previous chunk's audio or text into the next chunk's decode), so Whisper
has no way to know the two chunks are one continuous sentence — it fills
in a plausible-sounding completion for the truncated first half instead of
leaving it visibly incomplete.

This is an architectural tradeoff of the per-chunk, low-latency,
type-as-you-go design (each chunk must be typed immediately per the user's
own target workflow, see [058 §1](058-configurable-scope-for-hallucination-fixes-all-chunks-vs-first-chunk-only.md)),
not a hard bug — `SilenceMs` is an existing, already-tunable knob. No prior
ticket proposes tuning it or mitigating cross-chunk context loss
specifically; [054](054-short-pause-acoustic-gating-and-context-priming.md)'s
short-pause work addressed *false-positive* chunks (pure noise/silence
misidentified as speech), not this *true-positive-but-badly-split*
case (genuine continuous speech legitimately triggering a pause-based
split at a bad boundary).

### 2.3 First sub-utterance misrecognition ("Let's show Peter Frost's chunk.")

Investigated but **not conclusively root-caused**. Possible explanations,
none confirmed:
- Segmenter chunk-boundary clipping (as in §2.2) truncating the actual
  start or end of "This should be the 1st chunk", feeding `voxtype` a
  partial signal it filled in with a plausible-but-wrong full sentence.
- Plain ASR misrecognition unrelated to boundary/segmenter effects — the
  `small.en` model is not immune to ordinary accuracy errors, especially
  on short, low-context utterances (no similar term is in
  `spec/models.yaml`'s `speech_context.terms` vocabulary hint list, so no
  vocabulary-biasing explanation applies either).

The exact chunk audio from this test could not be inspected to disambiguate
these (see §2.4) — recorded here as an open question for a future pass
rather than asserted as a fixed root cause.

### 2.4 Ring buffer / journalctl evidence: rotated past this test

`voxi chunks list` was checked during this investigation but the 10-slot
ring buffer had already rotated past the user's test session — it showed
only chunks `#184`–`#193` from an unrelated later recording session
(~11:36–11:38), not the test transcripts quoted in §1. `journalctl --user
-u voxi-agent.service --since "-2 hours"` confirmed multiple
`start`/`stop` cycles consistent with the user's described test sequence
but carries no transcript content to cross-check against. The chunk #186
evidence cited in §2.1 is from this later, unrelated session — it
corroborates the *pattern* (leading dash-fragments are recurring, not a
one-off), not the exact audio/text from the user's original test. This is
reported honestly as a gap rather than fabricated.

## 3. Suspected Areas / Implementation Plan

1. **Leading dash-fragment stripping (§2.1)**: add a generic pattern-based
   strip in `internal/asr/asr.go` (alongside `StripLeadingHallucinations`)
   that recognizes a short, punctuation-prefixed leading fragment shape
   (e.g. `^-\s*\S+\.?\s+` before a longer, well-formed remainder) and
   strips it, independent of the model-specific `stop_words` list. Needs
   care to avoid stripping legitimate dictated text that happens to start
   with a hyphen (e.g. a real word or code token the user intentionally
   spoke/typed) — scope the heuristic tightly (very short fragment, ≤1-2
   words, directly followed by a capitalized sentence start) and add unit
   tests for both the artifact case and legitimate-hyphen-prefix
   non-stripping.
2. **VAD micro-pause splitting (§2.2)**: no code change proposed yet —
   record as a known tradeoff for a future design discussion (e.g.
   raising `SilenceMs`, or feeding limited trailing-audio/text context
   from the previous chunk into the next chunk's `voxtype` invocation via
   `--initial-prompt`, weighed against 054's finding that `--initial-prompt`
   priming increases hallucination risk on ambiguous audio).
3. **First sub-utterance misrecognition (§2.3)**: no action — insufficient
   evidence to root-cause; left open for a future pass if the pattern
   recurs with inspectable chunk audio.

## 4. Acceptance Criteria

- A generic leading dash-fragment heuristic exists in `internal/asr/asr.go`
  and is unit-tested against both the `"-Transcribe."` / `"-H."` artifact
  shape and legitimate hyphen-led dictated text (no false-positive strip).
- `go test ./...` passes; `make restart-service` run since
  `internal/asr`/`internal/eager` are on the live daemon's path.
- VAD micro-pause splitting (§2.2) is documented as a known, tunable
  tradeoff (this ticket) rather than silently left undocumented; no code
  change required to close this ticket unless a maintainer chooses to
  scope one in.

## 5. Non-Goals

- Not a fix for [057](057-recording-start-stop-race-delayed-hallucinated-typing-after-stop-cannot-restart-recording.md) —
  that bug is fixed and separately live-verified by this same test session.
- Not the scope-toggle feature in [058](058-configurable-scope-for-hallucination-fixes-all-chunks-vs-first-chunk-only.md) —
  this ticket is about a missing filter pattern, not filter scope/timing.
- Not a redesign of the VAD segmenter's chunk-boundary algorithm — §2.2 is
  recorded as a known tradeoff for future discussion, not committed work.
- Not a conclusive fix for the §2.3 misrecognition — left open, insufficient
  evidence.

## Resolution

**Implemented**: commit `604d0f9` — `fix(asr): strip leading dash-fragment hallucination prefix`

### What was done (§3.1 only)

Added `StripLeadingDashFragment(text string) string` to `internal/asr/asr.go`
and wired it into both transcript cleaning code paths in `CleanWhisperTranscript`
(Strategy 1 canonical quote extraction and Strategy 2 line-by-line fallback),
applied before the existing `StripLeadingHallucinations` / `StripTrailingHallucinations`
stop-word filters.

### Heuristic design

A single compiled `leadingDashFragmentRe` regexp:

```
^-\s*\S+[.,!?]*\s+(?:[A-Z])
```

Matches when **all** of:
1. Text begins with a literal `-` (optional space after).
2. Exactly one non-whitespace token follows (the garbled fragment word),
   optionally trailed by punctuation and whitespace.
3. The remaining text immediately starts with an upper-case letter (new sentence).

The capital-letter assertion is the key tightening condition: it ensures the
heuristic fires only when the dash-fragment is clearly a prefix artefact fused
onto a well-formed continuation, not when the entire transcript is a
dash-fragment or when the continuation is lower-case (e.g. a dictated bullet
point).  When the match fires, everything before the capital letter is dropped
and the remainder is returned trimmed.

### Test coverage (`internal/asr/asr_test.go` — `TestStripLeadingDashFragment`)

| Case | Input | Expected output |
|---|---|---|
| Observed live artifact | `-Transcribe. I will now check…` | `I will now check…` |
| Ring-buffer chunk #186 | `-H. Also file a follow-up ticket…` | `Also file a follow-up ticket…` |
| Whole-transcript fragment | `-Trap.` | `-Trap.` (unchanged — no capital remainder) |
| Legitimate hyphen-led list item | `- first item in list` | `- first item in list` (unchanged) |

### Out of scope (unchanged)

- §3.2 VAD micro-pause splitting — `SilenceMs` and segmenter not touched.
- §2.3 misrecognition — no action, left open.
