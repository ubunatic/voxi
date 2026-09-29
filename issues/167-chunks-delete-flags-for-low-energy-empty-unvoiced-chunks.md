# 167 — chunks delete: flags for low-energy, empty, unvoiced chunks

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Feature
**Related**: issue 165 (chunks delete), internal/chunks/command.go, internal/audio/audio.go (rejection reasons), docs/ChunkDiagnostics.md

---

## 1. Problem & Motivation

The 100 stored chunks contain many rejected ones (on the live machine: ~25 `low_energy_transient`,
~7 empty, ~3 `unvoiced_transient`). They are noise when hunting for chunks to promote to voice
samples. `voxi chunks delete` can only remove one chunk by index or everything.

/goal Add filter flags to `voxi chunks delete` that remove all chunks of the chosen categories, with tests and `make install`, or stop and report when blocked on a user decision or denied permission.

## 2. Specification

- New combinable flags on `voxi chunks delete`: `--low-energy` (rejection reason `low_energy_transient`), `--unvoiced` (`unvoiced_transient`), `--empty` (chunks whose outcome is empty in `chunks list`; the developer determines the exact field from live code and records it here).
- The flags select from all stored chunks, including the hidden older ones (as `--all` does).
- Mutually exclusive with `INDEX|last` and `--all`; error with usage otherwise.
- Confirmation on a terminal as for `--all` (state the matching count); `--yes` skips it.
- Output: `Deleted N chunks.`; when nothing matches, `No matching chunks.` and exit 0 without touching files.
- Reuse `Buffer.Delete` staging/rollback so audio and manifest stay consistent; no half-deleted chunks.
- Tests use temp buffers only; never touch `/run/user/*/voxi/chunks`.

## 3. Milestones

M1 (filter flags): implement, help text, tests (each flag, combinations, no match, conflicts, shadow chunks, count in prompt), `make install`.
Developer: re-verify against live code and recent commits before starting.

## 4. Review Notes

M1 (filter flags) delivered in 0f235c8: three combinable flags, shadow chunks included, atomic multi-index delete, tests pass.

### Pre-Work / Required Refinements (M1 follow-up)

Plausibility check on the live store (100 chunks): 23 `low_energy_transient`, 7 `empty`, 1 `unvoiced_transient`; all 31 have an empty `cleaned_transcript`. `--empty` currently matches on an empty transcript, so it would delete all 31, not just the 7 chunks that `voxi chunks list` shows as `(empty)`.

1. `--empty` must match `RejectionReason == "empty"` only (the category the user sees in `chunks list`). Each flag then selects exactly its own category.
2. Fix flag and help texts accordingly; update the `--yes` help to say it skips confirmation for `--all` and the category filters.
3. Add a test with chunks of all three reasons (all with empty transcripts) proving each flag deletes only its own category and combinations delete the union.
4. `make install`, commit as `fix(chunks): --empty matches the empty rejection reason only (issue 167)`.
