# 122 — Add Gemini Flash 3.7 support as LLM cleanup model

**Status**: In Progress — subprocess integration implemented; real cleanup blocked by `agy` latency
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Feature
**Related**: [docs/LLMTranscriptCleanup.md](../docs/LLMTranscriptCleanup.md), [internal/eager/eager.go](../internal/eager/eager.go), [Issue 121](121-llm-cleanup-timeout-adds-1-5s-dead-latency-per-chunk-under-cpu-load.md)

---

## 1. Problem & Motivation

Voxi's LLM cleanup step (`cleanWithLLM`, see `docs/LLMTranscriptCleanup.md`)
currently only supports a locally-hosted OpenAI-compatible endpoint
(`openai_base_url`, e.g. a local `qwen3-4b-instruct` server). Issue 121
documents that this local model consistently misses its 1500ms budget under
even light CPU contention, wasting ~1.5s per chunk on a timeout-then-fallback.
A faster or offloaded cleanup model would help both latency and quality.

## 2. Desired Integration — via the `agy` CLI, not a direct API call

**Important: do not implement this as a direct HTTP call to a Gemini API
endpoint.** Voxi should shell out to the local `agy` CLI tool (already
installed at `~/.local/bin/agy`) in non-interactive print mode, the same way
other local tooling is invoked as a subprocess rather than re-implementing
auth/provider routing in Voxi itself:

```sh
agy --print --model gemini-3.7-flash-<tier> --prompt "<system+user content>"
```

`agy models` confirms `gemini-3.7-flash-high` / `-medium` / `-low` are
available model IDs today. This keeps API keys, provider auth, and model
routing entirely inside `agy`'s existing configuration — Voxi never handles
Gemini credentials directly.

Open implementation questions to resolve during design/advisory phase:

- Which tier (`high`/`medium`/`low`) is the right default for cleanup
  latency vs. quality — likely `low` or `medium` given cleanup needs to be
  fast, not deeply reasoned.
- Whether to reuse `agy --print` per chunk (process-spawn overhead per
  request) or investigate a longer-lived `agy` session/pipe mode if one
  exists, to avoid repeated cold-start cost — needs a canary probe of `agy`'s
  process-spawn latency before committing to a per-chunk-subprocess design
  (per `docs/Canary.md`).
- How to map the existing request contract (system message + YAML user
  message documented in `docs/LLMTranscriptCleanup.md`) onto `agy --prompt`'s
  single string input — likely concatenate system + user content the same
  way, since `agy` in print mode does not expose a system/user role split.
- Response parsing: `agy --print` returns plain text to stdout by default;
  `--output-format json`/`stream-json` may give more structured/parseable
  output and should be evaluated for reliability (e.g. detecting failures
  vs. a legitimate empty/unchanged response).
- Timeout handling: `--print-timeout` (default 5m) needs to be bounded down
  to something appropriate for eager-chunk cleanup (issue 121 discusses the
  right budget — likely still in the ~1-2s range, well below `agy`'s
  default).
- Config surface: add a new `cleanup_backend` (or similar) config option
  alongside the existing `llm_cleaner`/`cleanup_model`/`openai_base_url`
  keys so users can choose local-HTTP vs. `agy`-subprocess cleanup, per
  `docs/LLMTranscriptCleanup.md`'s existing config-loading conventions.

## 3. Privacy/Architecture Note (surface to user, do not decide unilaterally)

Voxi's README describes it as "privacy-first... with no cloud
transcription." Gemini Flash via `agy` is a cloud-backed model — this is a
deliberate scope difference from the existing local-only cleanup path, not
a replacement for it. Cleanup is already optional and off by default; when
implementing this, make cloud-backed cleanup clearly opt-in and keep the
local-model path as the default/no-cloud option, and consider whether
README/docs need a caveat that enabling `agy`-backed cleanup sends
transcript text to a cloud provider (via `agy`'s own provider routing).

## 4. Out of Scope

- Implementing support for other `agy`-routed models (Claude, GPT-OSS) —
  file separately if wanted; this ticket is scoped to Gemini Flash 3.7
  specifically, per the request that created it.
- Changing the local-HTTP cleanup path's behavior (issue 121 territory).

## 5. Verification

- Canary probe (`docs/Canary.md`): confirm `agy --print --model
  gemini-3.7-flash-<tier> --prompt "..."` works end-to-end from a plain
  shell before writing any Voxi integration code.
- Extend `docs/studies/2026-09-13-llm-cleanup-evaluation.md`-style
  evaluation (see `cleanup_eval_test.go`'s pattern) with a Gemini-backed run
  covering the same test cases (literal commands/questions preserved as
  text, YAML-looking multiline speech, ordinary typo correction, an
  already-applied replacement left alone).
- Confirm fallback behavior (`fallback_reason` taxonomy from issue 115) is
  preserved for the new backend — timeout, connection/process-spawn error,
  invalid/empty output must all degrade to the pre-LLM transcript exactly
  like the local-HTTP path does today.

## 6. Sprint checkpoint (2026-09-14)

The opt-in `agy` subprocess path, configuration, settings, diagnostics, docs,
and fallback tests are implemented. `go test ./...` and `go vet ./...` pass;
`make restart-service` rebuilt, installed, and restarted the live agent.

The CLI canary confirmed that the installed `agy` requires `--print=<prompt>`.
One-shot Gemini 3.7 Flash low calls took about 9.1 seconds in text mode,
7.3 seconds in JSON mode, and 4.5 seconds in stream-JSON mode. The real Voxi
evaluation exercised all five transcript cases; all five hit the 1.5-second
deadline and correctly fell back to the original transcript. No successful
cleanup was observed through Voxi, so this issue is not complete.

Next: establish and test a viable persistent `agy` stream-JSON session (or
another measured design that meets the eager deadline), then rerun the same
five cases and require successful cleanup before closing this ticket.
