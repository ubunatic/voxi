# 146 — Plan `voxi say --llm` for fluent document reading

**Status**: Closed — resolved
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

Add `voxi say` LLM narration using `lmcoder` and its configured backend host.
Narration is enabled by default, starts playback after rewriting the first
paragraph, then sends the remaining document in the same isolated session while
playback proceeds. Later narration is queued as soon as the continuation is
ready.

## 3. Desired behavior

- Use `lmcoder` and the configured host. Resolve the host from `--llm <host>`,
  then `tts_llm_host` in `~/.config/voxi/config.yaml`, then the embedded spec
  default `localhost`. This developer host is configured as `x600`.
- Enable LLM narration by default. `--no-llm` reads the original text directly.
- Send the first paragraph with instructions to prepare narration and signal
  that the rest of the document will follow. Queue its resulting chunk or
  chunks as soon as they are ready.
- While those chunks play, provide the rest of the document so the LLM can
  produce subsequent narration with whole-document context. Keep feeding
  chunks to TTS without waiting for a complete-document rewrite.
- Support speech-oriented transformations such as rendering a Markdown table
  as clear spoken comparisons. Decide whether summarization or translation is
  in scope, and how the user selects those behaviors.
- Use a unique session for each `say` request and reuse it for the continuation
  so context cannot leak between documents.
- If the LLM is unavailable or the first request fails, queue the original
  document. If the continuation fails after the first paragraph is queued,
  append the remaining original text so playback still covers the document.
- Keep summarization and translation out of scope. Narration preserves facts and
  detail while rendering Markdown structure for listening.

## 4. Implementation notes

- The first paragraph is rewritten and queued before the remaining text is
  submitted. The continuation uses the same fresh per-request lmcoder session,
  allowing playback and complete remaining-document processing to overlap.
- lmcoder canary checks confirmed `--host`, `--session`, stdin input, and plain
  output work for this flow.
- Tests cover default-on behavior, host selection, session continuity, and
  fallback when continuation fails.
