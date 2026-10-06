# 185 — Apply vocabulary replacements consistently to streamed transcripts

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Bug
**Related**:

---

## 1. Problem & Motivation
Configured feedback replacements do not appear to be applied consistently to recognized chunks. The reported chunk used model `r2t2-confucius4` with engine `openai-transcribe`, while eager dictation currently loads and applies replacements only for Cohere. Therefore the configured `neos → neus` mapping is skipped for this engine. The “news” transcript has no matching configured source and should remain unchanged under the exact-match rules unless the user adds that alias.

**Goal**: Apply configured vocabulary replacements consistently to streamed transcripts and verify the reported examples, or stop and report when blocked on a user decision or denied permission.

## 2. Technical Specification / Findings
`voxi chunks show 3158 --format json` reports `model: r2t2-confucius4`, `engine: openai-transcribe`, and raw/cleaned transcript “The tool is called Neus. We are using Neus.” The nearby “Neos” chunk has the same engine and should exercise the configured mapping. Replacement matching is exact, case-insensitive, and case-adaptive; do not introduce fuzzy matching for “news.”

## 3. Implementation & Verification Plan
Load and apply configured replacements for every eager ASR engine. Add focused regression coverage for the R2T2/OpenAI transcription path and verify exact matching remains unchanged for unconfigured forms.
