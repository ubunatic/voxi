# Voxi Roadmap

Reconciled from the active issue backlog on 2026-10-23 (previous passes:
2026-09-29, 2026-09-27, 2026-09-11, 2026-09-10, 2026-09-05). This is a communication
artifact, not a scheduling tool; `issues/README.md` remains the authoritative
tracker.

## Value axis

Voxi earns its place as a daily driver by making local Wayland dictation feel
immediate, responsive, and **trustworthy**: the microphone starts and stops
predictably, speech becomes accurate text with minimal latency, every word the
user spoke gets typed exactly once as the right characters, and nothing else ever
reaches the desktop.

With multiple ASR engines active in production (`cohere-transcribe` via `crispasr`,
`r2t2-confucius4` via `llama-server`, `whisper` via `voxtype`) and speech synthesis
established (`piper`, `voxcpm`), the current value axis focuses on:

1. **Dictation trust & accuracy** (first priority):
   - *Zero lost speech & correct characters* — exactly-once delivery, pause handling,
     and layout fidelity.
   - *Robust silence handling* — no hallucinated text on background noise or non-speech.
2. **Model evaluation, onboarding & runtime efficiency** (leading focus this pass):
   - Structured, reproducible evaluation of new state-of-the-art ASR, TTS, and voice cloning models.
   - Strict adherence to desktop guardrails: zero-PyTorch / no heavy Python daemons in host production runtime.
   - Rapid containerized feasibility spikes (Podman/Docker) followed by lean core runtime extraction (GGUF, ONNX, C++).
   - Disk hygiene: pre-flight checks, pruning unused weights, and utilizing fast scratch mounts (`/mnt/kdev/scratch`).
3. **First-class cloud & local engine versatility**:
   - Direct integration of high-accuracy cloud endpoints (official Gemini Transcribe API) alongside local inference.
4. **Natural reading & voice cloning (TTS)**:
   - Low first-chunk latency (<300 ms), high naturalness, and permissive licensing.

---

## Now — Model Evaluation Infrastructure & Disk Hygiene

> **Update 2026-10-23:** Shipped the centralized catalog of tested models
> ([`docs/TestedModels.md`](TestedModels.md), issue 199) and multi-engine benching
> for `crispasr` (issue 133). The immediate focus is establishing the `/voxi-model`
> skill, reclaiming disk space, and standardizing bench fixtures.

- **[200 local /voxi-model skill](../issues/200-add-local-voxi-model-skill-for-evaluating-and-onboarding-asr-tts-and-voiceclone-models.md)**
  (Open, P2). Standardizes model onboarding across Claude Code, Codex, and Antigravity.
  Enforces catalog checks against `docs/TestedModels.md`, disk space pre-flights,
  `/mnt/kdev/scratch` usage, containerized spikes, and benchmark reporting.
- **[182 remove unused model files](../issues/182-remove-unused-model-files-to-free-disk-space.md)**
  (Open, P2). Implements `voxi models prune` / cleanup to purge stale weights and dead
  canary artifacts before downloading heavy new models or building container images.
- **[176 bench fixtures in sample store layout](../issues/176-speech-context-bench-fixtures-convert-testdata-speech-context-to-the-sample-store-layout.md)**
  (Open, P2). Migrates speech-context bench audio to `~/.local/share/voxi/samples/dictation/`
  so all candidate models are benchmarked against identical local reference audio.
- **[115 modifier-buffer flush drops](../issues/115-prevent-modifier-buffer-flush-drops-from-expired-stopdraintimeout.md)**
  (In Progress, P1). Implementation complete; awaiting human live-dictation check to
  close the remaining P1 drop bug.

---

## Next — Candidate Model Canaries & Direct Engine Onboarding

> **Update 2026-10-23:** Guided by the `/voxi-model` skill, evaluate our top-ranking
> ASR and TTS candidates while hardening production engine connections.

### ASR & Cloud Engines
- **[201 onboard official Gemini Transcribe API engine](../issues/201-onboard-official-gemini-transcribe-api-as-first-class-asr-engine.md)**
  (Open, P2). Native `gemini-transcribe` engine connecting directly to the Google Gemini
  REST API with inline `audio/wav` payloads via `GEMINI_API_KEY`, bypassing proxy bridges.
- **[179 assess and benchmark top new ASR candidates](../issues/179-assess-newly-announced-speech-models-from-ai-news-video-for-voxi.md)**
  (Open, P2). Run isolated canaries and bench runs for:
  - **NVIDIA Parakeet TDT 0.6B v3**: Evaluate chunked accuracy and RTF on `t14` CPU/GPU as a drop-in upgrade for eager mode.
  - **Moonshine Streaming (v2 Medium, 245M)**: Test GGUF C++ streaming draft latency.
  - **SenseVoice Small**: Test ultra-low latency CPU transcription via `sherpa-onnx`.
- **[138 treat R2T2 "language None" as no-speech](../issues/138-treat-r2t2-language-none-scaffold-as-an-explicit-no-speech-result.md)**
  (Open, P2). Eliminates silence/noise hallucinations in the active R2T2 daily driver.
