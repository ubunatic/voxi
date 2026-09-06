# 066: Canary: Cohere Transcribe and Nemotron 3.5 Streaming as Alternative ASR Backends

**Status**: Proposed / Canary
**Priority**: P2 (Medium)
**Severity**: Informational
**Category**: Research / Feature
**Related**: [064 FluidVoice model landscape research](064-fluidvoice-model-landscape-fluid-intelligence-licensing-and-underlying-stt-model-portability-research.md), [039 OSS STT landscape research](039-oss-stt-landscape-and-custom-vocabulary-research.md), [041 sherpa-onnx hotwords canary](041-sherpa-onnx-hotwords-canary.md), [050 Optional warm-model daemon transcription](050-optional-warm-model-daemon-transcription.md)

---

## 1. Problem & Motivation

Issue 064's research (triggered by evaluating `altic-dev/FluidVoice`'s
model lineup) turned up two STT models that are both **open-weight,
Apache-2.0-style licensed, and Linux-portable** — a materially stronger
combination than anything issue 039's landscape survey found at the time:

- **Cohere Transcribe** — open-sourced ~March 2026, Apache-2.0, ~2B
  params, tops the Hugging Face Open ASR Leaderboard, reportedly beating
  Whisper large-v3 (5.42% WER). Has a dedicated whisper.cpp-style C++
  runtime (`CrispASR`) plus ONNX exports.
- **Nemotron Speech 3.5 (streaming variant)** — NVIDIA, open weights
  (OpenMDW-1.1), streaming architecture that matches Voxi's own eager
  streaming pipeline more closely than a batch model would. A community
  `sherpa-onnx` INT8 export already exists, meaning it could reuse the same
  sherpa-onnx Go bindings path issue 039/041 already scoped.

Both are candidates to sit alongside (or replace) Voxi's current whisper.cpp
`small.en` baseline. This is unverified secondhand research (web search,
not a hands-on build) — this ticket exists to actually run both models
against Voxi's existing benchmark harness before any adoption decision.

## 2. Explicit Gate — Do Not Start Implementation Until

- Both models are confirmed installable/runnable locally under a CPU or
  single-GPU budget comparable to Voxi's current whisper.cpp setup (license
  terms, weight download source, and runtime dependency all re-verified
  first-hand, not taken on issue 064's secondhand research alone).
- The canary itself (Section 3) is run and recorded — this ticket does not
  pre-authorize a backend swap, only a measurement.

## 3. Desired Design (Canary Only — Not Production Plumbing)

1. **Verify licensing/artifact provenance first-hand**: re-confirm Cohere
   Transcribe's Apache-2.0 license and Hugging Face weight availability, and
   Nemotron 3.5 streaming's OpenMDW-1.1 terms and the community sherpa-onnx
   INT8 export's provenance/trustworthiness (who published it, is it
   reproducible from the official checkpoint).
2. **Build/install** each candidate locally, independent of Voxi's existing
   whisper.cpp pipeline — `CrispASR` (or ONNX runtime) for Cohere Transcribe,
   sherpa-onnx for Nemotron 3.5 streaming (reusing whatever sherpa-onnx
   install/Go-bindings groundwork issue 041 already did, if any survives).
3. Reuse issue 032's fixture corpus (`testdata/speech-context/`, private
   WAVs, git-ignored) to run the same paired comparison Voxi has used for
   every prior engine/vocabulary canary: baseline vs. candidate, same
   technical-vocabulary terms.
4. Record WER, exact keyterm recall, latency/RTF (streaming latency matters
   more for Nemotron given Voxi's eager pipeline), and model/binary size
   against the whisper.cpp `small.en` baseline, using the same hardware
   prior canaries (032/040/041) ran on.
5. Evaluate operational cost: new runtime dependency footprint, Go bindings
   maturity (sherpa-onnx path) or CGo/C++ bridging cost (`CrispASR`),
   model download size, and whether either integrates with Voxi's existing
   `internal/speechcontext` vocabulary-biasing mechanism at all (Cohere
   Transcribe and Nemotron may not support prompt-based biasing the way
   whisper.cpp does — check before assuming parity).

## 4. Acceptance Criteria (for the canary itself, not adoption)

- A working, reproducible install/run command for each candidate is
  recorded in this ticket.
- The comparison table (WER, keyterm recall, latency/RTF, size, dependency
  cost) is recorded against the whisper.cpp baseline, for both candidates,
  with hardware details.
- An explicit recommendation: adopt one/both as an additional selectable
  backend, adopt as a default-swap candidate (only if a clear win with no
  material regression), or reject with reasons — matching the verdict style
  of issues 041/051.

## 5. Non-Goals

- No production backend integration in this ticket — canary/benchmark only.
- Not re-litigating issue 064's licensing research — this ticket's job is
  to verify it hands-on and measure accuracy/latency, not redo the survey.
- Not evaluating Cohere Transcribe/Nemotron's decoder-level vocabulary
  biasing support in depth beyond a yes/no check — a deeper biasing
  investigation (if either is adopted) would be its own follow-up, mirroring
  how 032/040/041 handled whisper.cpp/sherpa-onnx.

## 6. Background

Raised 2026-09-06, following the FluidVoice evaluation sprint (issues
062-065) and specifically issue 064's model-landscape research, at the
user's request to canary the two highest-value model findings before
treating them as anything more than research.
