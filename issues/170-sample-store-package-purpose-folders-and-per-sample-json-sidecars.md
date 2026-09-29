# 170 — Sample store package: purpose folders and per-sample JSON sidecars

**Status**: Open — filed from 169
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Architecture
**Related**: 169, docs/SampleStore.md §3–§4, supersedes 099, 119

---

## 1. Problem & Motivation

Samples live in one flat dir with a fragile `corpus.tsv` manifest (099) and one allowlist file as the
only guard against training on dev samples. Decision in docs/SampleStore.md §3–§4.

## 2. Technical Specification

- New package `internal/sample` (replaces the store part of `internal/devsample/sample.go`):
  root `$XDG_DATA_HOME/voxi/samples` (default `~/.local/share/voxi/samples`), purpose folders
  `dictation/`, `noise/`, `voice/`, one `<id>.json` sidecar per audio file.
- Sidecar fields: `id`, `transcript`, `keyterms`, `created`, `source`, `consent` (voice only).
- API: `Open(root)`, `List(purpose...)`, `Get(id)`, `Add`, `Delete`, `Move(id, purpose)`; IDs unique
  across purposes; atomic temp+rename writes; private root 0700/0600, public root 0755/0644.
- Legacy read: `LoadLegacyTSV(dir)` for migration (171) and `ExportTSV` for scripts.
- Switch `scripts/speech_context_bench` and `scripts/clack_features` to the store reader.

## 3. Implementation & Verification Plan

Unit tests in `t.TempDir()`: round trip, duplicate id across purposes refused, corrupt sidecar reported
per file without breaking listing, permissions, TSV export equals legacy format. `make install`.
Close 099 as superseded when this lands.

## M1 (store package) delivered, review findings

M1 delivered commit 3b362e1: `internal/sample` with purpose folders, sidecars, permissions, TSV load/export, tests pass, installed.

### Milestone 2 (review fixes and script switch): Pre-Work / Required Refinements

1. **Scripts not switched.** The spec requires `scripts/speech_context_bench` and `scripts/clack_features` to read via the store (`LoadLegacyTSV` until 171 migrates). Do it; keep their behaviour unchanged.
2. **`Move` is not all-or-nothing.** It renames the audio, removes the old sidecar, then writes the new one. If a later step fails, audio and sidecar end up in different purpose folders. Write the new sidecar first, then move the audio, and roll back on any failure (restore audio, remove the new sidecar). Add a failure-injection test (for example a read-only target dir).
3. **Missing tests:** move into `voice` without consent is refused and leaves everything untouched; move with consent succeeds and audio follows the sidecar; delete removes audio and sidecar; `Get` of an unknown id returns `os.ErrNotExist`; a sidecar whose `id` differs from its filename is reported.
4. **Real-data plausibility check (read-only):** run `LoadLegacyTSV` on the real `~/.config/voxi/samples` in a throwaway test or command (not committed, never write there) and confirm the row count equals the count of non-comment rows in `corpus.tsv`, keyterms split correctly, and no transcript is empty by accident. Report the two numbers.
5. Close 099 as superseded once done (`harnez issues close 099 "superseded by 170"`), after the above passes.
