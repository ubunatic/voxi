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
Delivered in `c65065a` (`feat(sample): add core sample editor TUI (issue 224 M1)`): `Store.UpdateDetails` + `NormalizeKeyterms`, `voxi sample editor` / bare `voxi sample edit` CLI routing with TTY guard, and core `loom` browse/search/edit/move/delete widget. Reviewed by `rev-224-m1` (`agy:gemini-4-neon`, severity `none`) and live-checked in a forked PTY.

### M2: Playback, Add Flows (Mic & Recent Chunks) & Docs
**Pre-Work / Required Refinements:**
- Pass `c.Context()` and `deps.Dependencies` through `runEditorCommand` -> `runSampleEditor` (without bloating `NewCommand` further).
- Wire off-key-loop runner callbacks (`spawn` + `pane.Invalidate()`) cleanly so background playback, mic recording, and optional ASR prefill refresh the UI thread-safely and cancel cleanly on `q` / `Ctrl-C`.

**M2 Scope:**
- `p`/`Space` audio playback off the key loop using `devsample.PlayerCommand` (and injected `Player` interface for tests); pressing again, moving, deleting, or quitting cancels active playback cleanly; missing audio reports cleanly in status.
- `a` Add flow:
  - Source choice: `[1] Record from mic` (off-key-loop `devsample.CaptureUtteranceWithReader` with controlled `io.Pipe()` + cancelable `ctx` so `Enter`/`Space` stops recording cleanly while `loom` owns raw stdin, optional non-fatal `TranscribeRawTranscript`, temp WAV written via existing `internal/sample` audio helper and removed after `Store.Put`) or `[2] Import from recent chunks` (`chunks.Buffer.List(true)`, chunk picker list, `Buffer.ReadWAV`, prefilled transcript + `devsample.SuggestKeyterms` if available).
  - Shared form via `loom.TextInput` for `ID` (validating format and uniqueness via `Store.Has`), `Purpose` (`dictation`/`noise`/`voice`), `Transcript`, `Keyterms`, plus own-voice consent prompt and non-empty transcript guard when adding to `voice`.
- Update `docs/SampleStore.md` (document `editor` / bare `edit` plus existing `publish`/`merge` commands).
- Unit tests with fake player/recorder/chunk buffer in `t.TempDir()` (zero sleeps, never touching `~/.local/share/voxi/samples/`), followed by `make lint`, full `make test-q1` (or `QUOTA_BYPASS=1 make test`), and `make install`.

Delivered in `a3272d5` (`feat(sample): add playback, mic/chunk import, and docs to sample editor (issue 224 M2)`): `p`/`Space` playback, `a` add flow (mic via controlled pipe + recent chunks via `chunks.Buffer`), `editor_render.go` split clearing the file-length lint warning, and `docs/SampleStore.md` update. Reviewed by `rev-224-m2` (`agy:gemini-4-neon`, severity `low`).

### M3: Context Cancel Cleanup & Final Verification
Delivered in `ba37629` (`fix(sample): cancel playback and recording contexts on completion (issue 224 M3)`): added `defer cancel()` inside both spawned closures (`togglePlaybackLocked`, `startMicRecordingLocked`) and wrapped `defaultSampleRecorder` with a child context + `defer cancel()`/`defer pr.Close()`/`defer pw.Close()`, plus 4 deterministic child-context cancellation regression tests. Reviewed by `rev-224-m3` (`agy:gemini-4-neon`, severity `none`). Verified with `QUOTA_BYPASS=1 make test` (`go vet ./...` + all packages `ok`, `internal/sample 1.091s`), `make lint` (66 pre-existing warnings, exit 0), and `make install`.
