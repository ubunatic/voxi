---
title: Eager Delivery Safety
weight: 25
---

# Eager Delivery Safety

This document records the delivery and stop semantics established during the
September 2026 injection-safety work. The authoritative implementation remains
in `internal/eager`, `internal/typing`, and `internal/asr`; issue 083 tracks the
remaining full safety contract.

## Safety pipeline

An eager chunk carries a stable `sessionID/chunkID` from audio finalization
through transcription, acceptance, and typing. Before an irreversible typing
attempt, the delivery ledger claims that identity under a file lock and fsyncs
the claim. A second claim, including one made by a fresh daemon process, is
rejected and recorded as `delivery_duplicate`.

The injector boundary records selected path, attempt, process ID when available,
duration, cancellation/error, and completion telemetry. Dictation text is not
written into these lifecycle records by default.

## Stop and queue semantics

Stopping capture is deliberately fast: the recording process is killed and
reaped, while already-queued transcription work drains asynchronously. A normal
job observed by the worker after cancellation uses a bounded detached context so
accepted speech is not silently lost. A job already running when cancellation is
observed remains session-bound and is canceled. The segmenter's trailing `Final`
flush is also detached because it represents speech buffered before stop.

This distinction is important. Detaching every job would allow stale ASR output
to type after stop; canceling every job drops the last words the user spoke.
`queuedJobContext` captures the worker's queue-state decision once and applies it
to both transcription and typing.

## User-visible failures

Hard audio-write, transcription, and typing failures are emitted through the
eager output stream. Direct CLI users see stdout; the daemon's inherited stream
is captured by the systemd user journal. Expected empty, silence-artifact,
stop-word, pathological, and other normal acceptance rejections remain quiet.
Buffered modifier-release flushes use the same diagnostic path as immediate
typing.

## Conservative transcript safety

The ASR boundary rejects extreme/pathologically repetitive output before typing
and stores privacy-safe structural metadata. It also rejects adjacent duplicated
complete sentence pairs, a distinct failure shape from a repeated trailing
clause. These guards are explainable and bounded; they do not attempt semantic
rewriting.

## Known limits

The current acoustic gate detects sustained voiced energy, not speaker identity.
The promoted distant-background fixtures overlap ordinary noise and plausible
quiet speech in simple energy metrics. One duplicated-sentence fixture is now
covered, but the non-repetitive `Aye, you did that.` fixture still needs a
validated speaker-distance, confidence, or equivalent signal. No unsupported
RMS threshold should be added without a canary comparison against close-mic
speech and ambient/noise fixtures.

Issue 083 also retains open work for emergency stop, active injector/FIFO
lifecycle canaries, failed-injection policy, and broader generation fencing.

## Verification baseline

The safety changes were verified with focused race tests, `go test -count=1
./...`, `make check`, `make restart-service`, and `git diff --check`. The daemon
was restarted after live-path changes. Fixture-based background-voice tests
remain gated by the external ASR/corpus environment and must never invoke real
desktop injection.
