# 123 — Retest LLM cleanup models against current YAML request format and 2.5s timeout

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Testing / Evaluation
**Related**: [docs/LLMTranscriptCleanup.md](../docs/LLMTranscriptCleanup.md), [docs/studies/2026-09-13-llm-cleanup-evaluation.md](../docs/studies/2026-09-13-llm-cleanup-evaluation.md), [internal/eager/cleanup_eval_test.go](../internal/eager/cleanup_eval_test.go), [Issue 121](121-llm-cleanup-timeout-adds-1-5s-dead-latency-per-chunk-under-cpu-load.md), [Issue 122](122-add-gemini-flash-3-7-support-as-llm-cleanup-model.md)

---

## 1. Problem & Motivation

The existing 5-case model evaluation (`docs/studies/2026-09-13-llm-cleanup-evaluation.md`,
`cleanup_eval_test.go`) is the only real-model evidence Voxi has for cleanup
quality (literal commands/questions preserved, YAML-looking multiline speech,
ordinary typo correction, an already-applied replacement left alone). Two
things have changed since that study that make its results stale:

- The request contract has since been hardened to the current YAML
  `transcript` + `chunk` (`mean_rms`, `peak_rms`, `applied_replacements`)
  shape documented in `docs/LLMTranscriptCleanup.md` — the 2026-09-13 study
  predates or only partially reflects this format (it already found the
  baseline flattened line breaks in YAML-looking speech; issue 112 tracks
  that specific fidelity gap as still open).
- The cleanup deadline was just raised from 1.5s to 2.5s
  (`llmCleanupTimeout` in `internal/eager/eager.go`, this session,
  2026-09-15) after live telemetry showed successful local-model cleanups
  routinely landing at 1,338-1,500ms — near-zero headroom under the old
  budget. The evaluation's timeout/fallback numbers were all measured
  against the old 1.5s bound.

## 2. Desired Verification

Rerun the same evaluation method against the current YAML request format and
the new 2.5s deadline, for both configured backends:

- `local_http` (`qwen3-4b-instruct-2507-q4`): confirm the near-miss timeouts
  observed live on 2026-09-15 (elapsed_ms 1,338-1,500 under real background
  CPU load) now complete reliably inside 2.5s, and re-check whether the
  extra budget changes cleanup quality/behavior at all (it shouldn't, since
  the model call itself is unchanged — only the deadline moved).
- `agy` / Gemini backend (issue 122): rerun the same 5 cases now that
  `LLMCleanupRecord.ElapsedMS` telemetry exists, to get real per-call timing
  instead of only pass/fail — issue 122's checkpoint found 100% fallback at
  the old 1.5s bound (9.1s/7.3s/4.5s one-shot latencies observed via CLI
  canary); confirm whether 2.5s changes that outcome at all (unlikely, given
  the measured latencies, but worth confirming with the new telemetry
  rather than assuming).
- Use `LLMCleanupRecord.ElapsedMS` (new field, this session) to report
  actual near-miss/far-miss margins for every case, not just success/fail,
  so future timeout tuning has real data instead of a single pass/fail
  signal.

## 3. Out of Scope

- Further timeout tuning beyond the 2.5s value already set — this ticket is
  about re-validating with what exists now, not iterating on the budget
  again immediately.
- Any new model/backend beyond the two already integrated
  (`local_http`/qwen and `agy`/Gemini).

## 4. Verification

- Extend or rerun `cleanup_eval_test.go`'s `TestRealCleanupEvaluation`
  against both backends; capture `elapsed_ms` for every case in the study
  update.
- Update `docs/studies/2026-09-13-llm-cleanup-evaluation.md` (or add a dated
  follow-up study) with the new numbers rather than leaving the stale 1.5s
  results as the only record.

## 5. 2026-09-15 checkpoint

The five-case harness was rerun twice against local Qwen and once against
`agy`/Gemini using the current YAML contract and 2.5-second deadline. Qwen
completed 10/10 calls (737–2,033 ms); Gemini timed out on 5/5 calls
(2,520–2,525 ms including process teardown). The multiline Qwen output kept
line breaks but added trailing spaces. Per-case results are in the linked
study. The harness now records the production cleanup result and classifies
timeouts from `FallbackReason`, not the former 1.5-second heuristic.

Remaining verification: repeat Qwen under controlled background CPU load
and from a cold model state before claiming reliable completion under those
conditions. This checkpoint used warm, sequential requests only.
