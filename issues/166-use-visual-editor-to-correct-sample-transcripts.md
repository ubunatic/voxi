# 166 — Use $VISUAL/$EDITOR to correct sample transcripts

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Feature
**Related**: issue 164/165 session, internal/devsample/flow.go (`promptText`, `Record`, `SaveChunkAsSample`), internal/devsample/lineedit.go

---

## 1. Problem & Motivation

Correcting the ASR transcript of a recorded chunk (`voxi feedback sample save-chunk|save-last|record`)
happens in a single-line raw-mode editor. Long chunks wrap over several terminal rows and are
awkward to fix. Exact transcripts matter for voice cloning and training, so review should be easy.

/goal Open the user's editor for transcript correction by default when one is configured, with tests and `make install`, or stop and report when blocked on a user decision or denied permission.

## 2. Specification

- When stdin is a terminal and `$VISUAL` (else `$EDITOR`) is set, write the ASR text to a private temp file, run the editor attached to the terminal, and read the result back. Lines starting with `#` are ignored; a header comment reminds "type exactly what you said".
- Otherwise (no editor configured, piped stdin, or `VOXI_SAMPLE_EDITOR=off`) keep the current inline prompt unchanged.
- Empty result or non-zero editor exit: error, nothing written.
- Applies to `record`, `save-chunk` and `save-last` (shared prompt code).
- No `nano` fallback: without `$VISUAL`/`$EDITOR` behavior is unchanged.

## 3. Milestones

M1 (editor prompt): implement, unit tests with a fake editor, document in docs and `--help`, `make install`.
Re-verify against live code and recent commits before starting.
