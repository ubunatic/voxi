# 172 — voxi sample command set and feedback sample deprecation aliases

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: CLI
**Related**: 169, docs/SampleStore.md §5, depends on 170, 166, 168

---

## 1. Problem & Motivation

`voxi feedback sample` mixes sample handling into feedback rules; verbs `save-chunk`, `save-last`,
`promote`, `remove` are unclear. Decision in docs/SampleStore.md §5.

## 2. Technical Specification

- Top-level `voxi sample`: `list [--purpose]`, `show`, `play`, `record [--purpose]`,
  `add --chunk N|--last [--purpose]`, `edit`, `move`, and `delete`.
  `merge` is added by 168 on top of this.
- **No compatibility layer** (user decision 2026-09-29: this is the only machine running current Voxi).
  Remove `voxi feedback sample` outright, drop the `samples` area from `config import`, and rename
  `voice prepare|train|clone --samples-dir` to `--store`. No aliases, no deprecation text.

## 3. Implementation & Verification Plan

Command tests for every verb; a test that the removed `feedback sample` command is gone; docs and help mention only the new names; `voxi man` regenerated, `make install`.

M1 delivers `sample list|show|play|add|delete`, removes `feedback sample` and the
`config import` samples area, and renames the voice `--samples-dir` flags to `--store`.
M2 adds `record|edit|move` and hardens `add|delete`; `import`, `publish`, public/recent list modes
and `merge` remain outside this ticket as described below.

## M1 (command core) delivered, review findings

M1 delivered commit 930ccff: `voxi sample list|show|play|add|delete`, `feedback sample` and the `config import` samples area removed, `--samples-dir` renamed to `--store`; tests pass, installed.

### Milestone 2 (rest of the verbs and hardening): Pre-Work / Required Refinements

1. **Help text:** every `voxi sample` subcommand has an empty `Short` in `voxi sample --help`. Give each a one-line `Short` and a `Long` with an example; help mentions only new names.
2. **`--store` help text is wrong:** it still says "directory containing corpus.tsv and WAV files" but only the flag was renamed. Word it truthfully for now (it still points at the legacy directory until issues 171/173 switch voice commands to the `voice/` purpose); do not add behaviour here.
3. **`sample add`:** add `--purpose` (default `dictation`, `voice` refused with a message pointing to issue 173 until consent exists), `--chunk` and `--last` covered by tests using a temp chunk dir, and the $EDITOR transcript step from issue 166 (`internal/devsample/editor.go`, reuse, do not copy) with the same `VOXI_SAMPLE_EDITOR=off` opt-out.
4. **New verbs:** `record [--purpose]` (port the behaviour of the old `feedback sample record` onto the store), `edit ID` ($EDITOR, uses `UpdateTranscript`), `move ID PURPOSE` (refuse `voice` for now, same message). Then remove the now-unused old `internal/devsample` recording/save code that has no remaining caller (keep migration, scripts, and shared capture/editor/playback helpers; say what remains).
5. **`delete` with several IDs** must be all-or-nothing when one ID is unknown: check all IDs first, then delete.
6. **Tests to add:** `add --chunk N` and `add --last` (audio copied, sidecar `source` correct, mode 0600), unknown/duplicate ID, `play` (fake `Run`/`LookPath`), multi-ID delete with an unknown ID leaves everything, `voxi feedback sample` no longer exists, `edit` and `move` including refusal into `voice`.
7. **Not in this ticket (decided):** `import` (single machine, dropped), `publish` and public list mode (issues 173/174), `--recent`, and `merge` (issue 168).

### M2 Delivered

All seven refinements are implemented. `sample add` and `record` support only `dictation`/`noise`
until issue 173 adds consent; `edit` persists through `UpdateTranscript`; `move` refuses `voice`.
Legacy `Record`/`SaveChunkAsSample` flows are removed. Shared capture, transcript/keyterm prompts,
editor, and playback helpers remain in `internal/devsample`; its legacy corpus parser/store helpers
remain for migration and current voice-training readers until issues 171/173 switch them.

Import, publish, public/recent listing, and merge remain deferred/out of scope.

## M2 (rest of the verbs) delivered, review findings

M2 delivered commit 9b95e3c: `record`, `edit`, `move`, help texts, hardening, tests; `feedback sample` gone; tests pass.

### Milestone 3 (noise samples): Pre-Work / Required Refinements

1. **Noise samples may have an empty transcript** (for example the existing no-speech sample `artifact-keyboard-smash`). `sample add`, `sample record` and `sample edit` currently reject an empty transcript for every purpose. Require a non-empty transcript only for `dictation`; allow it for `noise`. Tests for both.
2. `sample record --purpose noise` should not ask the transcript question at all when the ASR result is empty; go straight to keyterms/save with an empty transcript after one confirmation line.
3. Note in the ticket which `internal/devsample` legacy helpers remain and which issue (171) deletes them.

### M3 items 1–3 delivered

Empty transcripts are allowed for `noise` samples across add, record, and edit; `dictation` still
requires a transcript. When noise recording has no ASR result, it skips transcript editing, prints
one confirmation line, and continues to keyterms/save.

The legacy corpus API still present in `internal/devsample/sample.go` includes `SamplesDir`,
`ManifestPath*`, `WAVPath*`, `LoadManifest*`, `SaveManifest*`, `ParseManifest`, `FormatManifest`,
`SanitizeName`, `Find`, `Upsert`, and `RemoveEntry`. Existing TTS readers still use this layer;
issue 171 removes it after migrating the legacy corpus. Shared capture, transcript/keyterm, editor,
and playback helpers remain. Old flow helpers `Remove`, `Play`, `Import`, and `Promote` also remain
internally but are no longer exposed as CLI commands.
