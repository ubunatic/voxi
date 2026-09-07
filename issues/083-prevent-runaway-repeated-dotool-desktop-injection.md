# 083 — Reject Pathological Repetitive ASR Output Before Desktop Injection

**Status**: Open
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
