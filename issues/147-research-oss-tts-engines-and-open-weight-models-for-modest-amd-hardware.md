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

### Local CPU canary (2026-09-25)

Host: AMD Ryzen 5 PRO 5650U (12 logical CPUs), 23.3 GiB RAM. No GPU provider
was used. Piper ran without `--cuda`; Kokoro explicitly used
`CPUExecutionProvider` with four ONNX intra-op threads and one inter-op thread.
The fixed utterance was “Hello. This is a short local speech synthesis
benchmark.” Results are cold process runs, including interpreter/model load;
RTF is wall latency divided by generated WAV duration. These are local
latency/smoke results, not a voice-quality evaluation.

| Engine | Install / assets | Latency | WAV duration | RTF | CPU time | Peak RSS |
|---|---|---:|---:|---:|---:|---:|
| Festival (`text2wave`) | System package already installed | 2.403 s | 3.91 s | 0.615 | 2.38 s | 374.0 MiB |
| espeak-ng | System package already installed | 0.013 s | 3.59 s | 0.004 | <0.01 s | 8.3 MiB |
| Piper 1.8.0, `en_US-lessac-medium` | Python venv install; 60.3 MiB ONNX + config | 1.877 s | 3.30 s | 0.569 | 8.36 s | 189.6 MiB |
| Kokoro ONNX (`kokoro-onnx` 0.4.7) | Python venv install; 310.4 MiB model + 26.9 MiB voices | 3.928 s | 3.24 s | 1.211 | 10.41 s | 578.0 MiB |

Measurements varied slightly between runs. The Kokoro run required a local
canary compatibility shim: the installed package emits int32 `speed` for the
`input_ids` graph, while the downloaded graph expects float32. With that cast,
it generated a valid WAV. Treat this as an upstream package/artifact
compatibility finding to resolve or pin before integration. Neural cold-start
latency trails espeak-ng substantially here; test a resident process and warm
requests before deciding whether Kokoro is responsive enough for streaming.

The persistent canary is `scripts/canary_tts/benchmark.py` with its small
Kokoro adapter. It reports latency, duration, RTF, CPU, and peak RSS; set
`VOXI_TTS_PYTHON`, `VOXI_PIPER_MODEL`, `VOXI_KOKORO_MODEL`, and
`VOXI_KOKORO_VOICES` to supply optional candidates. It does not fetch assets.
Candidate license details are separate from package license: the Kokoro-82M
model card declares Apache-2.0; Piper says to check each voice's model card,
and `en_US-lessac-medium` has MIT license metadata.

### Integration architecture

`internal/tts` already has an `EngineBackend` queue boundary, but production
`Engine.Synthesize` selects Festival/espeak-ng directly and starts a new
supervised command for each chunk. Extend this seam with named synthesizer
providers selected by `spec/tts.yaml` (retain Festival then espeak-ng as the
default fallback chain). Give each provider a prepare/synthesize contract and
an explicit model/voice path; neural providers should own one bounded,
supervised resident worker so model load is paid once, while WAV playback,
silence trimming, queueing, and interruption remain shared. Startup status
must explain unavailable provider/model and fallback. Keep downloads and Python
package installation out of `make install`; document opt-in setup and each
model/voice license. Add a style/preset field only after deciding whether it
changes narration text, synthesis prosody, or both. Yoda-like cadence is not
established by this benchmark.

Acceptance work for this issue: choose Piper or Kokoro after warm latency and
listening comparisons on target hardware; verify fallback, worker
shutdown/interruption, memory bounds, and voice/model license; compare natural
and deliberately stylized narration for factual preservation and audible
cadence.

### Initial implementation (2026-09-25)

`internal/tts` now accepts `VOXI_TTS_BACKEND=piper` and uses an installed
`piper` executable with an explicitly supplied `VOXI_PIPER_MODEL` path. An
optional `VOXI_PIPER_CONFIG` passes the voice JSON config. With backend `auto`,
Piper is selected when the model path is set and both the executable and model
are available; otherwise the established Festival then espeak-ng chain remains
active. `BackendStatus` reports the chosen provider. Neural package/model
downloads remain manual and outside `make install`. Piper's executable and
selected voice asset have separate license obligations; check both before use.

This first integration runs the Piper CLI per chunk and therefore pays model
startup on each chunk. It does not meet the resident-worker latency target and
Piper synthesis itself was not available for verification on this host. Keep
issue 147 open until warm multi-chunk latency, memory, listening quality,
interruption, and license checks are completed on target hardware. Current
canary script: `python3 scripts/canary_tts/benchmark.py`; supply model paths via
the documented environment variables above to measure neural candidates.
