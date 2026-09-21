# 132 — Evaluate Qwen3.8 live STT after release

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Performance
**Related**: [Issue 131](131-research-and-benchmark-r2t2-ai-open-stt-model.md), current Cohere transcription integration

---

## 1. Problem & Motivation

Evaluate the new Qwen3.8 live speech-to-text capability for voxi once it is publicly released. It is currently cloud-only, so local deployment and open-model integration cannot yet be assessed.

## 2. Technical Specification / Findings

Track the release and verify the authoritative API, model availability, licensing, streaming semantics, supported languages, pricing/quotas, and latency characteristics. Treat the current cloud-only status as an external dependency; do not add speculative integration before an accessible release exists.

When available, compare Qwen3.8 with the current Cohere backend using the same repository samples and equivalent live/streaming settings. Record transcription quality, first-token and final latency, stability, throughput/resource use, and operational constraints.

## 3. Implementation & Verification Plan

**/goal**: After Qwen3.8 becomes accessible, produce a reproducible comparison against Cohere and integrate it behind the existing transcription abstraction if it is a practical improvement; otherwise document why it should remain optional or be rejected.

- Re-check release status and run a canary against the official service/API.
- Benchmark both providers on the existing samples with methodology and results captured.
- Add the smallest maintainable provider/configuration path only if the canary passes and terms are acceptable.
- Verify focused tests and applicable project checks; document setup, limitations, and the adoption decision.

**Status**: Draft

---

Reserved placeholder ticket.
