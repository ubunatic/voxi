# 145 — Improve Markdown text normalization for TTS reading

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Feature
**Related**: 141 (monitor-gated TTS and `voxi say`), 143 (read selections aloud)

---

## 1. Problem

TTS currently reads Markdown syntax literally. A heading such as `### Results`
can be spoken as “hash hash hash Results”, and text containing `input/output`
may be read with an awkward slash. This makes Markdown documents and selected
Markdown text harder to listen to.

## 2. Goal

Normalize common Markdown and prose notation before text is split into TTS
chunks, so that the spoken output communicates the content rather than its
formatting marks.

## 3. Required behavior

- Recognize ATX headings beginning with one or more `#` characters followed by
  a space and a title, including forms such as `# Title`, `## Title`, and
  numbered headings such as `## 1. Title` (and other heading levels/numbers).
  Do not speak the heading markers or the heading number as formatting.
- Replace `&` with “and”.
- Expand `e.g.` to “for instance”.
- Expand `i.e.` to “meaning”.
- Replace `/` between two words with “and”, as in `input/output`. Preserve
  slashes between numbers, as in `3/4`.

## 4. Acceptance criteria

- Normalization produces readable spoken text for the examples above without
  reading Markdown heading markers aloud.
- Numeric slash expressions remain intact; ordinary path and URL slashes are
  not rewritten as conjunctions unless they match the specified word-between-
  words case.
- Unit tests cover heading levels, numbered headings, each replacement,
  numeric slash preservation, and text that should remain unchanged.
- The same normalization applies to text submitted through `voxi say` and
  selection/clipboard reading.
