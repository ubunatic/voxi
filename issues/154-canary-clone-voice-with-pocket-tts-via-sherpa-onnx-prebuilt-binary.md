# 154 — canary: clone voice with Pocket TTS via sherpa-onnx prebuilt binary

**Status**: Open
**Priority**: P1 (High)
**Severity**: Feature
**Category**: Canary
**Related**: `docs/studies/2026-09-27-local-voice-cloning-research.md`, issue 155, issue 153, `docs/Canary.md`

---

## 1. Problem & Motivation
The user wants to hand in a few WAVs and hear their own voice, without us managing a PyTorch stack.
Research ranked Kyutai Pocket TTS (≈100M params, English, reference-WAV cloning, CPU, ~200 ms first
audio) run through sherpa-onnx (Apache-2.0, native C/C++, prebuilt Linux binaries, Go binding) first.
Before building on it, probe it end to end (docs/Canary.md).

## 2. Technical Specification / Findings
- No PyTorch, no pip training environment. Use a prebuilt sherpa-onnx Linux release binary and the
  Pocket TTS ONNX model files that sherpa-onnx documents.
- Reference audio: the allowlisted samples in `~/.config/voxi/samples/voice-training.txt`
  (see issue 153); combine or pick the best clip.
- Verify and record the Pocket TTS **weights license** from the actual model card.
- Keep downloads under `~/.cache/voxi/` and scripts under `scripts/canary_voiceclone/`; nothing private is committed.

## 3. Implementation & Verification Plan
- Script `scripts/canary_voiceclone/run.sh`: download binary + model (pinned versions, checksums),
  synthesize 3 demo sentences with the user's reference WAV into WAV files.
- Record here: exact versions, license, CPU real-time factor and first-audio time on this machine,
  and whether a streaming or server mode exists in the binary.
- Done when the demo WAVs exist and play, and findings are written in this ticket.
