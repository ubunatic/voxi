# 207 — Raw microphone capture interpreted AU audio as PCM

**Status**: Closed — Fixed pw-record raw PCM capture in both eager modes; tests and live dictation verified
**Priority**: P1 (High)
**Severity**: Major
**Category**: Bug
**Related**: [206](206-doctor-misses-offline-remote-transcription-and-fallback-failure.md)

---

## Problem

Live dictation recorded noise and produced empty transcripts across microphones and ASR models. `pw-record` emitted an AU stream (`.snd` header and big-endian audio), but Voxi interpreted it as raw little-endian PCM. Doctor reported the microphone and ASR engine healthy despite the unusable recording.

## Goal

/goal Restore intelligible microphone capture in both eager modes and verify live dictation and automated tests; stop and report if blocked on a user decision or denied permission. On a loop, hard architectural or OS blocker, or no progress, clear the goal and write a failure report explaining why. Recheck live code and recent history before any future work.

## Fix and verification

Both eager capture paths use shared `pwRecordArgs` with `--raw`, requesting 16 kHz mono s16 PCM. A regression test checks the arguments. A live `pw-record --raw` probe produced 3,200 bytes without an AU header; `make test-q1` passed, `make restart-service` installed and restarted the agent, and the user confirmed dictation works. Issue 206 remains open for the separate offline-server and doctor fallback concerns.
