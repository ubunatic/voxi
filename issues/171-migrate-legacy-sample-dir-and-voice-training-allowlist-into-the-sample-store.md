# 171 — Migrate legacy sample dir and voice-training allowlist into the sample store

**Status**: Open — filed from 169
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Migration
**Related**: 169, docs/SampleStore.md §7, depends on 170

---

## 1. Problem & Motivation

Existing `~/.config/voxi/samples/` (29 WAVs, `corpus.tsv`, `voice-training.txt`) must move into the
new store without data loss. Decision in docs/SampleStore.md §7.

## 2. Technical Specification

- **One-shot, no compatibility layer** (user decision 2026-09-29: only this machine needs it). `voxi sample migrate [--dry-run]`, run by hand once; no automatic trigger, no `MIGRATED` marker.
- Purpose: allowlisted -> `voice`; empty or `[...]` transcript -> `noise`; else `dictation`.
  Print the plan; the user reviews it (known edge cases: `artifact-keyboard-smash`; decided: `kt-sentences-plus-*` are `dictation`, see below).
- Copy, verify size + SHA-256, leave legacy files in place until the user deletes them. A different existing target is an error. Modes 0600 (the two 0644 files are already fixed by hand).
- After the user confirms the live run: delete `LoadLegacyTSV`, `ExportTSV`, `voxi sample migrate` and the old `internal/devsample` store code, and point `scripts/clack_features` and `scripts/speech_context_bench` at the store (`List`). No reverse export.

## 3. Implementation & Verification Plan

Tests with a fake legacy dir in `t.TempDir()`: plan, hash conflict, modes.
Live run on the dev machine only with `--dry-run` first and user approval of the plan.

## Decision (user, 2026-09-29)

`kt-sentences-plus-*` samples are real-world speech affected by noise, kept for development and testing of dictation. Purpose is `dictation`; speech in them must be detected. Imperfect dictation samples are expected and stay `dictation`, not `noise`. Do not ask again.
