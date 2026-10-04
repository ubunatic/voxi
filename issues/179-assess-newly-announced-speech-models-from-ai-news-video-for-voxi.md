# 179 — Assess newly announced speech models from AI news video for Voxi

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Informational
**Category**: Research
**Related**: [064 FluidVoice model landscape](064-fluidvoice-model-landscape-fluid-intelligence-licensing-and-underlying-stt-model-portability-research.md), [066 alternative ASR backend canary](066-canary-cohere-transcribe-and-nemotron-3-5-streaming-as-alternative-asr-backends.md)

---

## 1. Problem & Motivation
The video [Gemini 4, GPT 6.1, Dots, Claude Sonnet 5.5, Ideogram 4.5, Flux 3: AI NEWS](https://www.youtube.com/watch?v=lHmZoRHMZyM) mentions new speech models. Assess the two intended candidates: [Whistle](https://cactuscompute.com/blog/whistle) and [Phonon 2](https://www.fermionresearch.com/research/phonon-2/), for relevance to Voxi's Linux voice-input workflow.

## 2. Technical Specification / Findings
Verify each model's task, availability, licensing, supported languages, runtime/platform requirements, and reported accuracy/latency from primary sources. Compare any STT capability with Voxi's current backend and existing findings in issues 039, 064, and 066. Record uncertainties and distinguish published results from the video's claims.

## 3. Implementation & Verification Plan
No code changes. Recommend whether a candidate warrants a separate canary or implementation issue, with links to primary sources and a brief rationale.

**Goal**: Assess Whistle and Phonon 2 against Voxi's needs and record a source-backed recommendation for each; stop and report if essential model, licensing, or runtime facts cannot be verified from public sources.
