# 185 — Apply vocabulary replacements consistently to streamed transcripts

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Bug
**Related**:

---

## 1. Problem & Motivation
Configured feedback replacements do not appear to be applied consistently to recognized chunks. The reported examples include chunks transcribed as “news” and “Neos” while the replacement list contains `neos → neus` and `Neus → neus`. Determine why configured replacements are missed or produce unexpected casing, and make the resulting transcript follow the configured mapping.

**Goal**: Apply configured vocabulary replacements consistently to streamed transcripts and verify the reported examples, or stop and report when blocked on a user decision or denied permission.

## 2. Technical Specification / Findings
Observed with `voxi chunks list` and `voxi feedback replacement list | grep -i neus`. Other displayed chunks include “Neus” and repeated “Neos”; the intended canonical spelling/capitalization is not specified, so confirm it from the replacement semantics and surrounding behavior before changing mappings.

## 3. Implementation & Verification Plan
Trace how replacement entries are loaded and applied to streamed transcripts. Add focused coverage for the reproduced mismatch and verify that configured replacements are applied with the intended casing.
