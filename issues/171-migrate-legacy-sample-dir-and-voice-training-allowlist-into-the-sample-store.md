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
- After the user confirms the live run: delete `LoadLegacyTSV`, `ExportTSV`, `voxi sample migrate`, `voxi sample export` and the old `internal/devsample` store code, and point `scripts/clack_features` and `scripts/speech_context_bench` at the store (`List`). No reverse export.

## 3. Implementation & Verification Plan

Tests with a fake legacy dir in `t.TempDir()`: plan, hash conflict, modes.
Live run on the dev machine only with `--dry-run` first and user approval of the plan.

## Decision (user, 2026-09-29)

`kt-sentences-plus-*` samples are real-world speech affected by noise, kept for development and testing of dictation. Purpose is `dictation`; speech in them must be detected. Imperfect dictation samples are expected and stay `dictation`, not `noise`. Do not ask again.

## M1 (migrate command) delivered; live run done

M1 delivered commit 4982deb. The live migration ran on the dev machine on 2026-09-29 with user approval: 29 samples copied and verified (24 dictation, 1 noise, 4 voice), all files 0600 in a 0700 store, legacy dir untouched, `voxi sample list` shows 29.

### Milestone 2 (legacy cleanup): Required work

1. Delete `LoadLegacyTSV`, `ExportTSV`, `PlanMigration`/`Migrate` and the `voxi sample migrate` command (and their tests), plus `sample export` if still present.
2. Point `scripts/clack_features` and `scripts/speech_context_bench` at the store (`Store.List`, audio via `AudioPath`), default root from `sample.Root`, keep their flags/behaviour otherwise. Read-only; run each once against the real store to confirm the 29 samples are found (report the counts).
3. Remove the old `internal/devsample` store code (`corpus.tsv` read/write, allowlist, promote/import) that has no remaining caller; keep shared helpers still used (capture, editor, transcribe, playback). List what remains and why.
4. `voice prepare|train|clone --store` must read from the store's `voice/` purpose instead of the legacy dir and `voice-training.txt` (only voice samples; the allowlist file is no longer read). Fix the flag help text accordingly. If this turns out larger than a small change, do items 1-3 and 5 and report exactly what is left for issue 173.
5. Never modify or delete anything under `~/.config/voxi/samples`; the user deletes the legacy dir by hand. Tests use `t.TempDir()`.

## Handoff (session wrap, 2026-09-29): M2 interrupted, tree does not build

The developer was stopped mid-way through M2 (legacy cleanup). The WIP is committed as-is so nothing is lost; it is **not finished and does not build**.

- Known breakage: `scripts/clack_features/main.go` has syntax errors from an interrupted edit (around lines 109-140, `loadPublicManifest` and following functions). `go build ./...` fails there.
- Seen in the tree (unverified): `internal/sample/migrate.go`, its test, and `internal/devsample/{audiofile,sample}*.go` deleted; edits in `internal/sample`, `internal/devsample`, `internal/tts/clone`, `internal/feedback/import_test.go`, both scripts.
- Live migration is DONE and verified (29 samples in `~/.local/share/voxi/samples`); the legacy dir is untouched and must stay so.

Resume point: finish M2 items 1-5 above. First `go build ./... && go vet ./...`, fix `clack_features`, then check each M2 item against the diff; run `make test-q1` once, `make install`. Do not close the ticket until the build and tests are green.
