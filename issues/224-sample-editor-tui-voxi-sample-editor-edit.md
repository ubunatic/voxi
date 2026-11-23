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

## 4. Decision (2026-11-23)

- **Loom widget correction**: `ubunatic.com/loom v0.2.18` provides `loom.TextInput` (`NewTextInput`), not `loom.TextEntry`. Use `loom.TextInput` for inline single-line input (search filter, sample ID, transcript, comma-separated keyterms/jargon), with word-wrapped transcript display in the detail pane.
- **Mic recording & raw stdin**: `loom.Pane` owns terminal stdin in raw mode during the TUI session. Drive `devsample.CaptureUtteranceWithReader` with a controlled pipe/cancel signal rather than sharing live stdin directly, so `Enter`/`Space` in the TUI stops capture cleanly without stdin contention.
- **Testability & safety**: Inject store, chunk buffer, audio player, and mic recorder interfaces into the TUI model. All tests must use `t.TempDir()`, never touch `~/.local/share/voxi/samples/`, and use zero sleeps.
- **Two milestones**: Ship browse/edit/move/delete + CLI routing first (M1), then playback, add-from-mic/chunk, and docs sync (M2).

## 5. Milestones & Verification Plan

### M1: Store Helpers, CLI Routing & Core Browse/Edit/Move/Delete TUI
- Add `Store.UpdateKeyterms(id string, keyterms []string) error` (or `Update` helper preserving trimmed unique keyterms) in `internal/sample/store.go`.
- Update `internal/sample/command.go`:
  - `voxi sample edit` accepts `MaximumNArgs(1)`: `edit <ID>` retains `$VISUAL`/`$EDITOR` behavior; bare `edit` launches the `loom` TUI (with explicit non-TTY error if no interactive terminal is attached).
  - Add `voxi sample editor` subcommand (`ExactArgs(0)`) launching the same TUI.
- Implement core `loom.Widget` in `internal/sample/` (`editor.go`):
  - Purpose tabs (`All`, `dictation`, `noise`, `voice`) via `1`–`4` / `Tab`, `/` search filter (`loom.TextInput`), `↑`/`↓`/`j`/`k` selection, `★` badge for keyterms, `✓` badge for `voice` consent, detail pane.
  - `e` Edit mode (`loom.TextInput` for `Transcript` and `Keyterms`; preserves non-empty transcript requirement on `dictation` and `voice`).
  - `m` Move purpose (`dictation`/`noise`/`voice`) via `Store.Move`, enforcing non-empty transcript + explicit `[y]`/`[n]` own-voice consent confirmation via `Store.GrantConsent` when moving into `voice`.
  - `d`/`Delete` confirmation prompt calling `Store.Delete`.
- Unit tests (`internal/sample/editor_test.go`, `command_test.go`) covering `ConsumeKey`, `loom.Render`, TTY guard, CLI routing, and all store mutations in `t.TempDir()`.

### M2: Playback, Add Flows (Mic & Recent Chunks) & Docs
- `p`/`Space` audio playback off the key loop using `devsample.PlayerCommand` (and clean stop on re-press or quit).
- `a` Add flow:
  - Source choice: `[1] Record from mic` (controlled `devsample.CaptureUtteranceWithReader` + optional `TranscribeRawTranscript`, saving WAV + sidecar via `Store.Put`) or `[2] Import from recent chunks` (`chunks.Buffer.List(true)`, `Buffer.ReadWAV`, prefilled transcript and `devsample.SuggestKeyterms`).
  - `loom.TextInput` prompts for `ID`, `Purpose`, `Transcript`, `Keyterms`, plus own-voice consent when adding to `voice`.
- Word wrapping/narrow-width handling in detail view; update `docs/SampleStore.md` (and check `website/man/index.html` / `voxi man` tests if applicable).
- Unit tests with fake player/recorder/chunk buffer in `t.TempDir()`, followed by `make lint`, `make test-short`, and `make install`.
