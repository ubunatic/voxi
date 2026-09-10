# 080 — Surface Eager Typing/Transcription Failures Beyond Private Telemetry

**Status**: Closed
**Priority**: P0 (Critical)
**Severity**: Critical
**Category**: Bug
**Related**: [077 voxtype-optional Cohere path](077-make-voxtype-optional-and-remove-whisper-only-engine-assumptions.md), [078 install-crispasr target](078-add-make-install-crispasr-target-for-the-crispasr-binary.md)

---

## 1. Problem & Motivation

Typing/keystroke-injection failures in the eager dictation pipeline are
completely silent at the console/log level. They are only ever recorded to a
private telemetry JSONL file (`voxi telemetry query`) that nobody looks at
during normal use. For a dictation tool, this is close to the worst-case
failure mode: speech is transcribed correctly, but nothing is typed, and
there is no visible clue anywhere a normal user would look.

### Concrete bug, confirmed live this session

- This dev machine had no `dotool` binary installed at all (fixed separately
  in commit `4372dd0`, unrelated to this ticket — do not re-litigate that
  fix; it is not part of this bug).
- With `dotool` missing, a full live dictation session ran: speech was
  correctly transcribed via the Cohere pipeline (confirmed via `voxi chunks
  list` showing correct transcripts like "Testing one, two, three."), but
  nothing was typed into the focused window, and there was no visible error
  anywhere a normal user would look: not in `journalctl --user -u
  voxi-agent.service` (only showed "Recording started"/"Recording stopped",
  nothing about typing), not on the console, nothing.
- The only place the failure was recorded was the private telemetry event
  log, found via `voxi telemetry query --view events --chunk <id> --format
  json`, which showed:
  ```json
  {
    "event": "typing_completed",
    "success": false,
    "error": "dotool not found on PATH: executable file not found in $PATH"
  }
  ```

### Root cause in code

`internal/eager/eager.go` lines 542–556 (verified current at time of
filing; re-check by grepping for `TypingComplete` / `typing.TypeText` since
the file is under active development):

```go
if opts.TypeOutput {
    typeStart := time.Now()
    _ = recorder.Record(telemetry.Event{Event: telemetry.TypingStarted, Timestamp: typeStart, SessionID: sessionID, ChunkID: chunkID, ChunkIndex: job.Index})
    typeErr := typing.TypeText(context.Background(), d, text+" ")
    typeEnd := time.Now()
    typeSuccess := typeErr == nil
    typeError := ""
    if typeErr != nil {
        typeError = typeErr.Error()
    }
    _ = recorder.Record(telemetry.Event{Event: telemetry.TypingComplete, Timestamp: typeEnd, SessionID: sessionID, ChunkID: chunkID, ChunkIndex: job.Index, Success: &typeSuccess, Error: typeError})
    chunkMeta.TypingStartedAt = typeStart
    chunkMeta.TypingEndedAt = typeEnd
    _, _ = chunkBuf.Update(chunkMeta)
}
```

