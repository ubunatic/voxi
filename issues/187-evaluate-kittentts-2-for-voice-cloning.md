# 187 — Evaluate KittenTTS 2 for voice cloning

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Feature
**Related**:

---

## 1. Problem & Motivation
Explore whether KittenTTS 2 is a suitable voice-cloning model for voxi.

## 2. Technical Specification / Findings
- Voxi TTS uses an embedded spec and external engines/models: Piper is invoked as a CLI with an ONNX model; VoxCPM is the closest precedent, with pinned runtime/model URLs and SHA-256 checks, a native CLI, reference WAV/transcript arguments, and WAV output (`spec/tts.yaml`, `internal/install/voxcpm.go`, `internal/tts/process.go`). The TTS backend interface already consumes generated audio files. `voxi voice clone` installs a consented voice sample locally (`internal/tts/clone/profile.go`).
- Initial upstream research indicates KittenTTS 2 has a C++ inference path, but its voice-cloning workflow reportedly requires offline Python preparation. Its custom runtime/model format and CPU LibTorch dependencies are not a drop-in fit for existing Voxi loaders. The no-Python requirement is therefore the primary feasibility gate; confirm against the pinned upstream version before implementation.
- Research sources: [C++ runtime](https://github.com/KittenML/kitten-tts-2-cpp), [architecture](https://github.com/KittenML/KittenTTS/blob/main/docs/architecture.md), [model card and license](https://huggingface.co/KittenML/kitten-tts-2). Review the model-weight license separately from runtime source licensing before redistributing or packaging weights.

## 3. Implementation & Verification Plan
Run a canary to establish whether a consented Voxi voice sample and transcript can be prepared and synthesized with the native runtime without Python or JavaScript. If feasible, pin runtime/model revisions and hashes, assess dependencies, architecture support, license/attribution, CPU latency and memory, then integrate it as an optional WAV-producing TTS backend and verify end-to-end. If native preparation cannot satisfy the no-Python constraint, document that blocker and recommendation instead of proceeding.

/goal Evaluate KittenTTS 2 for voice cloning and document a recommendation based on a no-Python feasibility canary, or stop and report if blocked on user input or denied permission.
