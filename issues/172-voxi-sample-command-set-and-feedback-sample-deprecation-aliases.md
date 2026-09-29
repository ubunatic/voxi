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
- **No compatibility layer** (user decision 2026-09-29: this is the only machine running current Voxi).
  Remove `voxi feedback sample` outright, drop the `samples` area from `feedback import` in favour of
  `sample import`, and rename `voice prepare|train|clone --samples-dir` to `--store`. No aliases, no deprecation text.

## 3. Implementation & Verification Plan

Command tests for every verb; a test that the removed `feedback sample` command is gone; docs and help mention only the new names; `voxi man` regenerated, `make install`.
