# 178 — Find voxi's cleanup model on the lmcoder proxy instead of assuming 8734

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Enhancement
**Related**: 158, 106, 126; lmcoder issues 155 (server side) and 137

---

/goal voxi finds its transcript-cleanup model on lmcoder whenever it is
served and uses it without the user switching lmcoder's model or voxi's
config, and degrades to raw ASR with a clear message when it is not served;
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

## 3. Implementation & Verification Plan
- Before cleanup (cached briefly, so it adds no per-chunk latency), look the
  model up on the proxy's `/v1/models`; use it if live, otherwise fall back
  to raw ASR and record why (existing `llmFallback` reasons).
- `voxi settings check`: report "model served" vs "proxy reachable but model
  not served", naming the model and the lmcoder command that would serve it.
- Never load or switch lmcoder models from voxi.

Verify: with lmcoder serving voxi's model plus another model on a second
port, cleanup uses voxi's model; with voxi's model stopped, cleanup falls back
and `voxi settings check` says the model is not served.
