# 190 — Prune voxi doctor pre-repair snapshots

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Feature
**Related**: 189, docs/AudioDoctor.md

---

## 1. Problem & Motivation
`voxi doctor --fix` saves a full `pw-dump` (~350 KB) to `~/.local/state/voxi/doctor/` before the
first repair of every run. Nothing removes them; five accumulated during the 189 live review alone.

## 2. Technical Specification / Findings
Snapshots are evidence for upstream reports and test fixtures, so keep recent ones.

## 3. Implementation & Verification Plan
- Keep the newest N snapshots (N in `spec/doctor.yaml`), delete older ones after a new snapshot is written.
- Test with `t.TempDir()`; update docs/AudioDoctor.md "Known Limits".
