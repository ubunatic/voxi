# 146 — Plan `voxi say --llm` for fluent document reading

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Feature
**Related**: 141 (monitor-gated TTS), 145 (Markdown normalization for TTS)

---

## 1. Problem

`voxi say` currently sends text directly to the TTS engine. Raw Markdown tables,
lists, and other structured sections can sound awkward when read literally.
Rewriting a large document as one LLM request could improve the narration, but
waiting for the entire rewrite before playback would delay the first audio.

## 2. Goal

Add a planned `voxi say --llm` mode that uses `lmcoder` and its configured
backend host (localhost by default) to produce fluent speech-ready text while
starting playback promptly. Done when the command can begin with the first
paragraph, continue processing the complete document as playback proceeds, and
queue later spoken chunks as they become available without losing document
context.

## 3. Desired behavior

- Use `lmcoder` and the configured host; default to the local host when no host
  is configured.
- Send the first paragraph with instructions to prepare narration and signal
  that the rest of the document will follow. Queue its resulting chunk or
  chunks as soon as they are ready.
- While those chunks play, provide the rest of the document so the LLM can
  produce subsequent narration with whole-document context. Keep feeding
  chunks to TTS without waiting for a complete-document rewrite.
- Support speech-oriented transformations such as rendering a Markdown table
  as clear spoken comparisons. Decide whether summarization or translation is
  in scope, and how the user selects those behaviors.
- Define behavior for a failed or unavailable LLM, session continuity, and
  whether the original text is read as a fallback.

## 4. Open questions

- How to reconcile low first-audio latency with transformations that need the
  complete document for context.
- Whether to use a reusable named `lmcoder` session for each reading request,
  and how to avoid context leaking between documents.
- Which output contract prevents omissions or invented content when the LLM
  converts structured text into narration.
- How the configured host is selected and reported, especially when using a
  remote host.
