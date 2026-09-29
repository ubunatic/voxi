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
