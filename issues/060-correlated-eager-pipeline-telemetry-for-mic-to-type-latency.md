# 060 — Correlated Eager Pipeline Telemetry for Mic-to-Type Latency

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Major
**Category**: Observability
**Related**: [053 recent audio chunks](053-ring-buffer-recent-audio-chunks-and-transcripts.md), [056 end-to-end stress testing](056-end-to-end-stress-session-testing-with-noise-and-load.md), [057 delayed typing after stop](057-recording-start-stop-race-delayed-hallucinated-typing-after-stop-cannot-restart-recording.md), [internal/eager](../internal/eager/), [internal/chunks](../internal/chunks/)

---

## Problem

The default Eager pipeline exposes aggregate throughput and a bounded recent-chunk
manifest, but it cannot reconstruct when one user gesture became recording,
transcription, and typing work. In particular, it cannot answer whether the user
closed the microphone while old chunks remained queued or were typed much later.

Record a durable, correlated local event timeline for each Eager session and chunk.
The timeline must distinguish the exact time a `start`/`toggle` control request
activates the microphone, capture actually starts, a stop request deactivates it,
capture actually stops, each audio chunk is finalized, Whisper starts and finishes,
and typing starts and finishes.

## Scope and Design Constraints

- Assign a stable session ID at mic activation and a stable chunk ID within that
  session. Every event carries both IDs where applicable so drains from an old
  session remain distinguishable after a new session starts.
- Persist events locally with nanosecond-capable timestamps and append semantics so
  a crash does not discard already-recorded stages. Keep permissions private.
- Enrich chunk events using the captured PCM, independent of Whisper: duration,
  byte count, mean and peak RMS, voiced-frame ratio, and a documented probable-
  silence decision. Record transcript word count only after Whisper completes;
  do not store transcript content in the telemetry timeline.
- Telemetry failure must not stop dictation, transcription, or typing.
- Existing recent-chunk metadata may expose the same correlation and acoustic
  fields where useful, but is not a substitute for the longer-lived event timeline.

## Open Questions / Explicit Uncertainties

- Retention and a user-facing query/export command are intentionally deferred;
  this ticket establishes the event database and machine-readable JSONL format.
- `Super+X` is observed at the Eager daemon control request boundary. Kernel event
  timing before the request reaches Voxi is outside this ticket unless the existing
  modifier protocol already carries it.
- "Probable silence" is an acoustic heuristic, not a speech classifier. Base it on
  the existing VAD threshold and voiced-frame analysis and record the underlying
  metrics so the decision remains inspectable.

## Acceptance Criteria

- Start/stop and toggle requests produce correlated `mic_activated` and
  `mic_deactivated` events with exact wall-clock timestamps.
- Every default-Eager chunk can be traced through `chunk_finalized`, Whisper
  start/completion, and (when accepted and enabled) typing start/completion.
- Events for background drains retain the old session ID after a new mic session
  begins, making post-close transcription and delayed typing directly measurable.
- Chunk events include duration, bytes, mean/peak RMS, voiced ratio, probable
  silence, and eventual transcript word count without storing transcript text.
- The local telemetry store is append-only, private (`0600`), tolerant of write
  failures, and covered by tests asserting schema, correlation, ordering, and
  acoustic values.
- `go test ./...` and relevant repository checks pass; `make restart-service` is
  run and the live user service is confirmed active.

## Verification

- Unit-test event serialization/persistence and audio-metric calculations with
  deterministic PCM and timestamps.
- Exercise an Eager capture session with fake capture/transcription/typing paths
  where practical and assert session/chunk IDs and stage ordering.
- Run `go test ./...`, repository checks, and inspect the installed service after
  restart.
