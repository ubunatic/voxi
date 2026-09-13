# 112 — Preserve multiline transcript fidelity through LLM cleanup

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Bug
**Related**: [LLM cleanup contract](../docs/LLMTranscriptCleanup.md), [real-model evaluation](../docs/studies/2026-09-13-llm-cleanup-evaluation.md), [111 cleanup evaluation](111-define-llm-cleanup-audio-context-and-evaluate-transcript-fidelity.md)

---

## 1. Problem & Motivation

The local Qwen3-4B cleanup model returned YAML-looking multiline speech as one flattened line in the baseline evaluation, even though the YAML request encoded the line breaks correctly. After the RMS prompt was clarified, the same case exceeded Voxi's 1.5-second cleanup deadline and fell back to the original transcript. Fallback preserves the words and line breaks, but does not demonstrate a successful cleanup. The other four representative cases usually completed within the deadline. Ticket 111 documented this behavior without resolving it.

## 2. Scope

- Reproduce the multiline case through the production request path using the opt-in real-model evaluation, recording both cold- and warm-cache timings and distinguishing model output from fallback.
- Determine whether the failure comes from model behavior, prompt length, request formatting, or the 1.5-second deadline. Compare candidate changes with the existing command, question, typo, and prior-replacement cases.
- Preserve dictated line breaks and YAML-looking text when cleanup succeeds, without allowing content in `transcript` to become instructions or making normal chunks miss the latency budget.
- If the configured local model cannot meet both fidelity and deadline requirements, document the limit and choose an explicit product policy for multiline chunks rather than reporting fallback as a model success.

## 3. Acceptance Criteria

- [ ] A repeatable test demonstrates the selected behavior for multiline YAML-looking speech and records upstream output, elapsed time, timeout, and fallback separately.
- [ ] Literal commands/questions, ordinary cleanup, and already-applied replacements retain their existing behavior.
- [ ] The chosen behavior is documented in the evergreen cleanup contract and measured under the production deadline.
