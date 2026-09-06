# 073: Read FluidVoice's Model-Download Code Paths for Direct Weight-URL Reuse (Research)

**Status**: Proposed / Research
**Priority**: P3 (Low)
**Severity**: Informational
**Category**: Research
**Related**: [064 FluidVoice model landscape research](064-fluidvoice-model-landscape-fluid-intelligence-licensing-and-underlying-stt-model-portability-research.md), [066 canary: Cohere Transcribe and Nemotron 3.5 streaming](066-canary-cohere-transcribe-and-nemotron-3-5-streaming-as-alternative-asr-backends.md)

---

## 1. Problem & Motivation

Issue 064 already established, independent of FluidVoice, that two of the
STT models FluidVoice lets users select — Cohere Transcribe and Nemotron
Speech 3.5 streaming — are open-weight and Apache-2.0/OpenMDW-1.1 licensed,
with community ONNX/sherpa-onnx exports. Issue 066 gates any Voxi adoption
on hands-on verification of those exports' provenance.

`altic-dev/FluidVoice` is itself a GPLv3, source-available Swift app (per
issues 062/063). Since the app is open source, its actual model-download
code (whatever fetches these STT models to the user's machine on first
selection) is readable without any reverse-engineering — it is plain source
review of a public repo, same as issues 062/063/067 already did for other
parts of the codebase.

The idea this ticket investigates: FluidVoice's own download code may point
directly at the *official* publisher-hosted weight artifact (Hugging Face,
NVIDIA NGC, Cohere's own hosting, etc.) for a given model, rather than a
FluidVoice-controlled mirror. If so, that URL is just a pointer to the
model's real publisher — nothing FluidVoice-proprietary about it — and
Voxi could fetch the exact same artifact directly, skipping any guesswork
about which specific model variant/quantization/export FluidVoice
validated as working. This is explicitly **not** about FluidVoice's Fluid
Intelligence model (064 already settled that as proprietary/unavailable)
— it is about the download mechanism for the third-party STT models 064/066
already scoped as licensable and portable.

## 2. Research Questions

1. Where in the FluidVoice source does model download/fetch happen for the
   STT engine roster (Cohere Transcribe, Nemotron Speech 3.5, Parakeet
   Flash/TDT, Whisper tiers)? Identify the specific file(s)/function(s).
2. For each model FluidVoice can download, what is the literal URL or URL
   template it fetches from? Is it:
   - the model publisher's own official hosting (Hugging Face repo, NVIDIA
     NGC/catalog, Cohere's own distribution) — i.e. a plain pointer with no
     FluidVoice-specific gating, or
   - a FluidVoice-controlled endpoint/mirror/CDN (which would raise
     redistribution-terms questions distinct from the publisher's own
     license)?
3. Does the download path apply any authentication, license-acceptance
   gate, checksum/signature verification, or format conversion before or
   after fetching? If FluidVoice shows the user a license/terms prompt
   before downloading a given model, capture its exact wording — Voxi
   adopting the same model would need an equivalent acceptance step.
4. For Cohere Transcribe and Nemotron Speech 3.5 streaming specifically
   (066's two canary candidates): does FluidVoice's exact fetched
   artifact match the community ONNX/sherpa-onnx exports 064 already
   found, or a different variant (e.g. FluidVoice's own CoreML conversion)?
   If different, note whether FluidVoice's variant is itself
   Linux-portable or CoreML-locked — this could change 066's plan.
5. Does anything in FluidVoice's download code encode license
   acceptance, telemetry, or attribution requirements Voxi would need to
   replicate to use the same artifact in good faith (e.g. a "you accept
   NVIDIA's/Cohere's model license" click-through, a required attribution
   string, a usage-reporting call)?

## 3. Deliverables

- A `## Research Findings` section citing the exact FluidVoice source
  file(s)/line(s) for the model-download mechanism, per model.
- The literal URL(s)/URL template(s) FluidVoice fetches from, with a
  verdict on whether each points at the model's own official publisher
  hosting or a FluidVoice-controlled mirror.
- Any license-acceptance, checksum, or attribution step FluidVoice
  performs around the download, quoted verbatim where found.
- An explicit recommendation for 066: whether to reuse FluidVoice's exact
  fetched URL/variant directly, and whether Voxi's own download flow (if
  066 leads to adoption) should surface an equivalent "you are accepting
  <publisher>'s model license" acknowledgment before fetching, modeled on
  whatever FluidVoice does (or, if FluidVoice does nothing, a note that
  Voxi would need to originate this step itself).

## 4. Non-Goals

- No code changes, no download implementation in Voxi in this ticket —
  research only, feeding into 066 if it proceeds.
- Not re-investigating Fluid Intelligence (064 already settled: proprietary,
  not applicable here since it's never distributed as a plain download URL
  in the first place).
- Not re-litigating 064's licensing verdicts — this ticket only asks where
  the actual bytes come from and what gate (if any) sits in front of them.
- Not a legal opinion — if FluidVoice's download mechanism raises a
  genuine redistribution question (e.g. it turns out to be a
  FluidVoice-controlled mirror with its own terms), flag it explicitly for
  the user to decide rather than resolving it here.

## 5. Background

Raised 2026-09-07, following issues 064/066, at the user's suggestion:
since FluidVoice is open source, its own model-download code is a
legitimate, non-adversarial way to find out exactly which URL/artifact to
pull for the STT models 064 already cleared as open-weight and portable —
and to see whether the app itself handles (or omits) a license-acceptance
step Voxi would want to mirror. Explicitly deprioritized below 066: the
user wants the already-identified open-source model candidates (066's
canary) working first, before spending effort on this convenience/
verification angle.
