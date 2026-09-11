# 083 — Reject Pathological Repetitive ASR Output Before Desktop Injection

**Status**: In Progress
**Priority**: P1 (High)
**Severity**: Critical
**Category**: Bug
**Related**: [056 end-to-end stress testing](056-end-to-end-stress-session-testing-with-noise-and-load.md), [057 delayed typing after stop](057-recording-start-stop-race-delayed-hallucinated-typing-after-stop-cannot-restart-recording.md), [074 Cohere backend integration](074-wire-cohere-transcribe-in-as-an-additional-selectable-asr-backend.md), [082 transcript replacements](archive/082-deterministic-post-transcription-vocabulary-replacements-for-cohere.md)

---

## 1. Problem & Motivation

A user reported that Voxi typed a short `uktuk`-like sequence hundreds of
times. Retained chunk evidence now shows that Cohere produced one pathological
giant token beginning `Ubuntuktuktuktuk...`; Voxi accepted that result and
submitted it to typing once. This was malformed repetitive ASR output, not
evidence of hundreds of dotool invocations.

Recording had been stopped **before the first recognized word/transcript
appeared**. The primary defect is missing transcript sanity validation before
an irreversible desktop side effect. Post-stop behavior remains part of the
safety contract because a late result should not surprise the user after stop.

Uncontrolled synthetic keyboard input can corrupt documents, trigger shortcuts,
or continue acting on whichever window gains focus. Voxi must ensure that one
accepted transcript is submitted for typing at most once, stopping prevents or
strictly bounds post-stop output, and runaway injection can be halted promptly.

## 2. Evidence and Investigation Questions

### Confirmed evidence

- The user observed a short `uktuk`-like sequence typed hundreds of times; the
  initial description of dotool looping was an understandable symptom report,
  not a process-level diagnosis.
- Recording was stopped before any recognized word/transcript appeared.
- `voxi chunks show 90` identifies session
  `20260907T192950.546562862Z-000001`, chunk `/1`: 6.74 seconds of audio,
  Cohere transcription in 2.455 seconds, and one enormous raw/cleaned token
  beginning `Ubuntuktuktuktuk...`. Metadata records
  `transcript_word_count: 1`, `accepted: true`, typing start at
  `21:30:00.189798`, and completion at `21:30:00.194410` (about 4.6 ms).
- Those timestamps and the single chunk establish that Voxi handed one giant
  accepted result to the fast dotool path once. They do not indicate repeated
  Voxi injection attempts or a stuck dotool process.
- The eager worker currently invokes `typing.TypeText(context.Background(), d,
  text+" ")` for accepted chunks. This context is detached from the recording
  session, so session cancellation does not necessarily cancel in-flight typing.
- `TypeText` sends a generated script to persistent `dotoolc` when its FIFO is
  ready and otherwise starts `dotool`. Transcript acceptance has no explicit
  extreme-token, output-size, or repeated-substring circuit breaker.
- Existing telemetry identifies typing by session/chunk, but the inspected
  service journal contains recording lifecycle lines only and does not identify
  the responsible transcript or process.

### Hypotheses to test, not assumed causes

- Cohere/ASR completed after stop or its accepted result remained eligible for
  typing after stop. Establish the intended cancellation/draining contract and
  exact timeline.
- Stop may drain accepted work rather than cancel it, or stale results from an
  old session may be typed after a new session starts.
- The malformed token may arise from a Cohere decoding failure on this audio or
  from parsing/cleanup. Preserve the fixture and isolate which layer produced it.
- Issue 082 added Cohere post-processing and the service was restarted during
  rollout. There is no evidence either caused the incident; inspect sequencing
  only to establish a timeline.

### Fresh reproduction: legitimate final speech killed on stop (2026-09-11)

The same stop-boundary failure happened again during an ordinary daily-driver
dictation session. Chunks `#46`–`#51` in session
`20260911T070942.882005086Z-000002` were accepted and typed normally, including
the preceding fragment `"Also, the"`. The next valid 8.00-second voiced chunk
`#52` (`voiced_ratio=0.7625`, finalized at `09:10:04.117`) entered
transcription, ran for 1.44 seconds, and ended with:

    transcribe_error: signal: killed

It has no transcript and no typing timestamps. The following 1.34-second chunk
`#53` started immediately afterward but was stopped before transcription could
run and ended with:

    transcribe_error: context canceled

Chunk `#54` was only a low-energy trailing transient. This confirms that the
user's final spoken content can be lost even when the preceding chunks arrive
without interruption: stopping cancels the in-flight transcription subprocess
and the next queued job. This has now happened more than once and is elevated
to **high-priority work** within the broader injection-safety contract. The
fix must preserve the no-stale-typing guarantee while recovering or otherwise
surfacing legitimate speech that was already captured at stop time.

