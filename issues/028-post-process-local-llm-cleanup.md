# Local LLM Post-Process Hook for Dictation Cleanup & Voice Commands

- **Status:** Proposed / Research & Design
- **Related Issues:** 020 (Voice Input), 021 (Fluent Streaming), 026 (Continuous Eager Sentence Streaming), 027 (Continuous Listening & Turn-Taking — shares the "human over" stop delimiter)

## Context & Vision

The `2026-08-18-voxtype-popular-applications.md` study documents a `[output.post_process]` hook pattern: raw transcript in on `stdin`, cleaned text out on `stdout`, wired in before text is typed. Community usage pipes this through small local LLMs (Ollama, `llama3.2:1b` etc.) to strip filler words, fix punctuation, and recognize spoken commands.

**Note on source reliability:** the study's exact config keys/CLI syntax for the upstream `voxtype` project are unverified (no canary run against the real binary/repo). Treat the hook *contract* below as the harnez-owned design, not a transcription of an external spec.

**Goal:** Add a generic, pluggable post-process step to harnez's voice-input pipeline that can run transcript cleanup and voice-command handling through a local LLM, without hard-wiring harnez to any specific LLM runtime.

---

## 1. Hook Design

A **generic `post_process` hook**, not a built-in Ollama client:

- Config-driven external command (or in-process plugin interface — TBD during design) that receives the raw/finalized transcript and returns cleaned text (or a command result) to type.
- Runtime-agnostic: works with a shell script, `ollama run`, `llama.cpp` server call, or a future homeserver-hosted LLM endpoint — harnez doesn't assume which.
- Must degrade safely: if the hook fails or times out, fall back to typing the untouched transcript rather than blocking or dropping it (mirrors the Synthetic Input Safety doc's sanitize/validate requirement — cleaned output still needs the same ANSI/log-injection sanitization as raw ASR text before it reaches `dotool`).

## 2. Scope of Cleanup & Voice Commands

In scope for this ticket:

1. **Filler/punctuation cleanup**: remove "um/uh/like", fix punctuation and capitalization.
2. **Voice commands**, detected and acted on instead of typed:
   - `"clear history"` — clears local dictation history (existing pattern from the study).
   - `"camel case ..."` / `"snake case ..."` — case-transform the following phrase.
   - `"markdown start"` / `"markdown stop"` — bracket a span of dictation and format it as Markdown on commit (list/heading/emphasis inference) instead of plain prose.
   - `"human over"` — stop/finalize recording. **Shared surface with issue 027**: 027 owns wake-word/turn-taking activation; this ticket only needs the hook to recognize and strip the delimiter from the committed transcript. Don't duplicate 027's endpointing logic here — coordinate on where the delimiter is detected (audio/VAD layer vs. text/LLM layer) during design.
3. **Code-aware formatting** (stretch, not blocking v1): dictated code patterns like "const foo equals bar" → `const foo = bar`. Flag as a later phase since it needs identifier/context awareness beyond simple text cleanup.

## 3. Pipeline Placement & Session Context

Runs **after** the eager-streaming chunk commit (issue 026) — i.e., on already-finalized sentence chunks, right before typing. This keeps eager-streaming's 0.5–1.5s latency target intact for the ASR side; LLM latency is additive on top of each committed chunk.

Open design question to resolve before implementation: **how does the LLM keep session context across chunks?**

- Option A: harnez resends prior chat history (transcript-so-far) as part of each prompt — simple, stateless on the LLM side, but re-pays prompt-processing cost per chunk.
- Option B: rely on LLM-side context caching (e.g. persistent KV-cache session, `llama.cpp` server's slot/cache reuse, or an Ollama keep-alive session) so only the new chunk needs to be processed — faster, but depends on runtime support and needs a canary before committing to it.

This ticket should spike both and measure latency before picking a default.

## 4. Runtime / Model Target

Not decided. Candidates: `llama.cpp` (local, simple to embed/call), Ollama (matches the study's examples), or a future LLM server on the user's homeserver (not yet set up — capacity available but unconfigured). The hook contract in §1 must not assume any of these; the reference implementation can start with whichever is easiest to stand up locally (likely `llama.cpp` or Ollama) and swap later without changing the hook interface.

## Acceptance Criteria

- [ ] `post_process` hook contract defined (invocation shape, timeout/failure behavior, sanitization pass on output).
- [ ] Reference implementation with a chosen local runtime for filler/punctuation cleanup.
- [ ] Voice commands: clear history, camelCase/snakeCase, markdown start/stop, human-over delimiter stripping (coordinated with issue 027).
- [ ] Session-context strategy (resend-history vs. LLM-side caching) benchmarked and a default chosen.
- [ ] Canary test demonstrating fallback-to-raw-transcript on hook failure/timeout.
- [ ] Code-aware formatting explicitly deferred to a follow-up issue if not done in v1.
