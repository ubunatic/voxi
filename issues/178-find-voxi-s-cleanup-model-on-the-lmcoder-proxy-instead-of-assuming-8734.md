# 178 — Find voxi's cleanup model on the lmcoder proxy instead of assuming 8734

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Enhancement
**Related**: 158, 106, 126; lmcoder issues 155 (server side) and 137

---

/goal voxi sees all models lmcoder has loaded, with their capabilities and
speed, picks one for transcript cleanup per request without switching what
lmcoder serves, and degrades to raw ASR with a clear message when none fits;
stop and report when blocked on a user decision or denied permission.

## 1. Problem & Motivation
voxi sends cleanup requests to `http://127.0.0.1:8734/v1` with model
`qwen3-4b-instruct-2507-q4` (`internal/config/config.go`,
`internal/eager/eager.go` `cleanWithLLM`). Whatever model sits on 8734
answers. When lmcoder serves a different primary model, cleanup silently
uses the wrong model or the user has to switch lmcoder back. The user wants
voxi to find its model when lmcoder has it, with no switching.

## 2. Technical Specification / Findings
- Issue 158 already plans the routing part: call the proxy on 8735 by model
  name (`voxi` alias or `qwen3-4b-instruct-2507-q4`). This ticket adds the
  discovery and diagnostics on top; do 158 first or together.
- `voxi settings check` (`internal/settings/check.go`) only tests that
  `GET <base>/models` answers. It does not check that the cleanup model is
  listed, let alone live.
- lmcoder's `/v1/models` lists all spec models without a "live" flag today;
  lmcoder issue 155 adds a way to tell live models apart. Until then voxi can
  only check that the name is known.

## 3. Decisions (2026-10-03)
lmcoder keeps its main chat model as the default and, per lmcoder issue 155,
lists every loaded model on `GET :8735/v1/models` with an extra `lmcoder`
object: `live`, `default`, `capabilities`, `context` and measured `speed`
(generation and prompt tokens per second); `?live=1` lists only live models.
voxi chooses per request by model name. Choosing a model never changes what
lmcoder runs.

## 4. Implementation & Verification Plan
- Read `/v1/models?live=1` from the proxy, cached briefly so cleanup adds no
  per-chunk latency.
- Use the configured `cleanup_model` when it is live. Otherwise pick the
  fastest live model with the `chat` capability, and record which one was
  used in the cleanup record. With no live model, fall back to raw ASR with
  the existing `llmFallback` reasons.
- `voxi settings check`: list the live models with capability and speed and
  say which one cleanup will use.
- Never load or switch lmcoder models from voxi.

Verify: with the 4B main and the 1.5B fast model live, cleanup uses the
configured model; with `cleanup_model` set to a model that is not loaded, it
uses the fastest live chat model and the check says so; with lmcoder down it
falls back to raw ASR.
