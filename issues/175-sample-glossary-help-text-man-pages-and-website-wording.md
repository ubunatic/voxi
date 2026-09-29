# 175 — Sample glossary, help text, man pages and website wording

**Status**: Closed — resolved
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Documentation
**Related**: 169, docs/SampleStore.md §1, depends on 172

---

## 1. Problem & Motivation

Users and agents cannot tell chunk, sample, corpus, promote, clone and training apart.

## 2. Technical Specification

- Glossary from docs/SampleStore.md §1 becomes the reference; link it from `voxi sample --help`,
  `voxi chunks --help`, `voxi voice --help` Long texts.
- Update README.md, docs/{ChunkDiagnostics,TTSReading,VoiceInput,ASREngines,LLMTranscriptCleanup,
  LiveMicMeter}.md, website/{index,dev/index,man/index}.html, `testdata/speech-context/README.md`.
- Rename the word "corpus" out of user-facing text; keep it only for the legacy TSV format.

## 3. Implementation & Verification Plan

`rg -n 'feedback sample|save-chunk|promote|corpus' docs website README.md` only shows intended hits;
man pages regenerated; website synced via `uman website sync voxi` after user go.

## Delivery (2026-09-29)

- `internal/glossary.Audio` (chunk, sample, purpose, publish, merge, voice) is the Long help of
  `voxi chunks`, `voxi sample` and `voxi voice`, and points to docs/SampleStore.md.
- Reworded: AGENTS.md Speech Samples, README.md config import (no samples area), docs/ASREngines.md,
  docs/EagerDeliverySafety.md, docs/SampleStore.md status, website/dev/index.html,
  testdata/speech-context/README.md. ChunkDiagnostics, TTSReading, VoiceInput, LLMTranscriptCleanup,
  LiveMicMeter and website/index.html had no outdated wording.
- `website/man/index.html` and the roff man page regenerated; no `feedback sample`, `promote` or
  `noise-samples` left.
- Remaining `corpus` hits are intended: the legacy TSV format, dated studies, and ASREngines history.
- Found on the way: the speech-context bench cannot read `testdata/speech-context/corpus.tsv` since 171
  → issue 176.
- Not done: website publish (`uman website sync voxi`) waits for the user's go.
