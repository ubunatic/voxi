# 064: FluidVoice Model Landscape: Fluid Intelligence Licensing and Underlying STT Model Portability (Research)

**Status**: Research Complete
**Priority**: P3 (Low)
**Severity**: Informational
**Category**: Research
**Related**: [039 OSS/source-available STT landscape and custom-vocabulary research](039-oss-stt-landscape-and-custom-vocabulary-research.md), [062 FluidVoice automatic vocabulary training research](062-fluidvoice-automatic-vocabulary-training-from-user-corrections-research.md), [063 FluidVoice spoken-punctuation research](063-fluidvoice-spoken-punctuation-and-dictation-literal-post-processing-rules-research.md), [047 OSS voice-typing tool landscape research](047-oss-voice-typing-tool-landscape-research.md), [066 canary ticket built from this ticket's findings](066-canary-cohere-transcribe-and-nemotron-3-5-streaming-as-alternative-asr-backends.md)

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

## 6. Research Findings (2026-09-06)

### 6.1 Fluid Intelligence — Verdict: **Proprietary/unavailable**

No public license, EULA, download-domain, or pricing terms were found for
Fluid Intelligence anywhere — not in the FluidVoice repo (no
`PrivateAIProvider.swift` code path exposes a fetchable weights URL or
license string; it talks to a private endpoint/runtime, not a plain local
model file the repo ships), not on `altic.dev/fluid`, not in third-party
coverage (bitdoze.com, explainx.ai, mactools.pro write-ups all repeat the
README's own framing verbatim, none report new terms). Third-party coverage
confirms the plain reading of the README: it is "a separately maintained
local AI runtime — not shipped as open source," free today with no paid
tier live yet, but explicitly left open for future monetization ("hosted
API, premium model tiers, or enterprise licensing" per secondary coverage,
unconfirmed by NVIDIA/altic-dev themselves). Nothing found suggests this
has changed since the README was fetched this session.

**Direct answer**: No — Fluid Intelligence is not something Voxi can "just
use." It is privately maintained specifically so altic-dev can give away
the rest of the app for free; there is no license grant, no published
weights, and no download artifact to extract even if reverse-engineering
were on the table (it isn't, per this ticket's Non-Goals and general
project ethics).

### 6.2 Third-party STT models — per-model verdicts

| Model | Publisher | License | Weights open? | Runtime | Verdict |
|---|---|---|---|---|---|
| Parakeet Flash (`parakeet_realtime_eou_120m-v1`) | NVIDIA | NVIDIA Open Model License (permissive commercial + non-commercial use) | Yes, on Hugging Face | Official: NeMo 2.5.3+, CUDA/Linux only (Ampere/Blackwell/Hopper/Volta) — no official ONNX/CoreML export. **Unofficial** community ONNX ports exist: `soniqo/Parakeet-EOU-120M-ONNX-INT8`, `thomas097/Parakeet-ONNX` — unverified maintenance/completeness | **Portable in principle but nontrivial** — same class as issue 039's Parakeet-TDT finding; community ONNX exports narrow the gap but are third-party and unvetted |
| Nemotron Speech 3.5 streaming (`nvidia/nemotron-3.5-asr-streaming-0.6b`) | NVIDIA | OpenMDW-1.1 (Linux Foundation open-model license NVIDIA adopted 2026; broad commercial/redistribution rights, similar spirit to CC-BY/Apache) | Yes, 600M params, cache-aware streaming, 40 language-locales | Official: NeMo/CUDA. **But** a community `sherpa-onnx` INT8 export already exists: `apbaxel/sherpa-onnx-nemotron-3.5-asr-streaming-0.6b-int8` — and issue 039 already established sherpa-onnx has existing Go bindings and a portable C++/ONNX runtime comparable in footprint to whisper.cpp | **Licensable and portable** — the strongest candidate found in this ticket; a ready-made sherpa-onnx artifact removes the "no lightweight Go/C++ path" objection that kept 039's Parakeet-TDT conclusion at "feasible but heavy" |
| Cohere Transcribe | Cohere / CohereLabs | **Apache-2.0** (as of ~March 2026 open-sourcing) | Yes — 2B params, Conformer encoder / Transformer decoder, tops the HF Open ASR Leaderboard (5.42% WER, beating Whisper large-v3) | Runs via `transformers`, but also has ONNX exports (`vigneshlabs/cohere-transcribe-03-2026-int8-onnx`, INT8-quantized, CPU/Apple Silicon/GPU, no PyTorch needed) **and** a dedicated `CrispASR` — a whisper.cpp-style C++ CPU runtime built specifically for this model's architecture | **Licensable and portable** — this reverses the ticket's own starting assumption. Cohere Transcribe is not an API-only product; it was open-sourced with a whisper.cpp-equivalent runtime already available. This is the single highest-value finding in this ticket |
| Apple Speech | Apple | N/A — macOS/iOS system framework | No standalone distributable model | Platform-locked to Apple hardware | **Proprietary/unavailable** (confirmed, as expected) — out of scope for Linux by construction |
| Whisper (Tiny/Base/Small/Medium/Large) | OpenAI / ggml community | MIT | Yes | Already Voxi's baseline via `voxtype`/`internal/asr` | Not re-researched — already covered |

### 6.3 Direct answer to "can we just use their model?"

- **Fluid Intelligence (the enhancement model)**: No. Proprietary, unpublished terms, kept private by design.
- **The STT models FluidVoice merely offers as options**: It depends on
  which one, and none of it is really "theirs" to grant or withhold —
  these are independent third-party models Voxi could adopt regardless of
  FluidVoice. Two of the five are genuinely promising *right now*,
  independent of anything issue 039 found:
  - **Cohere Transcribe** (Apache-2.0, open weights, whisper.cpp-style
    `CrispASR` C++ runtime, beats Whisper large-v3 on WER) — the most
    concrete near-term upgrade candidate surfaced across issues 039/064.
  - **Nemotron Speech 3.5 streaming** (OpenMDW-1.1, open weights, existing
    community `sherpa-onnx` INT8 export, and issue 039 already confirmed
    sherpa-onnx has Go bindings) — a second concrete candidate, notable
    because it's a *streaming* model (matches Voxi's eager-streaming
    architecture more naturally than a batch Whisper-style model).
  - Parakeet Flash remains "portable but nontrivial" as in 039.
  - Cohere Transcribe/Apple Speech-as-API-only assumption in this ticket's
    original framing was **wrong for Cohere** — corrected above.

### 6.4 Candidates flagged for future canary-evaluation ticket (not built here)

Per this ticket's Non-Goals, no canary was built. Two candidates are
flagged as worth a follow-up canary ticket, in priority order:

1. **Cohere Transcribe via `CrispASR` or the ONNX INT8 export** — Apache-2.0,
   best measured WER of anything surveyed across 039/064, CPU-runnable,
   whisper.cpp-equivalent runtime already exists. Highest-value candidate
   found in either research ticket.
2. **Nemotron Speech 3.5 streaming via the community sherpa-onnx export** —
   open weights, streaming architecture fits Voxi's eager pipeline, and
   sherpa-onnx Go bindings already exist per issue 039.

Both would need independent verification of the community-published ONNX/
sherpa-onnx artifacts' correctness and maintenance status before any canary
work — they are third-party conversions, not NVIDIA/Cohere-official
exports (Cohere's own ONNX/CrispASR path is the exception — that one comes
from Cohere-adjacent tooling, not a random community re-export, so it
warrants the higher priority above).

Both candidates above were carried forward into
[066](066-canary-cohere-transcribe-and-nemotron-3-5-streaming-as-alternative-asr-backends.md),
the canary ticket that gates any adoption decision on hands-on
verification and a real benchmark — see that ticket for current status
before treating any finding here as production-ready.

Separately, [073](073-read-fluidvoice-s-model-download-code-paths-for-direct-weight-url-reuse-research.md)
(lower priority, deferred until 066's canary is done) investigates
whether FluidVoice's own (GPLv3, readable) download code points directly
at these models' official publisher hosting, which could shortcut
identifying the exact artifact/variant to fetch.

**See also**: the session retrospective at
[docs/studies/2026-09-07-fluidvoice-review-and-chunk-diagnostics.md](../docs/studies/2026-09-07-fluidvoice-review-and-chunk-diagnostics.md)
covers this research alongside the sibling FluidVoice tickets (062/063/065)
and notes this ticket's findings are secondhand web-search research, not
independently verified — a caveat 066 already gates on.
