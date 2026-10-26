# 204 — Try Ming-3.0-tts, Fish Audio Drama 3 Preview, and Breeze TTS 2

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Feature
**Related**: [lmcoder #230 — TTS runtime trials](../../lmcoder/issues/230-try-ming-3-0-tts-for-text-to-speech.md)

---

## 1. Problem & Motivation
Try Ming-3.0-tts, Fish Audio Drama 3 Preview, and Breeze TTS 2 as TTS candidates
for Voxi. Assess whether they improve the read-aloud experience, coordinating
runtime findings with lmcoder #230.

## 2. Technical Specification / Findings
For each candidate, confirm the exact upstream model, license, supported languages, voice options,
runtime or API requirements, and compatibility with Voxi's TTS interface.
These details are currently unverified.

## 3. Implementation & Verification Plan
Check live project/code status and recent commits before starting. Synthesize
and listen to the same representative short and long text samples for each
candidate, comparing with the current TTS baseline and one another.
Record intelligibility, pronunciation, naturalness,
latency, real-time factor, resource use, and any playback/integration limits.
Keep the setup reproducible and link results in lmcoder #230 with an adoption
recommendation or concrete blockers for each candidate.

/goal Complete reproducible trials of Ming-3.0-tts, Fish Audio Drama 3 Preview,
and Breeze TTS 2 for Voxi and record an adoption verdict for each, with results
linked in both tickets, or stop and report when blocked
on a user decision or denied permission. On a loop, hard architectural or OS
blocker, or no progress, clear the goal and write a failure report explaining
why the evaluation failed.
