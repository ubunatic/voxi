# 205 — Try dessa TTS model

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Feature
**Related**: [lmcoder #237 — dessa runtime trial](../../lmcoder/issues/237-try-dessa-tts-model.md)

---

## 1. Problem & Motivation
Try the model named "dessa" as a TTS candidate for Voxi's read-aloud workflow,
coordinating runtime findings with lmcoder #237.

## 2. Technical Specification / Findings
The exact upstream model identity is unverified. Confirm the spelling/release,
license, supported languages and voice options, runtime or API requirements,
and compatibility with Voxi's TTS interface before implementation.

## 3. Implementation & Verification Plan
Check live project/code status and recent commits before starting. Synthesize
and listen to representative short and long text samples, comparing with the
current TTS baseline. Record intelligibility, pronunciation, naturalness,
latency, real-time factor, resource use, and playback/integration limits.
Keep the setup reproducible and link results in lmcoder #237 with an adoption
recommendation or concrete blockers.

/goal Complete a reproducible dessa TTS trial for Voxi and record an adoption
verdict, with results linked in both tickets, or stop and report when blocked
on a user decision or denied permission. On a loop, hard architectural or OS
blocker, or no progress, clear the goal and write a failure report explaining
why the evaluation failed.
