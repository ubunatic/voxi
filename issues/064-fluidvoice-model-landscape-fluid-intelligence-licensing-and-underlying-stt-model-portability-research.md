# 064: FluidVoice Model Landscape: Fluid Intelligence Licensing and Underlying STT Model Portability (Research)

**Status**: In Progress — starting research per user request
**Priority**: P3 (Low)
**Severity**: Informational
**Category**: Research
**Related**: [039 OSS/source-available STT landscape and custom-vocabulary research](039-oss-stt-landscape-and-custom-vocabulary-research.md), [062 FluidVoice automatic vocabulary training research](062-fluidvoice-automatic-vocabulary-training-from-user-corrections-research.md), [063 FluidVoice spoken-punctuation research](063-fluidvoice-spoken-punctuation-and-dictation-literal-post-processing-rules-research.md), [047 OSS voice-typing tool landscape research](047-oss-voice-typing-tool-landscape-research.md)

---

## 1. Problem & Motivation

Issues 062 and 063 established that `altic-dev/FluidVoice`'s *codebase* is
100% Swift on macOS-only frameworks (Speech, Accessibility API, CoreAudio,
CoreML) — nothing there is portable/clonable to Voxi's Go/Linux stack, only
ideas are. That conclusion was about FluidVoice's *code*.

This ticket asks a narrower, different question the user raised directly:
"can we just use their model?" — i.e. is there a model (as opposed to code)
FluidVoice uses or ships that Voxi could adopt as an alternative/additional
STT backend, independent of FluidVoice's Swift/macOS app entirely?

FluidVoice's README draws a distinction between two categories of model that
must not be conflated when answering this:

1. **"Fluid Intelligence"** — FluidVoice's own on-device AI *enhancement*
   model (smart formatting, context-aware capitalization, post-processing).
   The README states it is "a separate, privately maintained local AI
   runtime" and that it is "keeping Fluid Intelligence private for now so we
   can sustainably offer the core dictation experience for free. This may
   change in the future." No license or download terms for it were found
   during the initial scan. This model is *not* in the GPLv3 repo.
2. **The STT models** FluidVoice lets users choose between: Nemotron Speech
   3.5, Parakeet Flash / TDT v2 / v3, Cohere Transcribe, Apple Speech,
   Whisper (Tiny/Base/Small/Medium/Large). These are third-party models
   (NVIDIA, Cohere, Apple, OpenAI/ggml) that FluidVoice merely *integrates* —
   FluidVoice has no special claim or license over them. "Using their model"
   for this category has nothing to do with FluidVoice as an app; it means
   Voxi adopting one of these STT models directly from its actual publisher.

Issue 039 already surveyed the OSS/source-available STT landscape,
including NVIDIA NeMo Parakeet-TDT (0.6B/1.1B): CC-BY-4.0 weights available
on Hugging Face, materially higher accuracy than Whisper small.en, but
requires NeMo/PyTorch or a custom ONNX export runtime — concluded feasible
in principle but heavier than Voxi's current single-binary whisper.cpp
approach, and not selected as a near-term canary target. This ticket must
build on that finding rather than re-deriving it, and specifically cover
what 039 did not: Parakeet Flash and TDT v3 (newer variants than what 039
surveyed), Nemotron Speech 3.5, and Cohere Transcribe, plus the separate
Fluid Intelligence question.

## 2. Research Questions

### 2.1 Fluid Intelligence (enhancement model)

1. Is Fluid Intelligence's model weights/binary ever downloaded to the
   user's machine (e.g. as part of the app bundle or a first-run download),
   and if so, is that artifact inspectable/extractable at all (format,
   whether it's a recognizable CoreML package, whether it's remotely
   fetched per-request instead of shipped on-device)?
2. Do FluidVoice's website, Discord, or any public waitlist/paid-tier page
   state licensing, redistribution, or API-access terms for Fluid
   Intelligence — even informally (e.g. "available via API for teams")?
3. Is Fluid Intelligence gated behind a paid tier, or bundled free with the
   GPLv3 app but withheld as a separate proprietary component (as the
   README implies)? Has this changed since the README snapshot fetched
   this session?
4. **Verdict**: proprietary/unavailable, licensable (state terms), or
   unknown/no public information.

