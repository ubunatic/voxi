# 158 — Call lmcoder proxy on 8735 by model name instead of raw 8734

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Enhancement
**Related**: lmcoder issue 137

---

## 1. Problem & Motivation
voxi transcript cleanup (`internal/config/config.go:46`) calls the lmcoder chat server on 127.0.0.1:8734 directly. Whatever model
sits on 8734 answers, and `lmcoder load` or idle eviction swaps or stops it
without voxi transcript cleanup (`internal/config/config.go:46`) knowing.

## 2. Technical Specification / Findings
lmcoder 137 (commits 0cd3ef6..67c4742, 2026-09-27) makes the proxy on 8735
route each request by its `model` field to the live local server holding
that model (secondary models run on ports 8736-8799). Verified live: model
`qwen3-4b-instruct-2507-q4` and alias `voxi` reach 8734, a second model on
8736 is reached by its name. The proxy must be started with a local backend
(`lmcoder proxy --backend-host local`).

## 3. Implementation & Verification Plan
- Point the default endpoint to http://127.0.0.1:8735 and send an explicit
  model name (`voxi` alias or `qwen3-4b-instruct-2507-q4`).
- Keep 8734 as a documented fallback override.
- Verify: with a second model loaded via `lmcoder load <m> --port auto`,
  voxi transcript cleanup (`internal/config/config.go:46`) still gets answers from its own model.
