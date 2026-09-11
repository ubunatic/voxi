# Eager Safety Sprint Retrospective

**Date:** 2026-09-11  
**Scope:** Roadmap Now items 080, 083, 092, 100, and 103

## Outcome

The session landed sequential checkpoints for durable delivery identities,
visible hard failures, serialized toggles, queued-after-stop draining, and
duplicated-sentence rejection. Issues 080, 092, and 103 are closed. Issue 083
remains In Progress for emergency-stop, active injector/FIFO lifecycle, and
complete generation semantics. Issue 100 remains In Progress because its
non-repetitive background-voice false positive lacks a validated classifier;
the safe duplicated-sentence sub-case is covered.

## Architecture learnings

- At-most-once delivery needs a persistent identity ledger, not only an
  in-memory de-duplication map.
- Stop behavior must distinguish queued work from work already executing. A
  blanket canceled-context rule silently loses legitimate trailing speech.
- A dedicated control mutex is appropriate for `Toggle`'s check-and-act
  sequence, while the state mutex must remain narrow around lifecycle state.
- Daemon diagnostics can use inherited eager output because systemd captures the
  service stream; expected transcript rejection should not be presented as an
  operational failure.
- Voiced energy is not speaker identity. A safety fix must be canary-backed
  against quiet close-mic speech before changing acoustic thresholds.

## Process effectiveness

Frequent commits made recovery and review straightforward: each ticket-sized
milestone was independently buildable and the reviewer could inspect a compact
range. Reusing one advisor across sequential ticket audits preserved useful
context and avoided parallel workspace edits. The independent review found a
real buffered-error omission and a late context decision, validating the review
gate.

## Friction and failures

Several status-only `apply_patch` attempts used inaccurate surrounding context,
and one search used an unsupported `rg -E` flag. A reviewer also exposed tracker
drift between issue headers and the roadmap/index. One reused advisor response
returned stale context for the wrong ticket and had to be corrected explicitly.

## What to do differently

Begin with a generated status cross-check before making closure claims, use
append-only ticket notes when exact context is uncertain, and send a short
scope marker with every reused-advisor request. For safety work, require an
integration test at the irreversible injector boundary before closing a ticket;
helper-only context tests are useful but insufficient.

## Harness improvement proposals (not applied)

- Add a sequential-advisor mode that rejects a new request until the previous
  advisor has acknowledged the ticket identifier and scope.
- Add a tracker-sync check that compares every issue `Status` header with the
  corresponding `issues/README.md` and `docs/Roadmap.md` row before commit.
- Add a native helper for resuming a Codex advisor by provider session ID while
  clearly displaying its project and current ticket scope.
- Make malformed patch feedback include the nearby unmatched file context to
  reduce repeated retries.

## Issue 083 stop-drain follow-up

The daily-driver reproduction clarified that the final Super-X is a flush
command: stop capture, drain all audio captured before the request, transcribe
it, and type it promptly. The implementation now records the stop request
timestamp before cancellation, preserves pre-stop reads even when cancellation
is observed between read and processing, and drains in-flight, queued, and
segmenter-flushed jobs under a bounded asynchronous lease. Delivery is fenced
by lease eligibility before and after the durable at-most-once claim.

The independent review caught several subtle concurrency defects before the
commit, including a late stop timestamp, a post-read cancellation drop, and a
modifier-buffer claim before eligibility. The remaining known limitation is
the crash window after a durable claim but before injector submission; this
keeps the safety guarantee at-most-once at the cost of possible loss and
remains part of issue 083's open work.
