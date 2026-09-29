# 174 — Review public noise samples and move them to testdata/samples/noise

**Status**: Closed — resolved
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Privacy
**Related**: 169, docs/SampleStore.md §4, §7.6, §10, depends on 170

---

## 1. Problem & Motivation

`testdata/noise-samples/` is public and git-tracked. It holds `bg-voice-aye-you-did-that.flac` and
`bg-voice-hospital-hallucination.flac`, marked as distant background speech of unknown people, and
its `corpus.tsv` header still calls it "private local dev samples".

## 2. Technical Specification

- **Decided by the user (2026-09-29): the two `bg-voice-*` files stay public. Do not ask again.** Keep them
  in the move; no trimming or history removal.
- `git mv testdata/noise-samples testdata/samples/noise`, write sidecars, drop `corpus.tsv`,
  update `.gitattributes`/LFS rules and every path reference (`clack_features`, docs).

## 3. Implementation & Verification Plan

`go test ./...`, `git lfs ls-files` shows all FLACs, `voxi sample list --public` lists them.

## Delivery (2026-09-29)

- `git mv` of all 28 FLACs (including both `bg-voice-*`) to `testdata/samples/noise/`, one JSON sidecar
  each (0644, empty transcript, `created` from the old `#ts` lines, `source` names the old manifest);
  `corpus.tsv` removed. `.gitattributes` needed no change (`*.flac` rule); `git lfs ls-files` lists all 28.
- `scripts/clack_features` reads the public store through `internal/sample` (`noise/` only).
- `voxi sample list --public` lists all 28. Website/man page wording is left to 175.
