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

- `voxi sample migrate [--dry-run]`, also triggered once automatically when the legacy dir exists
  and the new store does not.
- Purpose: allowlisted -> `voice`; empty or `[...]` transcript -> `noise`; else `dictation`.
  Print the plan; the user reviews it (known edge cases: `artifact-keyboard-smash`; decided: `kt-sentences-plus-*` are `dictation`, see below).
- Copy, never move; verify size + SHA-256; write `MIGRATED` marker in the legacy dir; leave legacy
  files in place. Idempotent (same hash skipped, different hash is an error). Fix modes to 0600
  (today `calm-reference.wav` and `voice-training.txt` are 0644).
- Reversible via `voxi sample export --tsv --to DIR`.

## 3. Implementation & Verification Plan

Tests with a fake legacy dir in `t.TempDir()`: plan, idempotent rerun, hash conflict, marker, modes.
Live run on the dev machine only with `--dry-run` first and user approval of the plan.

## Decision (user, 2026-09-29)

`kt-sentences-plus-*` samples are real-world speech affected by noise, kept for development and testing of dictation. Purpose is `dictation`; speech in them must be detected. Imperfect dictation samples are expected and stay `dictation`, not `noise`. Do not ask again.