`typeErr` is captured and recorded to telemetry but never logged, printed,
or surfaced anywhere else. A user (or the daemon's own stdout/journal) has
zero visibility into "voxi transcribed your speech but failed to type it."

### No existing daemon-mode logging convention to reuse

Investigated during triage: `internal/eager` has no `log`/`slog` usage
anywhere (`grep -rn '"log"\|log\.Print\|slog\.' internal/eager/*.go` — no
hits). The only existing error-surfacing pattern in `eager.go` is
`if !isDaemon { fmt.Fprintf(d.Stdout, ...) }` (e.g. line 429–430 for
utterance-audio write errors), which is stdout-only and gated off entirely
in daemon mode — it would not reach `journalctl` either. There is currently
no pattern in this file for getting an error into the systemd journal when
running as `voxi-agent.service`. Whoever implements the fix needs to
establish this convention (or find one already used by `cmd/voxi`'s daemon
entry point) rather than copy a pattern that doesn't actually solve the
daemon-mode half of this bug.

### Same gap also present in transcription failures

Grepping all `recorder.Record(telemetry.Event{Event: telemetry....` call
sites in `eager.go` shows the identical pattern for transcription:
`TranscriptionComplete` (line 491) captures `Success`/`Error` from the ASR
subprocess (`err := cmd.Run()` at line 476) into telemetry only. The nearby
stdout print at lines 537–540 is gated on `accepted` (a rejection-heuristic
result, not `err == nil`) and `!isDaemon`, so a hard transcription
subprocess failure can likewise go unreported to both console and journal.
This is noted here as additional context/an open question for scoping the
fix — it is not certain every such case is user-visible-worthy (e.g. some
rejections are expected silence/noise filtering), and the fix should not
scope-creep beyond what's actually broken. `MicActivated`/`MicDeactivated`/
`CaptureStarted`/`CaptureStopped`/`ChunkFinalized` events were also grepped
and appear to be state markers rather than error-carrying events, so they
are likely not part of this gap — worth a quick sanity check at
implementation time.

## 2. Scope for the Fix (Open Questions, Not Decided Here)

- At minimum, typing failures should be logged somewhere a user actually
  sees during normal operation: stdout in non-daemon/direct mode (there's
  already a non-daemon stdout print for successful transcription at lines
  537–540 — same treatment should probably apply to typing errors), and to
  the systemd journal in daemon mode (`voxi-agent.service`, via `journalctl
  --user -u voxi-agent.service`) since that's the actual running mode most
  users hit day to day.
- Check whether `cmd/voxi`'s daemon/agent entry point (outside
  `internal/eager`) already has a `log`/`slog`-to-journal convention that
  `eager.go` should adopt for consistency, rather than inventing a new one.
- Decide whether the transcription-failure gap found above (§1, "Same gap
  also present in transcription failures") is in scope for this ticket or
  should be split into a follow-up — do not silently leave it unaddressed
  without a documented decision either way.
- Non-goal: do not touch `dotool` installation itself (already fixed,
  unrelated commit `4372dd0`).

## 3. Acceptance Criteria

- A typing failure (e.g. `dotool` missing or erroring) is visible to the
  user during normal operation: printed to stdout in direct/non-daemon mode,
  and visible via `journalctl --user -u voxi-agent.service` in daemon mode.
- The fix follows an established logging convention (found via investigation
  above) rather than inventing an ad hoc one, or explicitly documents why a
  new convention was introduced if none existed.
- The transcription-failure gap identified in §1 is either fixed alongside
  typing, or explicitly deferred to a follow-up ticket with a stated reason.
- Telemetry recording of these events is unchanged — this ticket adds
  live surfacing, it does not replace or alter telemetry.
- Tests cover the new surfacing path (e.g. a fake `typing.TypeText` failure
  produces the expected stdout/log output), not just that telemetry is
  recorded.

## 4. Verification

1. Run focused tests for `internal/eager` covering the new logging path.
2. Run `go test ./...` and `make check`.
3. Exercise a live typing failure (e.g. temporarily rename `dotool` off
   `$PATH`) in both direct (`voxi eager`) and daemon (`voxi-agent.service`)
   modes, and confirm the error is visible via stdout and
   `journalctl --user -u voxi-agent.service` respectively.
4. Because this touches `internal/eager`, which the live
   `voxi-agent.service` executes, run `make restart-service` (not only
   `make install`) before daemon-mode verification, per this repo's
   CLAUDE.md.

## 5. Non-Goals

- Installing or fixing `dotool` itself — already resolved in commit
  `4372dd0`, unrelated to this ticket.
- Redesigning the telemetry system or its schema.
- Any change to `internal/eager/cohere.go`'s Cohere weight-download logic or
  the `install-crispasr` Makefile target (issue 078, closed, unrelated).

## 6. Resolution (2026-09-11)

Closed by emitting hard audio-write, transcription, and typing failures through
the eager output stream, including modifier-buffer flush failures. Direct
invocations show the diagnostic on stdout, and the daemon's inherited stdout is
captured by `voxi-agent.service` in the user journal. Expected empty,
silence-artifact, stop-word, and pathological transcript rejections remain quiet.
Telemetry recording is unchanged, and focused tests cover the diagnostic
boundary without including transcript text.
