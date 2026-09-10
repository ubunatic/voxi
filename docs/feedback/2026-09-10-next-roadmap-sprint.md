# Next Roadmap Sprint Retrospective

**Date:** 2026-09-10
**Scope:** 075, 088, 056, 096, 097, 037

## Outcome

The sprint completed the implementation-ready part of the next roadmap:

- 075 received a broad product-truth update across README, VoiceInput docs,
  telemetry guidance, CLI help, GNOME copy, model comments/schema, website
  source, and the historical 074 note. It remains open for a final repository-
  wide wording audit.
- 056 gained a shared safety rule rejecting punctuation-only ASR decodes and
  unit coverage; its gated hardware E2E test still needs live verification.
- 088, 096, and 097 remained research items with explicit canary/measurement
  dispositions. No scheduler tuning or unlicensed external corpus download was
  introduced.
- 037 remains parked until the current injection-safety work settles; its next
  slice is now bounded to eager control-plane extraction.

## Process findings

Parallel read-only advisors were useful for separating implementation-ready
work from research. The most important guardrail was refusing to infer a
scheduler fix from aggregate telemetry without load correlation. The main
friction was broad documentation drift: issue 075 spans user docs, source
comments, schemas, CLI copy, website copy, and historical notes, so future
passes should start with a repository-wide search and a defined supported-vs-
historical boundary.

## Verification

`go test -count=1 ./...`, `make check`, and targeted package tests passed. The
real Phase 2 stress test was not run because this environment lacks the gated
private WAV/ASR setup; its required live verification remains explicit in issue
056. Website source was updated but not published.
