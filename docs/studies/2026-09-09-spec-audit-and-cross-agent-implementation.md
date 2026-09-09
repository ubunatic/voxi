# Spec/Code-Quality Audit, Roadmap Reconciliation, and Cross-Agent Implementation & Review

**Date:** 2026-09-09
**Feature issues:** [090](../../issues/090-spec-drift-monitor-w-section-flag-aliases-hardcoded-separately-from-spec-actions-yaml.md)-[092](../../issues/092-eagersessionmanager-toggle-has-a-check-then-act-race-under-concurrent-sigusr1-socket-invocation.md) (audit findings), [093](../../issues/093-collapse-an-immediately-repeated-trailing-sentence-clause-in-eager-transcripts.md)-[094](../../issues/094-collapserepeatedtrailingclause-wrongly-deletes-a-legitimate-short-answer-that-matches-the-question-s-last-word.md) (implementation + cross-review), [095](../../issues/095-normalize-spoken-number-words-to-digits-in-dictated-transcripts-library-vs-build-our-own.md) (follow-on feature request)
**Architecture reference:** [Spec.md](../Spec.md), [Roadmap.md](../Roadmap.md)

## Context

A single long session covering four distinct threads: a spec-driven-architecture and Go
code-quality audit, a roadmap reconciliation pass, a small ASR post-processing fix implemented and
reviewed via two different external coding-agent CLIs instead of a same-vendor subagent, and two
follow-on ASR quality requests surfaced by the user while dictating.

## Part 1: The spec/code-quality audit — verify claims, don't trust a fork's self-report

Asked to review the spec-driven approach (`docs/Spec.md`'s stated invariants) and general Go code
quality, "no nitpicks, real findings only." First dispatched a background fork to do the full
audit — it completed, but the task-notification result field twice returned a placeholder ("still
running") instead of its actual findings. Resuming it via `SendMessage` made it *worse*, not
better: on the second resume it treated itself as a *separate* observer waiting on "the audit
fork," apparently because a fork inherits the parent's full conversation context — including the
parent's own belief "I dispatched a fork and am waiting for it" — and a resumed fork can latch onto
that framing instead of recognizing itself as the fork being addressed.

Rather than fight the confusion, the audit was finished directly (no further forking) using
targeted reads and greps guided by one real finding the fork surfaced before losing the thread.
Three real, verified findings resulted:

- **090** — `internal/monitor/monitor.go`'s `ParseSections` (the `voxi monitor -w` flag parser)
  hardcodes its own alias table instead of resolving through `spec.LoadActions()`, in violation of
  `docs/Spec.md`'s own "don't shadow the spec" rule — confirmed by diffing the two vocabularies
  line by line.
- **091** — `make validate-spec` is just `go test ./spec/...`; no dependency anywhere in the module
  actually validates YAML against `spec/schemas/*.schema.json`, despite `docs/Spec.md` describing
  schema validation as CI-enforced. Confirmed by grepping for `jsonschema` imports (none) and
  `schema` references in `spec/*_test.go` (none).
- **092** — `eagerSessionManager.Toggle()` has a check-then-act race, confirmed by tracing both call
  sites that can invoke it concurrently (the `SIGUSR1` signal handler and per-connection socket
  goroutines in `runEagerDaemon`), not just reading the function in isolation.

**Lesson**: a subagent's "I finished, here are my findings" is not self-verifying, and neither is a
task-notification's summary line — both here turned out to be placeholder/confused text rather
than the real content, discovered only by actually looking. The eventual fix wasn't retrying the
fork; it was doing the verification work directly once the fork's signal became unreliable.

## Part 2: Roadmap reconciliation — a dispatched agent's judgment call, held open for a human

A separately dispatched (fresh, not forked) Opus agent updated `docs/Roadmap.md` against the
current open backlog. It found a legitimate merge candidate (052's CPU/GPU-priority research folded
into 088's more concrete, incident-driven ticket) but explicitly declined to act on it — "tracker
decision required — not taken here" — despite having read/write access. That distinction (a
roadmap synthesis pass should recommend, not silently restructure the tracker) held up under
scrutiny: when asked to execute on it, 037's park note stayed scoped to only the one work item the
roadmap had actually flagged, rather than closing the whole four-item umbrella ticket a more
literal reading of "close 037" might have done — items 1/2/4 (eager.go decomposition, test
coverage, doc archiving) were still real, untouched work with no relation to the park reason.

## Part 3: Cross-agent implementation and review — a second opinion caught what the first missed

For issue 093 (collapse an accidentally-duplicated trailing clause, e.g. `"Let's get started. get
started"` → `"Let's get started."`), implementation was handed to `codex exec` (model
`gpt-5.6-luna`) rather than a Claude subagent — a deliberate experiment. The result: a small,
correctly-scoped diff, positive/negative unit tests, `go vet`/`go test` passing — independently
re-verified rather than trusting codex's own "tests passed" summary. Reviewed inline and committed.

Then, as a second experiment, a **different** codex model (`gpt-5.6-sol`, read-only sandbox,
review-only prompt) was asked to review that same, already-merged code. It found a real bug that
had survived implementation, self-testing, *and* the inline human review:

```
"Was your answer no? No"  →  wrongly collapsed to  →  "Was your answer no?"
```

The collapse logic's punctuation-boundary anchor (added specifically to avoid deleting legitimate
mid-sentence repetition like "very very good") wasn't sufficient — it doesn't distinguish an
accidentally-duplicated *clause* from a genuinely independent short reply that happens to restate
the previous sentence's last word. Filed as 094, with a specific suggested fix (require the
collapsed suffix to be at least 2 words, since a 1-word echo is exactly the shape of a legitimate
short answer).

