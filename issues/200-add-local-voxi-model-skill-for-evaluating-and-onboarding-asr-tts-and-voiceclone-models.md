# 200 — Add local /voxi-model skill for evaluating and onboarding ASR, TTS, and VoiceClone models

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Tooling
**Related**: 066 (canary alternative ASR), 133 (crispasr bench), 154 (pocket-tts canary), 160 (voxcpm GGUF adoption), 187 (kittentts 2 evaluation), 199 (tested models catalog)

---

## 1. Problem & Motivation

Evaluating and onboarding new ASR, TTS, and voice cloning models into Voxi requires navigating specific architectural patterns and constraints:
- Desktop efficiency constraints: zero-PyTorch / no persistent heavy Python daemons in production runtime; native standalone binaries (C/C++, GGUF, ONNX) or lightweight loopback HTTP daemons (`llama-server`) are preferred.
- Standard hardware profiles: Lenovo ThinkPad T14 Gen2 AMD (Ryzen Pro 5650U Cezanne APU, Vulkan RADV) and workstation server `x600` (Ryzen 8700G, Radeon 780M).
- Strict latency budgets: ASR eager chunk RTF < 0.5 (CPU) / < 0.15 (GPU); TTS first-chunk audio latency < 300 ms.
- Spec-driven configuration: `spec/models.yaml` for ASR and `spec/tts.yaml` for TTS.
- Documentation & memory: updating `docs/TestedModels.md` and `docs/BenchBaseline.md` so benchmark results and adoption/rejection verdicts are preserved.

Without a dedicated skill, different coding agents (Claude, Codex, Antigravity) explore model integration ad-hoc, sometimes overlooking existing negative results in `docs/TestedModels.md` or violating the no-Python desktop runtime rule.

## 2. /goal

Create a project-local skill `/voxi-model` (anchored in `.agents/skills/voxi-model/SKILL.md` with multi-agent discoverability for Claude Code, Codex, and Antigravity) that guides agents through evaluating, canary-probing, benchmarking, and onboarding new ASR, TTS, and VoiceClone models into Voxi, or stop and report when blocked on user input or denied permission.

## 3. Skill Specification

### Location & Discovery
- Primary definition: `.agents/skills/voxi-model/SKILL.md` (standard for Antigravity & Codex).
- Compatibility links/paths: ensure Claude Code (`.claude/skills/voxi-model` or slash command entry) and Codex discover the skill.
- Cross-reference in `AGENTS.md` / `CLAUDE.md`.

### Workflow Stages Covered by `/voxi-model`
1. **Phase 1: Catalog & Prior Art Audit**:
   - Check `docs/TestedModels.md` before testing any model to avoid duplicating past evaluations or re-testing rejected models.
2. **Phase 2: Feasibility & Guardrails Gate**:
   - Runtime gate: Must ultimately compile to native binary, GGUF/ggml, ONNX, or lightweight loopback server. Reject heavy PyTorch requirements for host desktop distribution.
   - **Containerized Spike vs. Direct Lean Extraction**:
     - *Direct Lean Extraction (Preferred)*: If an easy path exists to extract or run the lean core runtime upfront (e.g., existing GGUF checkpoint, ONNX export, `whisper.cpp`/`sherpa-onnx`/`llama-server` bindings), do so immediately to avoid downloading heavy multi-gigabyte PyTorch/CUDA environments (only model weights need to be downloaded).
     - *Containerized Spike (Fallback/Exploratory)*: For novel or complex models where only an upstream PyTorch/heavy install is available, testing in Docker/Podman is permitted to evaluate voice quality, accuracy, and latency in isolation without polluting the host environment. Once proven, proceed to Step 2: lean-extract the core runtime for desktop distribution.
   - **Disk Space Pre-flight & External Scratch Mount**:
     - Pre-flight free disk space check (`df -h`) before downloading models (GGUF, ONNX) or building container images.
     - When local disk space is constrained or for large scratch builds/caches:
       - Check for external scratch mount `/mnt/kdev/scratch` (or another path configured/specced in Voxi spec/env).
       - If space is tight (<10 GB free) on root/home, prompt the user for permission to mount/use `/mnt/kdev` or an alternate specced external directory rather than filling the root drive.
   - License gate: Must be OSI / permissive open-source (MIT, Apache-2.0, BSD). Non-commercial licenses (CC-BY-NC) cannot be shipped.
   - Hardware target: Must specify CPU and/or Vulkan GPU expectations on `t14` / `x600`.
3. **Phase 3: Canary-First Probing**:
   - Isolate model probe in `scripts/canary_<model>/` before touching core engine code.
   - Test CLI flags, stdout/stderr formatting, startup banner, and device detection.
   - Measure first-audio / first-token latency, RTF, and RSS memory.
4. **Phase 4: Engine Integration & Spec Wiring**:
   - ASR: wire into `spec/models.yaml` (engine: `whisper`, `cohere-transcribe`, or `openai-transcribe`). If adding a new engine, extract runner into `internal/<engine>` and delegate from `internal/eager`.
   - TTS: wire into `spec/tts.yaml` (engine: `piper`, `voxcpm`, or loopback service).
5. **Phase 5: Benchmark Runner & Verification**:
   - ASR: Add model to `internal/bench` multi-engine runner (`voxi bench`). Ensure missing binaries/backends report per-row errors instead of aborting.
   - TTS: Run `scripts/canary_tts/benchmark.py` against reference sentence.
   - Pass `make test-q1`, update `docs/TestedModels.md` and baseline docs.
   - Execute `make restart-service` and verify live functionality.

## 4. Verification Plan

- Verify skill file syntax and YAML frontmatter (`name: voxi-model`, `description`).
- Verify discoverability in Antigravity (`agy`), Claude Code, and Codex.
- Commit the skill, index, and ticket updates cleanly.
