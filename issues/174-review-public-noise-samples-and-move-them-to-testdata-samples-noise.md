# 174 — Review public noise samples and move them to testdata/samples/noise

**Status**: Open — filed from 169
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

- User decision needed: keep, trim or remove the two `bg-voice-*` files (removal from history is a
  separate, explicit step, not part of this ticket).
- `git mv testdata/noise-samples testdata/samples/noise`, write sidecars, drop `corpus.tsv`,
  update `.gitattributes`/LFS rules and every path reference (`clack_features`, docs).

## 3. Implementation & Verification Plan

`go test ./...`, `git lfs ls-files` shows all FLACs, `voxi sample list --public` lists them.
