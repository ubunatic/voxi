# 184 — Make R2T2 the default ASR model, without LLM cleanup

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Enhancement
**Related**: 075 (Cohere-default messaging, closed as superseded by this), 181 (R2T2 via lmcoder; §5 voxi bootstraps lmcoder), 134, 138 (R2T2 empty `language None` clips), `spec/models.yaml`, `internal/config/config.go`

---

/goal A fresh voxi install uses `r2t2-confucius4` as its ASR model with LLM cleanup off, and docs and
product messaging name R2T2 as the default. Stop and report when blocked on a user decision or a
denied permission.

## 1. Problem & Motivation
User decision 2026-10-06: R2T2 Confucius4 transcribes much better than Cohere Transcribe and its
output needs no LLM cleanup. The shipped default is still Cohere, so new users get the weaker model.

## 2. Findings (2026-10-06)
- `spec/models.yaml:13`: `default_model: cohere-transcribe-03-2026`.
- `llm_cleaner` is a user setting (`internal/config/config.go`, default `false`). The user's config
  already has `asr_model: r2t2-confucius4` and `llm_cleaner: false`.
- `r2t2-confucius4` is served by lmcoder's proxy (`127.0.0.1:8735`, x600-first on t14, see 181).
  On a machine without lmcoder, the default would point at a server that does not exist. The
  fallback entry `r2t2-confucius4-voxi` needs model files that voxi does not download.
- `r2t2-confucius4` has `requires_gpu: true` and no `cpu_fallback`. With lmcoder serving from an
  upstream host, the local machine may need no GPU at all.
- 075 (P1, Open) aligns docs and messaging with Cohere as the default; it must be rewritten for
  R2T2 or closed as superseded.

## 3. Decisions (user, 2026-10-06)
- Fresh install: `voxi install` installs lmcoder (181 §5) and uses R2T2 through it. Cohere stays
  selectable but is no longer the default.
- No CPU fallback for R2T2 for now; `requires_gpu` stays as it is.
- LLM cleanup: the global `llm_cleaner: false` default is enough; users can still turn it on.
  No per-model flag.

## 4. Plan
- Depends on 181 §5 (voxi bootstraps lmcoder); do that first or in the same change.
- Set `default_model: r2t2-confucius4` in `spec/models.yaml`.
- Update README, website and docs that name Cohere as the default (absorbs 075's scope).
- Verify: `voxi install` in the Podman install test (see 183) ends with lmcoder installed and R2T2
  configured, and `voxi settings test` passes on t14.