### Intended daily-driver stop/flush workflow

The usual interaction is `Super-X` to start, several spoken chunks, then
`Super-X` again to finish quickly. The final toggle is an explicit instruction
to stop capturing immediately, flush the remaining captured audio, transcribe
it, and type the result without waiting for another silence boundary. It is not
an instruction to discard or kill legitimate speech already captured. The
current cancellation behavior therefore conflicts with the normal user
contract whenever the final toggle arrives while a chunk is in flight; the
implementation must distinguish this deliberate final flush from stale work
that should be prevented from typing.

Correlate history, retained chunk metadata/audio, eager telemetry, service
journal, generation/session IDs, and process lifetime where available. Preserve
the distinction between repeated content in one transcript and duplicate
delivery of one transcript.

## 3. Scope and Safety Requirements

- Add conservative transcript sanity validation between ASR cleanup/replacement
  and acceptance/typing. Detect extreme token length, pathological repeated
  substrings/runs, and unjustifiably large output.
- Calibrate limits against legitimate long words, URLs, hashes, code, repeated
  punctuation, and ordinary emphatic speech. Prefer explainable bounded rules
  over fuzzy or semantic rewriting.
- Preserve rejected output in privacy-appropriate chunk diagnostics: rejection
  reason, safe length/digest or bounded prefix, detected repeat unit/count, and
  provider/model. Rejected text must never reach typing or history as accepted
  dictation.
- Trace ownership from finalized audio through ASR, acceptance, replacement,
  queueing, `TypeText`, and process exit to place the guard at the single shared
  pre-side-effect seam.
- Define stop semantics for queued audio, in-flight ASR, completed late results,
  queued typing, and active typing. Results must carry a generation/session ID
  checked immediately before every irreversible side effect.
- Establish an at-most-once contract for each accepted event identity across
  cancellation, retries, errors, daemon restart, and overlapping sessions.
- Bound a single operation by characters and repetition characteristics. Fail
  closed with a visible diagnostic rather than injecting suspicious output.
- Propagate cancellation into typing. Toggle-off/service stop must cancel queued
  and active work, clean up owned processes/FIFO writers, and leave no detached
  injector.
- Provide an emergency stop that clears pending Voxi work and stops injectors
  Voxi owns without broadly killing unrelated user processes.
- Define bounded backpressure and partial-write/retry behavior; no failure may
  silently replay a whole script.
- Record stable event identity, safe text length/digest or bounded preview,
  attempt count, selected path, enqueue/start/end/cancel reason, process ID,
  duration, and circuit-breaker activation without logging private dictation by
  default.

## 4. Acceptance Criteria

- The captured `Ubuntuktuktuktuk...` token shape is rejected before typing with
  a specific diagnostic and rejection metadata.
- Repeated-substring/run and extreme-token/output rules reject pathological
  results while accepting representative long words, URLs, hashes, code, and
  normal repeated speech.
- Once stop is acknowledged, no stale-generation ASR result begins typing, or a
  deliberately different drain policy strictly bounds and clearly communicates
  post-stop output. Any already-started output is cancelled within a documented
  bound.
- Tests cover stopping while Cohere/ASR is in flight, after ASR completion but
  before dequeue, during typing, and immediately followed by a new session.
- Each accepted transcript identity reaches injection at most once; duplicate
  delivery is rejected and observable.
- Stop/service shutdown leaves no Voxi-owned dotool child, blocked FIFO writer,
  or pending typing event.
- Suspicious repetition or oversized output cannot produce hundreds of injected
  characters; limits are documented, spec-owned where appropriate, and covered
  by false-positive tests.
- Rejected pathological transcripts produce zero captured dotool commands and
  are observable as rejected chunks rather than typing successes.
- The emergency stop remains usable during active typing and does not target
  unrelated dotool users.
- FIFO-daemon and standalone paths have explicit timeout, cancellation, partial
  write, and retry semantics; neither implicitly replays an entire script.
- Diagnostics distinguish one repeated transcript, duplicate delivery, and a
  late post-stop result without storing full transcript text by default.
- Normal multi-chunk dictation and issue 082 replacements still type expected
  text exactly once.

## 5. Verification Plan

- Unit-test repeated substring/run detection, giant single tokens, total output
  limits, boundary values, Unicode, URLs, hashes, code, normal repetition,
  stale generations, late completion, and cancellation.
- Extend eager integration tests with captured dotool commands and process
  lifecycle assertions across rapid toggle cycles, queue saturation, errors,
  service cancellation, and old/new session overlap.
- Exercise both `dotoolc` FIFO and standalone paths with disposable fake
  executables/FIFOs and strict deadlines.
