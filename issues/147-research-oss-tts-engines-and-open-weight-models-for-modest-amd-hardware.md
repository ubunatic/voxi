# 147 — Research OSS TTS Engines and Open-Weight Models for Modest AMD Hardware

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Architecture
**Related**: [141](141-discovery-add-a-tts-engine-so-other-tools-can-have-voxi-read-text-aloud.md)

---

## 1. Problem & Motivation

Voxi needs a path to more natural and expressive reading voices, including optional playful character-style delivery such as a Yoda-like voice. The available open-source engines and open-weight speech models change quickly, and their hardware demands may exceed what modest local AMD systems can handle.

## 2. Technical Specification / Findings

Research current open-source TTS engines and open-weight models that can run locally on AMD Cezanne-class hardware or newer, with Phoenix-class hardware as the minimum fallback target. Compare voice quality and expressiveness, support for multiple or stylized voices, AMD/Linux runtime support, memory and compute needs, short-snippet latency, licensing, and practical installation requirements. Record uncertainties and distinguish verified results from vendor claims.

### Initial Advisor Survey (2026-09-25)

A `luna:med` advisor compared direct text-to-speech with conversion of existing Festival audio. This is a candidate shortlist, not a verified benchmark; check current model cards, checkpoints, and voice asset terms before selecting an implementation.

| Candidate | Approach | Initial assessment |
|---|---|---|
| [Kokoro](https://github.com/hexgrad/kokoro) | Text to speech | Compare first for naturalness versus model/runtime size. CPU speed and quality on Cezanne/Phoenix remain to be measured. |
| [Piper](https://github.com/OHF-Voice/piper1-gpl) | Text to speech | Use as a lightweight CPU baseline. Check the engine and selected voice licenses separately. |
| [XTTS-v2](https://huggingface.co/coqui/XTTS-v2) | Reference-conditioned text to speech / voice cloning | A heavier comparison if reference-voice matching matters; review its model license separately from toolkit code. |
| [OpenVoice V2](https://github.com/myshell-ai/OpenVoice) | Voice/timbre conversion, usually paired with a base TTS | Candidate for converting Festival WAVs or a synthesized base voice. Test whether the desired input path works and measure both quality and CPU cost. |
| [RVC](https://github.com/RVC-Project/Retrieval-based-Voice-Conversion-WebUI) | Audio voice conversion | Another route for Festival WAVs when a suitable target-voice model is available. Its common GPU-oriented workflows do not establish good CPU performance on these APUs. |
| [CosyVoice](https://github.com/FunAudioLLM/CosyVoice) | Text-conditioned speech generation with reference/streaming options | Stretch candidate if smaller systems miss the quality target; measure resource use locally and verify the exact checkpoint terms. |

Direct TTS can regenerate speech with different prosody. Audio voice conversion changes the speaker character of existing speech and may retain more of its timing, but can add artifacts. Keep the two paths' quality results separate. For the first prototype, compare Kokoro and Piper from text, then try OpenVoice V2 on Festival WAVs; add RVC if an authorized target-voice model is available. Measure startup/load time, warm synthesis time or real-time factor, peak memory, intelligibility, naturalness, pronunciation, voice similarity, and artifacts on a fixed set of short utterances.

Treat Cezanne and Phoenix as CPU-first targets. The [AMD ROCm APU compatibility matrix](https://rocm.docs.amd.com/projects/radeon-ryzen/en/latest/docs/compatibility/compatibilityryz/native_linux/native_linux_compatibility.html) does not establish supported acceleration for these generations; verify the exact APU, OS, and framework before relying on its iGPU. The advisor supplied candidate project links, but did not establish comparative benchmark results on target hardware.

## 3. Implementation & Verification Plan

**/goal**: Produce a concise, evidence-backed comparison and recommendation for a locally runnable TTS option that provides natural voices on the target hardware, and determine whether playful/Yoda-like delivery is feasible through supported voices or a separate technique. Define the target hardware and a representative short-text latency/quality evaluation, then verify the leading candidate on available hardware or document a reproducible canary plan if that hardware is unavailable.
