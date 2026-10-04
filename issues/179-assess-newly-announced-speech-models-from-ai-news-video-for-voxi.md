# 179 — Assess newly announced speech models from AI news video for Voxi

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Informational
**Category**: Research
**Related**: [064 FluidVoice model landscape](064-fluidvoice-model-landscape-fluid-intelligence-licensing-and-underlying-stt-model-portability-research.md), [066 alternative ASR backend canary](066-canary-cohere-transcribe-and-nemotron-3-5-streaming-as-alternative-asr-backends.md)

---

## 1. Problem & Motivation
The video [Gemini 4, GPT 6.1, Dots, Claude Sonnet 5.5, Ideogram 4.5, Flux 3: AI NEWS](https://www.youtube.com/watch?v=lHmZoRHMZyM) mentions recent speech models, including a very small CPU speech-to-text model (described as 16.9 MB) and ElevenLabs' V4 text-to-speech model. The captions do not reliably identify the tiny STT model. Determine whether any announced model is relevant to Voxi's Linux voice-input workflow.

## 2. Technical Specification / Findings
Identify the models from primary sources and verify availability, licensing, supported languages, runtime/platform requirements, and reported accuracy/latency. Compare STT candidates with Voxi's current backend and existing findings in issues 039, 064, and 066. Record uncertainties and distinguish published results from the video's claims. Assess TTS only for any direct Voxi use case; do not assume it belongs in the dictation pipeline.

## 3. Implementation & Verification Plan
No code changes. Recommend whether a candidate warrants a separate canary or implementation issue, with links to primary sources and a brief rationale.

**Goal**: Identify and assess the video's relevant speech models against Voxi's needs, then record a source-backed recommendation or rule them out; stop and report if model identity or essential licensing/runtime facts cannot be verified from public sources.
