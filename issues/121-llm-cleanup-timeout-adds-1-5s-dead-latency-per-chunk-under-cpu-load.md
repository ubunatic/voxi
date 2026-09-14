# 121 — LLM cleanup timeout adds ~1.5s dead latency per chunk under CPU load

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Performance / Core Eager Engine
**Related**: [Issue 115](115-prevent-modifier-buffer-flush-drops-from-expired-stopdraintimeout.md), [internal/eager/eager.go](../internal/eager/eager.go)

---

## 1. Problem & Motivation

Under moderate CPU load (~11-40%, from unrelated light background work), the
local LLM cleanup step (`qwen3-4b-instruct-2507-q4`, `cleanWithLLM` in
`internal/eager/eager.go`) consistently misses its 1500ms budget and falls
back to the raw ASR transcript — this fallback logic itself is correct and
was just hardened in issue 115 §5.5 (`fallback_reason` taxonomy). But every
chunk that falls back pays the **full 1500ms timeout as pure dead latency**
before typing proceeds, since cleanup runs sequentially after ASR completes
and typing waits for it to resolve one way or the other.

## 2. Evidence

Two live chunks dictated during light background CPU work
(`voxi chunks list`/`show`, cross-checked against
`~/.local/share/voxi/eager-telemetry.jsonl`):

| Chunk | ASR done (`transcription_completed`) | `llm_cleanup_fallback` fires | `typing_started` | Gap after ASR |
|---|---|---|---|---|
| #1252 | 18:30:24.272 | 18:30:25.775 (`reason=timeout`) | 18:30:25.789 | **1.52s** |
| #1253 | 18:30:37.488 | 18:30:38.990 (`reason=timeout`) | 18:30:39.006 | **1.52s** |

Both chunks: `llm_cleanup.modified: false`, `llm_cleanup.fallback_reason:
"timeout"` — the cleanup call never returned in time, and typing was
blocked on it regardless. Total per-chunk latency in this run was roughly
ASR time (~2.9s, itself elevated — RTF ~0.5 vs. typically lower on an idle
CPU) + the full 1.5s wasted cleanup timeout + negligible injection time
(~10ms).

## 3. Root Cause

`cleanWithLLM` bounds itself at 1500ms (request context + `http.Client.Timeout`,
per issue 115's audit) and correctly falls back to the ASR text on any
failure — but there is no fast-fail or load-awareness: every chunk always
attempts cleanup and always waits out the full budget before giving up, even
when the local model server is demonstrably contended (e.g. a recent
back-to-back run of `timeout` fallbacks in the same session).

## 4. Desired Fix (options to evaluate during implementation)

- **Adaptive skip**: if N consecutive chunks in the current session hit
  `fallback_reason: timeout`, skip attempting cleanup for a cooldown window
  (or for the rest of the session) rather than paying 1500ms per chunk for
  calls very likely to fail again.
- **Lower default timeout / fail faster**: investigate whether 1500ms is
  overly generous relative to typical successful-cleanup latency on an idle
  CPU, and whether a shorter budget (with the same fallback behavior)
  reduces wasted time without increasing false-timeout rate when the CPU
  genuinely is free.
- **Load-aware gating**: skip the cleanup attempt entirely when local CPU
  load (or a cheap proxy, e.g. recent ASR RTF trending high) suggests the
  cleanup call is unlikely to complete in budget.
- Out of scope: changing the ASR transcription latency itself, or the
  cleanup model/prompt.

## 5. Verification

- Reproduce with a synthetic CPU load generator (e.g. `stress-ng`) alongside
  `voxi eager --type`, confirm current ~1.5s-per-fallback-chunk tax via
  `voxi chunks show <N>` and `eager-telemetry.jsonl` timestamps as above.
- After fix, confirm the same load scenario no longer pays the full 1500ms
  on every chunk once contention is detected, while a genuinely idle CPU
  still gets successful cleanup at roughly the same rate as before (no
  regression in cleanup quality/availability when the CPU has headroom).
