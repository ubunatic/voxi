# 216 — Try and benchmark Paradee-8M-v1.1 TTS (small open-source model)

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Research
**Related**: 147 (OSS TTS research), 160 (VoxCPM GGUF adoption), 161 (VoxCPM 2), 187 (KittenTTS 2), 199 (tested models catalog), 200 (voxi-model evaluation skill)

---

/goal Determine whether Paradee-8M-v1.1 is a usable Voxi TTS candidate: canary-probe it, run the
standard TTS bench, and record the verdict in `docs/TestedModels.md`, or stop and report when blocked
on a user decision or denied permission.

## 1. Problem & Motivation
A user asked to try and benchmark Paradee-8M-v1.1, a small open-source TTS model (about 8M parameters,
per the name). It is not in the tested-models catalog (issue 199) and no file in the repo mentions it.
A model this small could fit the no-PyTorch, low-latency desktop constraints, but nothing is known yet
about its license, weight format, runtime, or voice quality.

## 2. Technical Specification / Findings
Unverified so far; to be established by the canary:
- Source, license, weight format (GGUF, ONNX or other), and a native runtime that avoids a Python daemon.
- Languages supported, and whether it can use the voice-clone path (issue 160 style).
- Hardware profiles from issue 200: ThinkPad T14 Gen2 (Ryzen 5650U, Vulkan RADV) and x600 (Ryzen 8700G).

Targets from issue 200: TTS first-chunk audio latency < 300 ms; real-time factor (RTF) and peak memory
recorded per hardware profile.

## 3. Implementation & Verification Plan
1. Canary first (issue 200 / docs `Canary.md`): find weights and a runtime, check the license, and
   synthesize one reference and one German sentence. Stop here if the license or runtime blocks it.
2. Bench on both hardware profiles with the existing TTS bench: first-chunk latency, RTF, peak memory,
   dropped words, and a listening rating against the current default engine.
3. Record the verdict (adopt, park or reject, with numbers) in `docs/TestedModels.md`. Adopt only if the
   spec change, install step and docs follow in separate tickets.