**Lesson**: for pattern-matching/heuristic string-processing code — where false positives are easy
to construct adversarially but easy to miss when writing your own positive-case tests — a second,
independent review from a *different* model than the one that implemented the change is worth
doing routinely, not just as a novelty. Same-agent inline review and the implementing agent's own
tests both missed this; a genuinely independent pass didn't. This finding is now folded into a
proposed `harnez` skill/CLI command (`/harnez-agent`, `harnez agent review` — filed in the `harnez`
repo, issues 288/291) so the pattern doesn't stay ad hoc.

A companion `agy` (multi-model CLI) trial on a trivial task surfaced pure CLI ergonomics friction
(a value-taking `-p` flag that silently swallows the next token unless written `-p="..."`) rather
than a capability finding — recorded for future reference but not a strong signal on `agy`'s
implementation quality either way.

## Part 4: Two more real findings from ordinary use, caught mid-conversation

While this work was in progress, the user reported two live dictation artifacts from ordinary use
of `voxi` (not from testing): an occasional stray "and" at recording startup, and the 093/094
double-phrase pattern. The first turned out to already have a built-in fix — `voxi feedback
silence-artifact add` matches a whole spurious utterance exactly, never embedded text, so it's safe
against false positives by construction; no new code needed. The second became 093/094 above.

A third, unrelated live report — spoken numbers ("two hundred eighty-eight") sometimes
transcribing as words instead of digits — became issue 095, scoped explicitly to avoid the same
false-positive class 094 just demonstrated (a naive word-swap would wrongly convert "the blue
**one**").

## Harness/process observations (not acted on — see main session report)

- A resumed fork can lose track of its own identity relative to the parent that spawned it,
  producing responses framed as the coordinator rather than the fork being addressed. Mitigation
  used here: stop resuming, do the work directly.
- The `harnez` repo's `issues lint --cached` pre-commit hook computes drift against the *staged*
  tree, so pre-existing unrelated untracked ticket files in that repo caused three separate
  false-positive lint failures this session — worked around each time by moving them aside,
  committing, then restoring them untouched. The underlying two draft tickets have been sitting
  uncommitted in that repo since 2026-09-08.
