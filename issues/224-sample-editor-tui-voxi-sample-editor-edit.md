# 224 — Sample editor TUI (voxi sample editor / edit)

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Feature
**Related**: 166, 172, 223 (Replacement categories)

---

## 1. /goal

Implement a `ubunatic.com/loom`-based TUI accessible via `voxi sample editor` and bare `voxi sample edit` (with no ID) to browse, play, add (mic recording + recent chunks), edit (via `loom.TextEntry`), move, annotate jargon, and delete samples in the private sample store — or stop and report when blocked on a user decision or denied permission.

## 2. Problem & Motivation

Managing private samples in `~/.local/share/voxi/samples/{dictation,noise,voice}/` (`docs/SampleStore.md`, `internal/sample/store.go`) currently requires separate CLI commands (`list`, `show`, `play`, `record`, `add`, `edit`, `move`, `delete`). A single interactive TUI makes curating dictation, noise, and voice samples fast and visual, and lets users annotate samples with their special jargon without letting benchmarked samples pass blindly through all replacement rules (#223).

## 3. Technical Specification

- **Entry points**:
  - `voxi sample editor` (alias) and `voxi sample edit` when invoked with no `ID` argument launch the TUI.
  - `voxi sample edit <ID>` preserves the existing `$VISUAL`/`$EDITOR` single-sample flow.
- **TUI Stack**:
  - Build as a `ubunatic.com/loom` application (matching `voxi settings` and `voxi monitor`, no new external TUI dependencies).
  - Use `loom.TextEntry` for inline text input (sample IDs, transcripts, keyterms/jargon annotations, search filter).
- **Layout & Core Flows**:
  - **Browse & Filter**: list/detail split with purpose tabs (`All`, `dictation`, `noise`, `voice`), `/` substring search, and audio playback (`p`/`Space`) via `internal/devsample`.
  - **Add (`a`)**: prompt to either **record from mic** or **import from recent chunks** (`internal/chunks`), enforcing explicit own-voice consent (`Consent` timestamp) when target purpose is `voice`.
  - **Edit (`e`)**: inline `loom.TextEntry` editing for `Transcript` and `Keyterms`/jargon annotations.
  - **Move (`m`)**: move sample across `dictation` / `noise` / `voice` (enforcing `voice` consent prompt).
  - **Delete (`d` / `Del`)**: confirmation prompt before removing both audio and JSON sidecar via `Store.Delete`.
  - **Jargon Annotation**: surface `Keyterms` / special jargon clearly in the detail and edit views so sample-based ASR evaluations can distinguish expected jargon from model-quality `repair` replacements (#223).

## 4. Implementation & Verification Plan

- Place TUI state, rendering, and key-handling in `internal/sample/` with unit tests using `t.TempDir()` (never touching `~/.local/share/voxi/samples/`).
- Run `make lint`, `make test-short`, and `make install` to verify.
