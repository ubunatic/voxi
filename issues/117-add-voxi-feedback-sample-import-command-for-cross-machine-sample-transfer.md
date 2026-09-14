# 117 — Add voxi feedback sample import command for cross-machine sample transfer

**Status**: Closed — Implemented voxi feedback sample import <path> with tests and live verification
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Feature
**Related**: [042 private dev sample recorder](042-private-dev-sample-recorder.md), [099 replace corpus.tsv sample manifest with a more robust storage format](099-replace-corpus-tsv-sample-manifest-with-a-more-robust-storage-format.md)

---

## 1. Problem & Motivation

Dev samples (`internal/devsample`, `voxi feedback sample record|list|play|remove`)
live in the private, per-machine directory `~/.config/voxi/samples/`
(`corpus.tsv`-compatible manifest + `<name>.wav` files) and are deliberately
excluded from any Voxi-owned sync/backup tooling. There is currently no
command to move a sample set built up on one machine onto another — a
developer who records samples on a laptop and wants them available on a
desktop (or vice versa) has to manually `scp`/`rsync` the WAV files and
hand-merge `corpus.tsv` lines, which is error-prone around name collisions
and manifest field ordering.

## 2. Desired Design

Add `voxi feedback sample import <path>`, where `<path>` is a directory
(or manifest file) in the same `corpus.tsv` + `<name>.wav` layout that
`SamplesDir`/`PublicSamplesDir` already use (`internal/devsample/sample.go`).

Behavior to define during implementation:
- Merge the source manifest entries into `SamplesDir(home)`, copying each
  referenced WAV alongside it (mode `0600`, matching `record`'s existing
  file permissions).
- Name-collision policy: skip by default, with an explicit `--overwrite`
  (or per-name prompt) to replace an existing sample — consistent with
  `record`'s existing overwrite confirmation for a single name.
- Validate each imported entry the same way `record` does (WAV exists and
  is readable, manifest fields well-formed) before accepting it, so a
  partially-copied or corrupted source directory can't poison the local
  library.
- Decide whether import is transport-agnostic (operate on an already-copied
  local directory, leaving `scp`/`rsync`/USB transfer to the user) or
  should also handle the transfer itself — default to the former (simpler,
  no new dependency) unless a concrete cross-machine workflow need argues
  otherwise.

## 3. Out of Scope

- Any network/cloud sync mechanism — this is local-directory import only.
- Changing the manifest format itself (tracked separately in issue 099).
