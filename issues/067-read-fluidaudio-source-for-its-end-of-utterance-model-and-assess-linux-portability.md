# 067: Read FluidAudio Source for Its End-of-Utterance Model and Assess Linux Portability

**Status**: Proposed / Research
**Priority**: P3 (Low)
**Severity**: Informational
**Category**: Research
**Related**: [065 FluidVoice silence detection/chunking research](065-fluidvoice-silence-detection-chunking-and-bad-chunk-rejection-research.md), [054 Short-pause acoustic gating and context priming](054-short-pause-acoustic-gating-and-context-priming.md)

---

## 1. Problem & Motivation

Issue 065's research found that FluidVoice's realtime transcription path
wires in a learned end-of-utterance (EOU) endpointing model
(`nvidia/parakeet_realtime_eou_120m-v1`), which is potentially more
sophisticated than Voxi's current acoustic-gating VAD (issue 054). That
research treated the model as opaque because the actual inference code is
delegated to an external Swift package, `FluidInference/FluidAudio`, which
065 did not have time to read.

**Correction to the framing this ticket was originally requested under**:
the ask was to "decompile the external Swift package in CoreML/Apple
Silicon," but `FluidInference/FluidAudio` turned out to be a public,
actively maintained (updated same day as this research), Apache-2.0
licensed repository (2.7k stars) — no decompilation is needed or
appropriate; the source is simply readable. There is also a community Go
bindings repo (`meddion/fluidaudio-go`) and an official Rust port
(`FluidInference/fluidaudio-rs`), which are worth checking for portability
signal in their own right.

## 2. Research Questions

1. What does FluidAudio's EOU model actually do algorithmically — is it a
   small classifier over acoustic features (energy/pitch/spectral) predicting
   "utterance ended," a streaming ASR-adjacent model repurposing hidden
   states, or something else? Read the relevant Swift source in
   `FluidInference/FluidAudio` (VAD/EOU-related files — locate via the
   repo's file tree, likely under a `Sources/FluidAudio/VAD` or similar
   path) rather than inferring from the model card alone.
2. Is the EOU model's inference path CoreML-only by construction (e.g. it
   loads a `.mlmodelc` bundle with no other runtime path), or does FluidAudio
   abstract over a portable format (ONNX) anywhere for other models in the
   package that the EOU model could plausibly follow?
3. Does `FluidInference/fluidaudio-rs` (official Rust port) or
   `meddion/fluidaudio-go` (community Go bindings) actually reimplement
   inference, or do they just bind to the same CoreML/Apple-only backend
   under the hood (i.e. bindings that still require macOS/Apple Silicon at
   runtime, not true portability)? Check each repo's actual runtime
   dependencies before concluding either way.
4. Is the underlying EOU model checkpoint (`nvidia/parakeet_realtime_eou_120m-v1`
   on Hugging Face) available in a non-CoreML format directly from NVIDIA
   (NeMo checkpoint, ONNX export), independent of FluidAudio's packaging?
5. Net verdict: is a Linux/Go-portable path to this specific EOU model
   realistic (adopt), only reachable with nontrivial reimplementation work
   (adopt in reduced form / flag as future canary), or is CoreML-only
   confirmed with no viable export path (reject)?

## 3. Deliverables

- A `## Research Findings` section answering the above, from actually
  reading FluidAudio's source (and the Rust/Go port repos' source, not just
  their READMEs).
- An explicit verdict per Section 2 Question 5, in the same
  adopt / adopt-in-reduced-form / reject style used in issues 062-065.
- If portable in any form: a rough note on what it would take to trial it
  against Voxi's `internal/audio` segmenter (issue 054) — not an
  implementation.

## 4. Non-Goals

- No code changes or canary build in this ticket — research and verdict
  only. If the verdict is favorable, a canary should be filed as a separate
  follow-up ticket (matching how issue 041 was scoped out of issue 039).
- No decompilation, binary reverse-engineering, or circumvention of any
  license/DRM — unnecessary here since the relevant repository is open
  source; if any component this ticket needs turns out to actually be
  closed-source after all, stop and report that rather than attempting to
  extract it by other means.

## 5. Background

Raised 2026-09-06, following the FluidVoice evaluation sprint (issues
062-065), at the user's request to dig further into the CoreML/Apple
Silicon-locked end-of-utterance model issue 065 flagged as unexplored.