- Add an exact bounded regression fixture based on the retained
  `Ubuntuktuktuktuk...` shape and prove it is rejected with zero injection.
- Every canary must replace injection with a capture sink. Never write to
  `/tmp/dotool-pipe`, invoke the real desktop injector, or depend on focus.
- Run `go test ./...`, `make check`, `git diff --check`, and isolated
  non-injecting canaries before closure.

## 6. Non-Goals

- Claiming dotool looped; retained evidence instead shows one giant transcript
  submitted once.
- Blaming Cohere replacements, service restart, or eager queueing without proof.
- Removing legitimate repeated words solely because they repeat.
- Killing all system-wide dotool processes or modifying unrelated keyboard
  behavior.
- Using the real focused desktop as a test target.

## 7. Implemented Safety Slice (2026-09-07)

Implemented the immediate critical boundary: spec-owned output/token/repetition limits,
the exact retained `Ubun` + repeated `tuk` regression with zero captured injector calls,
privacy-safe rejection metadata, session-derived ASR and typing cancellation, no flush
after stop, and no whole-script fallback replay after a failed FIFO attempt.

The issue remains open because the broader acceptance contract still needs an explicit
cross-session delivery ledger/duplicate telemetry, injector attempt/process telemetry,
and exhaustive FIFO/standalone process-lifecycle and active partial-write canaries.

## 8. Regression: trailing utterance silently dropped on every normal stop (2026-09-08)

The §7 "no flush after stop" and session-derived ASR cancellation were too broad: tying
*all* transcription/typing to the session `ctx` and gating `segmenter.Flush()` on
`ctx.Err() == nil` meant every ordinary Super-X stop — not just a stale/pathological
late result — dropped whatever utterance was still buffered but not yet VAD-finalized
at the moment of stop. User-reported: a three-chunk dictation test where the chunk
spoken right up to the stop keypress never appeared.

Fixed in `internal/eager/eager.go` (commit `8795182`): the trailing
`segmenter.Flush()` job is now marked `Final` and runs its transcription/typing on a
context detached from the already-canceled session `ctx`, while every other job (one
genuinely still mid-transcription when stop arrives, e.g. a stale/pathological result)
keeps the tested abort-and-never-type behavior this issue introduced
(`TestStopCancelsInflightTranscriptionBeforeInjection`,
`TestPathologicalTranscriptProducesZeroInjection`). New regression test:
`TestStopFlushesTrailingUtteranceForTranscriptionAndTyping`.

This is a caution for the remaining open work in §3/§8: the at-most-once/generation-ID
design still needs to distinguish "the last thing the user said, right up to stop" from
"a stale or pathological result arriving after stop" — collapsing both into one
`ctx.Err()` check reintroduces this regression.

## 9. Durable at-most-once delivery slice (2026-09-10)

Implemented the scoped accepted-transcript delivery contract without changing the
trailing-utterance flush policy. Each accepted eager chunk now carries its stable
`sessionID/chunkID` identity through immediate and modifier-buffered delivery. A
file-locked, fsynced ledger claims that identity before the irreversible injector
submission; duplicate claims are rejected and recorded as `delivery_duplicate`,
including across daemon/process restarts. The typing boundary now exposes one
injector attempt with selected path, completion, cancellation/error, duration, and
child PID when the production process runner is used. `injector_started` and
`injector_completed` telemetry records that lifecycle without storing dictation text.

Focused tests cover persistent duplicate rejection and the injectable process
attempt boundary. Issue 083 remains In Progress: emergency stop, exhaustive
active-process/FIFO lifecycle canaries, and the broader stop/generation policy are
not claimed by this slice.

## 10. Bounded final-Super-X drain implementation (2026-09-11)

Implemented in commit `df6af2b` after the fresh chunk `#52`/`#53` reproduction.
The final Super-X now means “stop capture and flush,” matching the daily-driver
workflow: all audio captured before the request may finish transcription and
typing promptly, including a subprocess already in flight, queued jobs, and the
segmenter's final buffered job.

The stop request is timestamped before cancellation and passed into the capture
session. A completed read before that boundary is preserved even if cancellation
is observed before frame processing; a read completing after the boundary is
discarded. A bounded five-second drain lease keeps the control path fast while
allowing eligible work to complete asynchronously. Generation eligibility is
checked before the durable delivery claim and again before injection, including
modifier-buffered delivery. Added coverage combines in-flight, queued, and
segmenter-flushed jobs, generation overlap, lease expiry, cancellation/read
interleaving, and ordered exactly-once typing.

The issue remains In Progress. Emergency stop, exhaustive FIFO/standalone
injector lifecycle canaries, and the documented crash window between durable
claim and injector submission remain open.
