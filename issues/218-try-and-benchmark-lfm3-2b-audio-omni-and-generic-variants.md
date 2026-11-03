# 218 — Try and benchmark LFM3 2B (Audio, Omni and Generic variants)

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Research
**Related**: 200 (voxi-model evaluation skill), 066 (canary of alternative ASR models), 132 (Qwen3.8 live STT evaluation), 199 (tested models catalog), 216 and 217 (other "try and bench" TTS tickets)

---

/goal Determine whether the 2B LFM3 variants (Audio, Omni and Generic) are usable for Voxi speech
input or output: canary-probe each variant, run the standard ASR or TTS bench where it applies, and
record the verdict in `docs/TestedModels.md`, or stop and report when blocked on a user decision or
denied permission.

## 1. Problem & Motivation
A user asked to try and benchmark "LFM3", a 2B-parameter model family with three named variants:
Audio, Omni and Generic. It is not in the tested-models catalog (issue 199), and no ticket or doc in
the repo mentions LFM. The variants probably differ in what they accept and produce (speech in, speech
out, or general text), so each one needs a separate verdict. The user's wording does not say which
variants are speech-capable, so the canary decides that.

## 2. Technical Specification / Findings
Not yet verified. To be established by the canary:
- Exact model names, sources, license, and weight format (GGUF, ONNX or other).
- Which variant does speech recognition, which does speech synthesis, and which does neither.
- A native runtime that avoids a Python daemon (issue 200 constraints).
- Hardware profiles from issue 200: ThinkPad T14 Gen2 (Ryzen 5650U, Vulkan RADV) and x600 (Ryzen 8700G).

Targets from issue 200: ASR eager chunk real-time factor (RTF) below 0.5 on CPU and below 0.15 on GPU;
TTS first-chunk audio latency below 300 ms.

## 3. Implementation & Verification Plan
1. Canary first (issue 200 / docs `Canary.md`): for each variant, find the weights and a runtime, confirm
   the license, and classify its modality. Drop any variant with no speech capability from the bench, and
   say so in the ticket.
2. For each speech-capable variant, run the bench that matches it: ASR on the dictation samples, or TTS
   on the reference text. Record RTF, first-chunk latency, peak memory and quality on both hardware profiles.
3. Record the verdict per variant (adopt, park or reject, with numbers) in `docs/TestedModels.md`. Adopt
   only if the spec change, install step and docs follow in separate tickets.
