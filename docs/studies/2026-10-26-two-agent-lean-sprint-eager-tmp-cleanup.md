---
kind: sprint
date: 2026-10-26
issue: 202
---

# Two-Agent Lean Sprint: Eager Daemon Survives /tmp Cleanup (Issue 202)

## Incident

A manual `/tmp` cleanup (the disk was full) deleted the eager daemon's temp dir, which is created
once at startup. After that, every chunk failed with `open .../utt_NNN.wav: no such file or directory`,
while `voxi monitor` still looked healthy. A service restart fixed it.

## Setup

- Searcher skipped: the ticket named the exact file and function (`internal/eager/service.go:86`).
  Replaced by a plan-only first developer turn, which confirmed the premise and the write set.
- Developer: `luna:high` (session dev202), 3 turns: plan (3 min), M1 (4 min), M2 (9.5 min).

## Milestones

- **M1 (temp-dir recovery)**, f5fb21c: `writeEagerChunkWAV` runs `MkdirAll` before each WAV write, and the base
  dir is under `$XDG_RUNTIME_DIR`.
- **M2 (monitor degraded state)**, c3595c0: the daemon publishes `voxi/eager-audio-failures.json`, and the monitor
  shows `⚠ audio degraded (N consecutive chunk write failures)` from 2 failures until the next success.
  The daemon removes its temp dir on SIGTERM (host review found a leftover dir after a restart).

## Verification

- The host ran the full `make test-q1`: green, no `--- FAIL`.
- Live: deleted the running daemon's temp dir, and the next dictation was typed with no errors.
- Live: injected a failure state file, and the owner saw the degraded line in `voxi monitor`.

## Lessons

- M2 was committed after a test run that stopped at `go vet` before any tests ran; the host's full run
  closed the gap. The skill rule held.
- The owner rejected one chained host command (restart, write, monitor scrape, `rm` in one line).
  Splitting it into separately approved steps, with the owner reading the TUI, worked better.
