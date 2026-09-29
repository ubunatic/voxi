# 172 — voxi sample command set and feedback sample deprecation aliases

**Status**: Open — filed from 169
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: CLI
**Related**: 169, docs/SampleStore.md §5, depends on 170, 166, 168

---

## 1. Problem & Motivation

`voxi feedback sample` mixes sample handling into feedback rules; verbs `save-chunk`, `save-last`,
`promote`, `remove` are unclear. Decision in docs/SampleStore.md §5.

## 2. Technical Specification

- Top-level `voxi sample`: `list [--purpose] [--public] [--recent]`, `show`, `play`, `record [--purpose]`,
  `add --chunk N|--last`, `edit` ($EDITOR, 166), `move`, `delete`, `import`, `export --tsv`, `publish`.
  `merge` is added by 168 on top of this.
- `voxi feedback sample <verb>` becomes a hidden alias for one release, printing a deprecation line on
  stderr; `feedback import` drops the `samples` area in favour of `sample import`.
- `voice prepare|train|clone --samples-dir` -> `--store` (old flag hidden alias).

## 3. Implementation & Verification Plan

Command tests for every verb and alias (stderr deprecation text), `voxi man` regenerated, `make install`.
