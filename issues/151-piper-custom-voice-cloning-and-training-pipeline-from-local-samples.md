# 151 — Piper custom voice cloning and training pipeline from local samples

**Status**: In Progress
**Priority**: P1 (High)
**Severity**: Feature
**Category**: Feature
**Related**: `spec/tts.yaml`, `internal/tts/`, `~/.config/voxi/samples/corpus.tsv`, `docs/studies/2026-09-26-linux-desktop-tts-reading-pipeline.md`

---

## 1. Problem & Motivation

`voxi say` and desktop `Super+Y` reading currently utilize pretrained public Piper ONNX models (`en_US-lessac`, `en_US-bryce`, `en_GB-alan`, etc.). Users who record feedback samples via `voxi feedback sample record` or maintain a speech corpus in `~/.config/voxi/samples/` want to clone their own voice into a fast, native Piper ONNX voice model (`my-voice.onnx` + `my-voice.onnx.json`).

Piper ONNX models run directly in the C++ runtime with zero Python dependency at playback time, yielding ~0.05 RTF and ~140MB RAM footprint.

## 2. Technical Specification & Findings

1. **Dataset Ingestion**:
   - Parse `~/.config/voxi/samples/corpus.tsv` (ID, WAV filename, expected transcript).
   - Pre-process WAV files to 22050Hz mono 16-bit PCM.
   - Format metadata into standard LJSpeech format (`wavs/` + `metadata.csv` with `id|transcript|transcript`).

2. **Fine-Tuning / Adaptation**:
   - Use Piper fine-tuning script (`piper_train` or lightweight PyTorch VITS fine-tuning on top of a pretrained medium base checkpoint such as `en_US-lessac-medium`).
   - Run training for a set number of epochs or until convergence on local CPU/GPU (APU).
   - Export PyTorch checkpoint directly to ONNX (`.onnx`) with configuration (`.onnx.json`).

3. **CLI Workflow & Integration**:
   - Command: `voxi voice clone` (or `voxi voice train` / `voxi voice build`).
   - Flag options: `--samples-dir`, `--name <voice_name>`, `--base <base_voice>`, `--epochs`.
   - Automated output placement: Installs into `~/.local/share/voxi/voices/<name>.onnx` and enables via `voxi config --tts-model`.

## 3. Implementation & Verification Plan

1. **Dataset Preparation Utility**: Build `internal/tts/clone` (or CLI script/tool) to validate WAV files, check audio length and transcripts from `corpus.tsv`.
2. **Training & Export Harness**: Create reproducible fine-tuning script managed via `uv` or self-contained runner.
3. **Canary Test**: Train a prototype voice with existing `~/.config/voxi/samples/` recordings.
4. **Export & Live Verification**: Synthesize sentences with `voxi say --voice <name>` and benchmark audio quality and RTF.
