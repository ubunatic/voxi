# 199 — Catalog tested ASR and TTS models with hardware, runtimes, benchmarks, and verdicts

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Documentation
**Related**: 041 (sherpa-onnx canary), 066 (Cohere Transcribe & Nemotron canary), 131 (R2T2 evaluation), 133 (crispasr bench), 147 (OSS TTS research), 187 (KittenTTS 2)

---

## 1. Problem & Motivation

Voxi has evaluated, benchmarked, and prototyped multiple ASR and TTS models over time across various hardware targets (AMD ThinkPad T14 Gen2 CPU/iGPU, discrete GPU setups, etc.). However, benchmark findings, runtime requirements, tested parameters, and adoption/rejection verdicts are scattered across individual study documents (`docs/studies/`), historical issue tickets, scripts (`scripts/canary_*`), and commit messages.

Without a single consolidated, evergreen catalog of tested models:
- Team members and automated agents repeatedly re-research models that have already been evaluated and rejected (or adopted).
- Hardware-specific performance numbers (e.g. CPU vs Vulkan GPU RTF, RAM/VRAM footprint, first-audio latency) are difficult to locate when making architecture decisions.
- Reasons for rejecting past models (e.g., non-commercial licenses, unverified streaming support, lack of ROCm/Vulkan support, Python dependencies) risk being lost or forgotten.

## 2. /goal

Create and maintain a consolidated, structured catalog in `docs/TestedModels.md` recording all ASR and TTS models evaluated or benchmarked in Voxi, with:
1. Model metadata: name, architecture, parameters/size, vendor/source, license.
2. Runtime & Engine: runtime binary/engine (voxtype, crispasr, sherpa-onnx, piper, llama-server, etc.), dependencies (C++, ONNX, Python/venv).
3. Test environment: hardware tested on (CPU model, GPU/Vulkan node, RAM), test audio/fixtures used (e.g. JFK clip, local sample store, synthetic clips).
4. Benchmark measurements: Real-time factor (RTF), first-chunk latency, memory (RSS) / VRAM usage, speedup.
5. Verdict & Status: Adopted (default or optional), Rejected, or Parked, with decisive evidence and reasons.
Or stop and report when blocked on user input or denied permission.

## 3. Scope & Structure for `docs/TestedModels.md`

### Target Document Schema
- **Section 1: ASR Models Matrix**
  - Columns: Model Name | Parameters | Engine / Runtime | Hardware Tested | RTF / Latency | Memory / VRAM | Verdict / Notes
  - Cover: Whisper (`base.en`, `small.en`, `large-v3-turbo`), Cohere Transcribe (`crispasr`), R2T2 Confucius4 (`llama-server`), Nemotron 3.5 Streaming (canary 066), sherpa-onnx / Zipformer (canary 041), etc.
- **Section 2: TTS Models Matrix**
  - Columns: Model Name | Parameters / Architecture | Engine / Runtime | Hardware Tested | RTF / First Audio | Memory / Footprint | Verdict / Notes
  - Cover: Piper (`~/.local/lib/voxi/piper/`), Festival, espeak-ng, Kokoro-82M (ONNX canary), Pocket-TTS, VoxCPM2, Chatterbox, KittenTTS 2.
- **Section 3: Standard Hardware Testbeds**
  - Define standard machines used in baselines (e.g. ThinkPad T14 Gen2 AMD Ryzen Pro 5650U with Radeon Graphics, 16GB RAM).

## 4. Verification Plan

- Collect and cross-check data from:
  - `docs/BenchBaseline.md`
  - `docs/TTSReading.md`
  - `docs/studies/*.md`
  - Relevant issues (041, 066, 131, 133, 147, 187)
  - Output of the model audit agent (`audit-models`)
- Commit `docs/TestedModels.md` and link it from `docs/README.md`.
