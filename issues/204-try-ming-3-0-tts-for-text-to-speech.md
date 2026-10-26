# 204 — Try Ming-3.0-tts for text-to-speech

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Feature
**Related**: [lmcoder #230 — Ming-3.0-tts runtime trial](../../lmcoder/issues/230-try-ming-3-0-tts-for-text-to-speech.md)

---

## 1. Problem & Motivation
Try Ming-3.0-tts as a TTS candidate for Voxi and assess whether it improves
the read-aloud experience, coordinating runtime findings with lmcoder #230.

## 2. Technical Specification / Findings
Confirm the exact upstream model, license, supported languages, voice options,
runtime or API requirements, and compatibility with Voxi's TTS interface.
These details are currently unverified.

## 3. Implementation & Verification Plan
Check live project/code status and recent commits before starting. Synthesize
and listen to representative short and long text samples, comparing with the
current TTS baseline. Record intelligibility, pronunciation, naturalness,
latency, real-time factor, resource use, and any playback/integration limits.
Keep the setup reproducible and link results in lmcoder #230 with an adoption
recommendation or concrete blockers.

/goal Complete a reproducible Ming-3.0-tts trial for Voxi and record an adoption
verdict, with results linked in both tickets, or stop and report when blocked
on a user decision or denied permission. On a loop, hard architectural or OS
blocker, or no progress, clear the goal and write a failure report explaining
why the evaluation failed.
