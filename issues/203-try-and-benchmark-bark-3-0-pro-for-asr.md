# 203 — Try and benchmark Bark 3.0 Pro for ASR

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Feature
**Related**: [lmcoder #227 — ASR serving trial and benchmark](../../lmcoder/issues/227-try-and-benchmark-bark-3-0-pro-for-asr.md)

---

## 1. Problem & Motivation
Try and benchmark Bark 3.0 Pro for Voxi dictation, coordinating with the linked
lmcoder ticket for model serving and shared benchmark results.

## 2. Technical Specification / Findings
The exact upstream model identity and ASR support are unverified. Confirm the
model, license, local runtime or API requirements, and supported languages;
record any naming or capability mismatch before attempting integration.

## 3. Implementation & Verification Plan
Check live project/code status and recent commits before starting. Run a small
transcription canary, then compare with Voxi's current ASR baseline using the
same representative dictation and noise samples. Record WER/CER, latency,
real-time factor, cold/warm behavior, memory use, and no-speech/hallucination
behavior, with reproducible model/runtime versions and configuration. Link the
results in lmcoder #227 and recommend whether to adopt it for Voxi.

/goal Complete a reproducible Bark 3.0 Pro ASR trial and benchmark for Voxi,
record an adoption verdict, and link results in both tickets, or stop and report
when blocked on a user decision or denied permission. On a loop, hard
architectural or OS blocker, or no progress, clear the goal and write a failure
report explaining why the evaluation failed.