- **[137 keyterm prompt biasing for openai-transcribe](../issues/137-wire-corpus-keyterms-into-openai-transcribe-prompt-for-vocabulary-biasing.md)**
  (Open, P2). Injects project vocabulary into R2T2 requests.
- **[184 make R2T2 default ASR model](../issues/184-make-r2t2-the-default-asr-model-without-llm-cleanup.md)**
  (Open, P2). Promotes R2T2 to official default once lmcoder coordination and silence checks are solid.

### TTS & Voice Feedback
- **Kokoro-82M (ONNX standalone)** (tracked in [179](../issues/179-assess-newly-announced-speech-models-from-ai-news-video-for-voxi.md)):
  Measure CPU first-chunk latency and audio quality against the Piper baseline.
- **[145 improve Markdown text normalization for TTS reading](../issues/145-improve-markdown-text-normalization-for-tts-reading.md)**
  (Open, P2). Strips formatting syntax before speaking desktop text aloud.
- **[158 call lmcoder proxy by model name](../issues/158-call-lmcoder-proxy-on-8735-by-model-name-instead-of-raw-8734.md)**
  (Open, P2). Fixes cleanup routing on port 8735.

---

## Later — Voice Cloning, Specialized Upgrades & Distribution

- **[187 evaluate KittenTTS 2 for voice cloning](../issues/187-evaluate-kittentts-2-for-voice-cloning.md)**
  (Open, P3). Test via containerized spike to verify whether voice cloning can be cleanly
  extracted without host Python/LibTorch dependencies.
- **[161 VoxCPM 2 (48 kHz multilingual) as optional model](../issues/161-voxcpm-2-multilingual-48-khz-as-optional-voxcpm-model.md)**
  (Open, P3). Upgrades cloned voice resolution once weights and Vulkan performance are validated.
- **[188 Google Cloud Speech-to-Text streaming engine](../issues/188-add-google-cloud-speech-to-text-streaming-engine.md)**
  (Open, P3). Optional gRPC streaming engine for cloud environments.
- **[125 pause media during recording](../issues/125-pause-media-playback-during-recording-resume-if-voxi-paused-it.md)**
  (Open, P3). Automatically pauses desktop media to prevent audio spill into transcripts.
- **[119 consolidate local config storage formats](../issues/119-consolidate-voxi-local-config-storage-formats-stop-words-replacements-vocabulary-samples-config-yaml.md)**
  (Open, P3). Consolidates remaining configuration into `config.yaml`.
- **Distribution & Hygiene**:
  - [001 website integration](../issues/001-website-integration.md) (In Progress, P2).
  - [116 installer portability across distributions](../issues/116-test-installer-portability-across-distributions-and-real-systemd.md) (Open, P2).
  - [183 container test of public go install](../issues/183-container-test-public-go-install-of-voxi-then-voxi-install.md) (Blocked on v0.1.16 release tag sync).

---

## Close / Park

- **132 (Qwen3.8 live STT)** — park; waiting on upstream public release.
- **122 (Gemini Flash cleanup)** — park; too slow for the 2.5s chunk latency boundary.
- **097 (public speech sample catalog)** — park; local sample store covers all dev needs.
- **096 (keyboard-clack classification research)** — park; spectral centroid/ZCR falsified.
- **186 (cross-platform GUI demo)** — blocked; awaiting user decision on TUI media-control scope.

---

## Shipped since the 2026-09-29 pass

- **Tested Models Catalog & Multi-Engine Bench:**
  - **199** ([`docs/TestedModels.md`](TestedModels.md)): Consolidated reference of all evaluated ASR/TTS models with hardware profiles, benchmarks, and verdicts.
  - **133** ([`internal/bench/bench.go`](../internal/bench/bench.go)): Multi-engine bench runner supporting `cohere-transcribe` via `crispasr` alongside Whisper models.
- **Audio Doctor & Upstream Resilience:**
  - **197 / 198** ([`internal/doctor/fix.go`](../internal/doctor/fix.go)): Comprehensive audio doctor checks, WirePlumber repairs, and fallback resilience.
  - **191** ([`internal/bench/bench_test.go`](../internal/bench/bench_test.go)): Expanded benchmark test coverage to >79%.
- **Release:**
  - **v0.1.16** cut, tagged, and published to Codeberg with signed assets and binaries.

---

## Reconciliation notes

- **New theme in `Now`:** Model evaluation infrastructure (tickets 200, 182, 176) establishing standard procedures and disk headroom before candidate trials.
- **New ticket filed in `Next`:** 201 (Official Gemini Transcribe API engine) for direct cloud ASR.
- **Moved to `Next`:** 179 focused explicitly on candidate canaries (Parakeet TDT 0.6B v3, Moonshine, Kokoro-82M).
- **Moved to `Shipped`:** 133, 191, 197, 198, 199, and v0.1.16 release.
- **Preserved:** Dictation-trust priorities (115, 138, 137, 184) and core non-Python desktop invariants.
