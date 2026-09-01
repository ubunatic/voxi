# 038: Vocabulary Feedback Command

**Status**: In Progress
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Feature
**Related**: [032 Small.en project vocabulary biasing](032-small-en-project-vocabulary-biasing.md)

---

## 1. Problem & Motivation

The opt-in speech-context feature reads persistent user terminology from
`~/.config/voxi/vocabulary.txt`, but users must currently edit that file by
hand. Voxi should offer the same reversible feedback workflow already used for
stop words and silence artifacts.

## 2. Desired Design

Add these commands:

```text
voxi feedback vocabulary add TERM
voxi feedback vocabulary list
voxi feedback vocabulary remove TERM
```

The commands manage the exact vocabulary file consumed by `voxi eager
--speech-context`. Terms use the speech-context sanitizer and spec-owned term
length limit, reject case-insensitive duplicates, support case-insensitive
removal, and list terms deterministically. Persistence must be atomic and keep
the file private to the user.

## 3. Acceptance Criteria

- Add, list, and remove operate on `~/.config/voxi/vocabulary.txt`.
- Invalid, empty, overlong, and duplicate terms produce clear errors.
- Stored terms are sanitized exactly as speech-context prompt terms are.
- Writes are atomic and result in a `0600` file in a private configuration
  directory.
- Unit tests cover command behavior, normalization, persistence, and failure
  cases.
- `go test ./...`, `make check`, `make install`, and `git diff --check` pass.
