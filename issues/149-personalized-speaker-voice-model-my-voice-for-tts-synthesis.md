# 149 — Personalized Speaker Voice Model (my-voice) for TTS Synthesis

**Status**: Closed — Folded into 155 (Chatterbox via tts-serve)
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Feature
**Related**: [141](141-discovery-add-a-tts-engine-so-other-tools-can-have-voxi-read-text-aloud.md), [147](147-research-oss-tts-engines-and-open-weight-models-for-modest-amd-hardware.md), [148](148-move-tts-playback-queue-ownership-from-monitor-tui-to-persistent-voxi-agent-daemon.md)

---

## 1. Problem & Motivation

Voxi currently supports standard public open-weight TTS voices (such as Piper's `en_US-lessac-medium`, `en_US-bryce-medium`, `en_GB-alan-medium`, and `en_GB-southern_english_female-low`). Users want the ability to synthesize speech using their own voice ("my voice") by leveraging recorded private feedback samples (e.g., recorded dev samples and ring-buffer chunks stored in `~/.config/voxi/samples/`).

## 2. Technical Specification / Options

To integrate a personalized speaker voice into Voxi without violating resource constraints (CPU/APU friendly, <10GB VRAM, pip-free runtime, independent of live STT recording):

### Option A: Fine-Tuned Piper ONNX Voice Model (`.onnx` + `.json`)
- **Mechanism**: Train or fine-tune an existing multi-speaker base VITS/Piper checkpoint on a dataset of recorded user samples (WAVs + transcripts in LJSpeech format). Export the trained weights to an ONNX model (`my-voice.onnx`, `my-voice.onnx.json`) placed in `~/.local/share/voxi/voices/`.
- **Runtime Impact**: Minimal. Pure C++ ONNX runtime, RTF ~0.05–0.08, ~140 MiB RAM, 0 VRAM usage.
- **Data Requirement**: Typically 3–15 minutes (30–100 transcribed sentences) for clear speaker likeness; initial acoustic transfer learning can be evaluated on ~1–2 minutes of sample audio.
- **CLI / Integration**:
  ```bash
  voxi speak --voice my-voice "Reading text in my own personalized voice."
  ```

### Option B: Zero-Shot Acoustic Conditioning / Voice Cloning
- **Mechanism**: Use an open-weight zero-shot model (e.g. XTTS-v2, F5-TTS, or Chatterbox) conditioned on short reference audio clips (~10–20s) directly without training.
- **Runtime Impact**: Requires a dedicated standalone neural inference runner (ONNX / ggml / llama.cpp-style engine) with higher memory and compute requirements than Piper.
- **Trade-off**: Higher resource footprint and slower cold-start latency vs. zero training overhead.

## 3. Implementation & Verification Plan

**/goal**: Provide a reproducible workflow to generate and package a personalized user voice model from local `~/.config/voxi/samples/` recordings, allowing `voxi speak --voice my-voice` to synthesize text using the user's voice profile.

1. **Dataset Export Tooling**:
   - Provide a utility to export transcribed local feedback samples from `~/.config/voxi/samples/` into a standardized dataset format (WAV clips + normalized transcripts).
2. **Model Training / Fine-Tuning Recipe**:
   - Establish a reproducible, containerized or standalone fine-tuning pipeline to generate an ONNX voice package (`<name>.onnx` + `<name>.onnx.json`) without polluting the host environment.
3. **Packaging & Voice Discovery**:
   - Ensure the daemon and CLI automatically detect and index any user-placed `.onnx` models in `~/.local/share/voxi/voices/`.
4. **Verification**:
   - Verify acoustic quality, speaker likeness, and real-time factor (RTF < 0.20 on AMD APU) on standard benchmark sentences.
