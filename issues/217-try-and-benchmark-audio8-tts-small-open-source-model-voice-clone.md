# 217 — Try and benchmark Audio8 TTS (small open-source model, voice clone)

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Research
**Related**: 216 (Paradee-8M TTS, same evaluation pattern), 200 (voxi-model evaluation skill), 160 (VoxCPM GGUF adoption), 187 (KittenTTS 2 voice cloning), 199 (tested models catalog)

---

/goal Determine whether Audio8 TTS is a usable Voxi TTS candidate, including voice cloning: canary-probe
it, run the standard TTS bench, and record the verdict in `docs/TestedModels.md`, or stop and report when
blocked on a user decision or denied permission.

## 1. Problem & Motivation
A user asked to try and benchmark Audio8 TTS: a small open-source model, about 0.6B parameters, covering
23 languages, with voice cloning, under the Apache License. It is not in the tested-models catalog
(issue 199). Voice cloning and multilingual output are the two properties that matter for Voxi, so both
get measured. The Apache license, if confirmed, would make it easier to adopt than non-commercial models.

## 2. Technical Specification / Findings
From the request, not yet verified: about 0.6B parameters, 23 languages, voice cloning, Apache License.
To be established by the canary:
- Source, weight format (GGUF, ONNX or other), and a native runtime that avoids a Python daemon.
- Which of the 23 languages work in practice; German is required (the user's language).
- Clone quality from a short reference sample, compared with the current cloned-voice engine (issue 160).
- Hardware profiles from issue 200: ThinkPad T14 Gen2 (Ryzen 5650U, Vulkan RADV) and x600 (Ryzen 8700G).

Targets from issue 200: TTS first-chunk audio latency < 300 ms; real-time factor (RTF) and peak memory
recorded per hardware profile.

## 3. Implementation & Verification Plan
1. Canary first (issue 200 / docs `Canary.md`): find weights and a runtime, confirm the license, and
   synthesize one English, one German and one clone sample. Stop here if the license or runtime blocks it.
2. Bench on both hardware profiles: first-chunk latency, RTF, peak memory, dropped words, and a listening
   rating for the clone against the current cloned-voice engine.
3. Record the verdict (adopt, park or reject, with numbers) in `docs/TestedModels.md`. Adopt only if the
   spec change, install step and docs follow in separate tickets.
