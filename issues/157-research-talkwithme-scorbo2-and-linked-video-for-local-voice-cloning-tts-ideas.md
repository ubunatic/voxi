# 157 — research TalkWithMe (scorbo2) and linked video for local voice cloning / TTS ideas

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Research
**Related**: issue 154 (Pocket TTS canary), issue 155, `docs/studies/2026-09-27-local-voice-cloning-research.md`

---

## 1. Problem & Motivation
The user pointed to two sources to review:
- https://github.com/scorbo2/TalkWithMe (Python, MIT, "Configurable AI chat with TTS/STT", pushed 2026-09-26)
- https://www.youtube.com/watch?v=P0N91YLX04A (content not yet reviewed)

Context: issue 154 showed Pocket TTS via sherpa-onnx clones the user's voice but drops words on longer
texts (int8 and fp32). We look for local, non-cloud tooling where a few WAVs are enough, ideally without
managing PyTorch ourselves.

## 2. Technical Specification / Findings
Unknown yet: which TTS/STT engines TalkWithMe uses, whether it does voice cloning, how it runs locally,
and what the video shows. Record findings here; do not assume.

## 3. Implementation & Verification Plan
/goal Summarize both sources in this ticket (engines, cloning support, local runtime, license, anything
reusable for voxi or better than Pocket TTS), with a keep/skip recommendation, or stop and report when
blocked on a user decision or denied permission (e.g. video not accessible without a transcript).
