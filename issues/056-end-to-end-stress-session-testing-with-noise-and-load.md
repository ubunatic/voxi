# 056: End-to-End Stress Session Integration Testing with Interleaved Acoustic Noise and CPU/GPU Contention

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Performance
**Related**: [052 cpu-gpu priority under load](052-cpu-gpu-priority-under-load.md), [054 short pause acoustic gating](054-short-pause-acoustic-gating-and-context-priming.md), [internal/audio/audio.go](../internal/audio/audio.go), [internal/eager/eager.go](../internal/eager/eager.go)

---

## 1. Problem & Motivation

Unit tests and isolated dev-sample benchmarks evaluate transcription accuracy and acoustic gating on individual, pre-cut WAV utterances. However, in real-world dictation workflows:
1. **Interleaved Transients & Pauses**: Dictation sessions span minutes, during which users pause between thoughts, type on their keyboard (creating click spikes and transients), cough, or bump the desk. Acoustic segmenters must properly reject non-speech spikes, retain real speech segments, and maintain stream alignment without drifting or emitting hallucinations across multiple consecutive utterances.
2. **System Contention & Pipeline Stalling**: Transcription frequently runs while the workstation is under heavy CPU and GPU load (e.g. compiling large projects, browser tabs running WebGL, local LLM inference). Resource starvation can delay reading audio from PipeWire/SoX, stall the ASR worker pipeline, increase latency, and cause frame drops or queue buildup.

We need a repeatable, scriptable integration/stress test harness that concatenates realistic speech samples with interleaved acoustic transients (such as keyboard click smash artifacts), runs continuous eager streaming playback, and optionally exerts synthetic CPU/GPU contention to observe session resilience and transcript integrity.

---

## 2. Proposed Experiment Design

### 2.1 Synthetic Session Composition (Audio Stitching)
Create an integration test harness or script (e.g. `scripts/stress_session_bench.go` or an integration test suite under `internal/eager`):
- Take a corpus of known speech fixtures (`testdata/speech-context/corpus.tsv`, dev samples).
- Splice them into a single continuous multi-minute audio stream with:
  - Variable pauses (e.g. 500ms, 1.2s, 3s).
  - Interleaved non-speech transient clips inserted during pauses (e.g. `artifact-keyboard-smash.wav`, single key clicks, breaths).
- Define the ground truth expected full transcript (the concatenation of accepted speech utterances, asserting that none of the noise transients produce typed tokens).

### 2.2 Feeding into Eager Streaming
Feed the spliced PCM stream directly into `voxi eager` or an in-process mock capture pipeline (simulating stdin/pipe read from audio capture):
- Verify:
  - Speech utterances trigger and transcribe accurately.
  - Interleaved transients trigger `rej:low_energy_transient` or are absorbed as silence without invoking `voxtype`.
  - Consolidated output transcript matches expected text without inserted hallucinated words (e.g. "Thanks for watching!", "Switch Boss.", "andcom.").
  - Chunks in the ring buffer accurately distinguish accepted speech from rejected noise chunks.

### 2.3 Synthetic Resource Contention (CPU & GPU Load Injection)
To simulate real-world workstation load and observe pipeline behavior under stall conditions:
- **CPU Stress**: Spawn background worker threads (e.g. `stress-ng --cpu N` or in-Go compute loops) to saturate CPU cores.
- **GPU Stress**: Run a lightweight GPU kernel or compute workload (e.g. compute shader, Vulkan/OpenCL matrix multiplication, or concurrent `voxtype`/llama-cli instance) competing for VRAM and GPU compute queues.
- **Metrics to Measure**:
  - Utterance latency and Real-Time Factor (RTF) distribution under load vs. idle.
  - Whether `voxtype` times out or stalls worker queue channel buffers.
  - Whether audio frames are dropped by the audio reader when the worker is busy.

---

## 3. Implementation & Experiment Plan

1. **Audio Splicer / Session Generator**:
   - Write a helper utility that reads WAV files from `testdata/` and generates a single concatenated test stream with configurable pauses and noise events.
2. **Integration Test Suite**:
   - Add a test (e.g. behind a build tag or `-test.run TestStressSession -test.short=false`) that runs the session through `AudioSegmenter` and worker channels.
3. **Contention Harness**:
   - Provide command-line flags or a script to enable synthetic CPU/GPU load during the session run.
4. **Validation & Assertions**:
   - Compare final concatenated transcript against expected text via Word Error Rate (WER) or exact sequence match.
   - Assert zero accepted noise artifacts in the session manifest.
