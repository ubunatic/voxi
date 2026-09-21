# 131 — Research and benchmark R2T2.ai open STT model

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Performance
**Related**: Current Cohere transcription integration and repository speech samples

---

## 1. Problem & Motivation

Investigate the open speech-to-text model/service associated with R2T2.ai and determine whether it can improve voxi's current Cohere transcription pipeline. The project needs an evidence-based comparison before adding another provider or changing the default model.

## 2. Technical Specification / Findings

Research the available R2T2.ai model, license, distribution/API, runtime requirements, supported languages, and integration surface. Resolve uncertainties about the exact model/repository and whether it is practical to run locally on the supported Linux/Wayland setup.

Use the existing voxi transcription samples and current Cohere path as the comparison baseline. Record transcription quality, latency, throughput/resource use, failure behavior, and operational constraints for equivalent runs. Do not assume R2T2.ai is compatible with the existing provider API until verified.

## 3. Implementation & Verification Plan

**/goal**: Produce a reproducible benchmark and, if R2T2.ai is viable, integrate it behind the existing transcription abstraction or add a narrowly scoped provider path, with tests and documented configuration. The work is done when the comparison against Cohere is recorded on the repository samples and the recommendation is clear: adopt, retain as an optional backend, or reject with reasons.

- Probe the authoritative R2T2.ai model/repository and establish a canary transcription path.
- Run both backends against the same available samples with equivalent settings; preserve benchmark methodology and results.
- Add the smallest maintainable integration only if the model passes the canary and licensing/runtime constraints.
- Verify with focused tests and the applicable project checks; document setup, limitations, and the decision.

**Status**: Draft

---

Reserved placeholder ticket.