### 2.2 Third-party STT models FluidVoice integrates

For each of the following, independent of FluidVoice: publisher, license,
weight availability, distribution format, and on-device runtime
requirements — specifically whether it is available in a form portable to
Linux (ONNX, GGUF, or a documented non-CoreML runtime) or is CoreML/Apple-
locked:

- **NVIDIA Parakeet Flash / TDT v2 / TDT v3**: are these newer variants
  published on Hugging Face like the TDT 0.6B/1.1B models covered in issue
  039? Same CC-BY-4.0-style terms? Same NeMo/PyTorch runtime dependency, or
  has an ONNX/GGUF export become available (e.g. via sherpa-onnx, which 039
  flagged as a plausible lightweight runtime for transducer models)?
- **NVIDIA Nemotron Speech 3.5**: publisher, license, weight availability,
  and runtime requirements — this model was not covered in issue 039 at
  all; establish it from scratch.
- **Cohere Transcribe**: is this an API-only commercial product (no
  downloadable weights), or does Cohere publish any open/self-hostable
  variant? If API-only, it is out of scope for an on-device Voxi backend
  and should be marked as such rather than investigated further.
- **Apple Speech**: confirm this is the macOS/iOS system framework with no
  standalone distributable model — out of scope for Linux by construction,
  brief confirmation only.
- Whisper (Tiny/Base/Small/Medium/Large): already Voxi's baseline via
  `voxtype`/`internal/asr` — no research needed here, note as already
  covered.

### 2.3 Feasibility verdict

For each STT model above that is not already ruled out as API-only or
platform-locked, produce one of:
- **Licensable and portable**: open weights, license compatible with a
  hobby AGPLv3 project, and an existing or plausible Linux-portable runtime
  (ONNX/GGUF/sherpa-onnx or similar).
- **Portable in principle but nontrivial**: open weights but only via a
  heavy runtime (NeMo/PyTorch) with no lightweight Go/C++-friendly path
  yet, mirroring issue 039's Parakeet-TDT conclusion.
- **Proprietary/unavailable**: no open weights, API-only, or
  platform-locked with no public licensing path.

## 3. Deliverables

- A `## Research Findings` section answering 2.1 and 2.2, with a verdict
  per §2.3 stated explicitly for each of: Fluid Intelligence, Parakeet
  Flash/TDT v2/v3, Nemotron Speech 3.5, Cohere Transcribe.
- An explicit answer to the user's literal question ("can we just use their
  model?") for both categories: Fluid Intelligence (expected: no, pending
  verification) and the underlying STT models (expected: it depends on the
  specific model, evaluated independently of FluidVoice).
- If any STT model comes back "licensable and portable" or "portable in
  principle but nontrivial" with a materially better cost/accuracy profile
  than issue 039's existing NeMo/Parakeet-TDT conclusion, flag it as a
  candidate for a future canary-evaluation ticket — do not build a canary
  in this ticket.

## 4. Non-Goals

- No code changes, no canary implementation, no ONNX/NeMo runtime
  integration in this ticket.
- Not re-litigating issue 039's Parakeet-TDT 0.6B/1.1B findings — cite and
  build on them, don't redo them.
- Not re-evaluating FluidVoice's Swift codebase/features (Command Mode,
  Rewrite Mode, vocabulary training, punctuation rules) — those are covered
  by issues 062/063 or are out of scope entirely.

## 5. Background

Raised 2026-09-06 after the user asked, following the FluidVoice research
in issues 062/063, "can we just use their model?" — a question about
FluidVoice's models rather than its code. FluidVoice's README (fetched
during the 062/063 research session) lists Fluid Intelligence as a
"separate, privately maintained local AI runtime" kept private "so we can
sustainably offer the core dictation experience for free," with no
published license or download terms found so far, and separately lists a
user-selectable STT engine roster (Nemotron Speech 3.5, Parakeet
Flash/TDT v2/v3, Cohere Transcribe, Apple Speech, Whisper tiers) that are
third-party models FluidVoice merely integrates. Issue 039 already surveyed
NVIDIA NeMo Parakeet-TDT (0.6B/1.1B) licensing and runtime feasibility in
depth; this ticket extends that survey to the newer/uncovered models and
separately resolves the Fluid Intelligence licensing question, which 039
did not address at all.
