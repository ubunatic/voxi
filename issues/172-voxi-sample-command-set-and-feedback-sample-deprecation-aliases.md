# 172 — voxi sample command set and feedback sample deprecation aliases

**Status**: In Progress — M1 removes legacy feedback/config entry points and adds store-backed core commands
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

M1 delivers `sample list|show|play|add|delete|export`, removes `feedback sample` and the
`config import` samples area, and renames the voice `--samples-dir` flags to `--store`.
Remaining: `record`, `edit`, `move`, store-to-store `import`, noise-only `publish`, public/recent
list modes, and their command tests. `merge` remains deferred to issue 168.

## M1 (command core) delivered, review findings

M1 delivered commit 930ccff: `voxi sample list|show|play|add|delete|export`, `feedback sample` and the `config import` samples area removed, `--samples-dir` renamed to `--store`; tests pass, installed.

### Milestone 2 (rest of the verbs and hardening): Pre-Work / Required Refinements

1. **Help text:** every `voxi sample` subcommand has an empty `Short` in `voxi sample --help`. Give each a one-line `Short` and a `Long` with an example; help mentions only new names.
2. **`--store` help text is wrong:** it still says "directory containing corpus.tsv and WAV files" but only the flag was renamed. Word it truthfully for now (it still points at the legacy directory until issues 171/173 switch voice commands to the `voice/` purpose); do not add behaviour here.
3. **`sample add`:** add `--purpose` (default `dictation`, `voice` refused with a message pointing to issue 173 until consent exists), `--chunk` and `--last` covered by tests using a temp chunk dir, and the $EDITOR transcript step from issue 166 (`internal/devsample/editor.go`, reuse, do not copy) with the same `VOXI_SAMPLE_EDITOR=off` opt-out.
4. **New verbs:** `record [--purpose]` (port the behaviour of the old `feedback sample record` onto the store), `edit ID` ($EDITOR, uses `UpdateTranscript`), `move ID PURPOSE` (refuse `voice` for now, same message). Then remove the now-unused old `internal/devsample` recording/save code that has no remaining caller (keep only what 171's migration and scripts still need; say what remains).
5. **`delete` with several IDs** must be all-or-nothing when one ID is unknown: check all IDs first, then delete.
6. **Tests to add:** `add --chunk N` and `add --last` (audio copied, sidecar `source` correct, mode 0600), unknown/duplicate ID, `play` (fake `Run`/`LookPath`), multi-ID delete with an unknown ID leaves everything, `voxi feedback sample` no longer exists, `edit` and `move` including refusal into `voice`.
7. **Not in this ticket (decided):** `import` (single machine, dropped), `publish` and public list mode (issues 173/174), `--recent`, `export --tsv` (removed together with the legacy reader by issue 171), `merge` (issue 168). Remove `export` from the command list and from the ticket spec.

