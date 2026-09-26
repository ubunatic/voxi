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

## 4. Canary Results (2026-09-27)

- **Pinned runtime:** sherpa-onnx `v1.13.8`, Linux x86-64 static release archive
  `sherpa-onnx-v1.13.8-linux-x64-static.tar.bz2`, SHA-256
  `d265986f7b990026e45bca573415575140106a2c663014924037d8f2c2e297cc`.
- **Pinned model:** sherpa-onnx Pocket TTS int8 archive
  `sherpa-onnx-pocket-tts-int8-2026-01-26.tar.bz2`, SHA-256
  `2f3b88823cbbb9bf0b2477ec8ae7b3fec417b3a87b6bb5f256dba66f2ad967cb`.
  These are downloaded to `~/.cache/voxi/` by `scripts/canary_voiceclone/run.sh`.
- **Weights license:** Kyutai's [Pocket TTS model card](https://huggingface.co/kyutai/pocket-tts)
  declares CC-BY-4.0. The sherpa ONNX archive bundles the CC-BY-4.0 license text, while its
  README says to consult the exporter license and describes it as non-commercial. The actual
  bundled license permits this local personal demo; retain attribution and treat commercial
  use of the converted archive as restricted pending clarification.
- **Reference:** the longest allowlisted clip, `llama-lo-route.wav` (8 seconds, mono, 16 kHz).
  It remains private and was not copied into the repository.
- **Machine:** AMD Ryzen 5 PRO 5650U, 6 cores / 12 threads. CPU provider, 6 inference threads.
- **Measurements:** CLI synthesis RTF was 0.677, 0.705, and 0.621 for demos 1–3; mean 0.668.
  The corresponding generated durations were 2.869, 1.628, and 2.679 seconds. Process-start
  to complete-WAV times were 2.764, 1.965, and 2.478 seconds. Since this is an offline CLI,
  it has no observable streaming first-audio event; complete-WAV time is the measured
  first-audio bound (2.764 seconds for the first, cold invocation).
- **Streaming/server:** this release exposes `sherpa-onnx-offline-tts`, documented as
  offline/non-streaming. No Pocket TTS streaming or TTS server executable is included. The
  similarly named websocket server binaries are not Pocket TTS synthesis servers.
- **Outputs:** `~/.local/share/voxi/voice-demo/demo-1.wav` through `demo-3.wav` exist as
  mono 24 kHz PCM WAVs. Audio was not played.
