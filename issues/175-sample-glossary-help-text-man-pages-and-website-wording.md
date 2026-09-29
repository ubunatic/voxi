# 175 — Sample glossary, help text, man pages and website wording

**Status**: Open — filed from 169
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
