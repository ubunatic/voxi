# 165 — voxi chunks delete

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Feature
**Related**: issue 053 (chunk ring buffer), internal/chunks/command.go

---

## 1. Problem & Motivation

`voxi chunks` can list, show and play recorded audio chunks (with their transcripts), but
not delete them. Private dictation audio should be removable on demand.

/goal Add `voxi chunks delete` with tests, installed via `make install`, or stop and report when blocked on a user decision or denied permission.

## 2. Technical Specification

- `voxi chunks delete [INDEX|last]` removes one chunk's audio and its transcript metadata; same INDEX/`last` addressing as `show` and `play`.
- `voxi chunks delete --all` removes every stored chunk.
- No arguments and no `--all`: error with usage (never delete implicitly).
- Interactive terminal: ask for confirmation for `--all`; `--yes` skips it.
- Out-of-range or empty store: clear error, non-zero exit; nothing else touched.
- Reuse the existing chunk store; audio and metadata must never be left half-deleted.
- Indices in `voxi chunks list` stay consistent after a delete.

## 3. Milestones

M1 (delete command): implement, cobra help/man text, unit tests (single, last, `--all`, bad index, empty store), `make install`.

Developer: re-verify against live code and recent commits before starting.
