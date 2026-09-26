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
   - Command: `voxi voice train` (and `voxi voice prepare`).
   - Flag options: `--samples-dir`, `--name <voice_name>`, `--base <base_voice>`, `--epochs`, `--batch-size`, `--accelerator`.
   - Automated output placement: Installs into `~/.local/share/voxi/voices/<name>.onnx` and enables via `voxi config --tts-model`.

## 3. Milestones & Delivery Status

### Milestone M1: Dataset Preparation & Training Harness Scaffolding
- **Delivered**: `a8230d8` + `d4e5b1c` (path containment fix & regression tests).

### Milestone M2: Fine-Tuning Execution & Export Workflow
- **Delivered**: `253209e`
- **Review Findings (Pre-Work for Hardening / M3)**:
  - `scripts/voice-training/runner.py:54`: Preserve existing output artifacts upon training failure rather than unlinking prior ONNX before training completes.
  - `internal/tts/clone/train.go:187`: Key downloaded checkpoint cache by URL hash or full URL namespace instead of solely basename to avoid collisions.
