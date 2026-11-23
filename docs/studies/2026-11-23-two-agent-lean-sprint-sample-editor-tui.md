---
kind: sprint
issue: 224
date: 2026-11-23
---

# Two-Agent Lean Sprint: Sample Editor TUI (Issue 224)

## 1. Bounded Discovery (Searcher: `searcher-224`, `claude:claude-haiku-5-5`, 41s)

- **`ubunatic.com/loom v0.2.18`**: No `TextEntry` type exists; the text widget is single-line `loom.TextInput` (`NewTextInput(value string)`, `Value()`, `SetValue()`, `Caret()`, `ConsumeKey()`, `Draw(c, r, focused)`). `Pane` exposes `loom.New(height)`, `Run(root Widget)`, `RunWatch(...)`, `Invalidate()`, `Close()`, `RunStartup(...)` (no `Pane.SetScreenMode`). `loom.Render(root, cols, rows)` renders screenshots deterministically for tests.
- **`internal/sample/store.go`**: `Sample` already has `Keyterms []string` (`json:"keyterms,omitempty"`), `Purpose` (folder-based), `Consent *time.Time`. `Store` has `List`, `Get`, `Add`, `Put`, `UpdateTranscript`, `GrantConsent`, `Delete`, `Move`, `AudioPath` — no `UpdateKeyterms` helper yet.
- **`internal/sample/command.go`**: `edit ID` currently sets `Args: cobra.ExactArgs(1)` (`:165`); `voxi sample editor` does not exist yet.
- **`internal/devsample/` & `internal/chunks/`**: `CaptureUtteranceWithReader(ctx, d, in)` captures 16kHz PCM (`pw-record`/`arecord`, stopped on Enter or cancel); `devsample.PlayerCommand` plays audio; `chunks.DefaultBuffer()` provides `List(reverse)` and `ReadWAV(c)`; `devsample.SuggestKeyterms` and `TranscribeRawTranscript` exist.
- **`cmd/voxi/main.go`**: Already wires `sample.NewCommand(d)` at `:400`.
- **Docs**: `docs/SampleStore.md:41` is stale (omits `publish`/`merge` and needs `editor`/`edit` update).

## 2. Milestones

- **M1 (Store Helpers, CLI Routing & Core Browse/Edit/Move/Delete TUI)**: Delivered in `c65065a` (`internal/sample/store.go`, `command.go`, `editor.go`, tests). `make test-q1` passed across 40 packages (`ok ubunatic.com/voxi/internal/sample 1.090s`), `go test -race -count=1 ./internal/sample/...` passed 66 tests (`5.068s`), `make lint` exit 0 (66 pre-existing warnings), `make install` succeeded, and live forked PTY run (`voxi sample editor`, `q`) rendered tabs and exited 0. Reviewed by `rev-224-m1` (`agy:gemini-4-neon`, 1m10s, severity `none`).
- **M2 (Playback, Add Mic/Chunks & Docs)**: Delivered in `a3272d5` (`internal/sample/command.go`, `editor.go`, `editor_render.go`, `editor_media.go`, `editor_test.go`, `editor_media_test.go`, `docs/SampleStore.md`). Fixed post-unlock `e.pending` spawn queue (eliminating inline-test `e.mu` deadlock) and split `editor_render.go` (restoring lint baseline 66 issues). `make test-q1` passed across 40 `ok` packages (`internal/sample 1.087s`), `make install` succeeded, and live PTY run (`voxi sample editor`, `a` -> `Esc` -> `q`) passed exit 0. Reviewed by `rev-224-m2` (`agy:gemini-4-neon`, 1m03s, severity `low`: missing `defer cancel()` in spawned playback/record closures could leak child ctx / `defaultSampleRecorder` pipe goroutine on early error).
- **M3 (Context Cancel Cleanup & Final Verification)**: Delivered in `ba37629` (`internal/sample/editor_media.go`, `editor_media_test.go`). Added `defer cancel()` inside both spawned closures (`togglePlaybackLocked`, `startMicRecordingLocked`) and child context + LIFO pipe cleanup in `defaultSampleRecorder`, plus 4 deterministic inline-spawn cancellation tests. `QUOTA_BYPASS=1 make test` (`go vet ./...` + all packages `ok`, `internal/sample 1.091s`), `make lint` (66 baseline warnings), `make install`, and final live PTY check passed; reviewed by `rev-224-m3` (`agy:gemini-4-neon`, 42s, severity `none`).

## 3. Outcome & Efficiency

- **Agents used**: `searcher-224` (`claude-haiku-5-5`, 41s), `dev-224` (`claude-haiku-5-5`, M1 7m38s), `rev-224-m1` (`gemini-4-neon`, 1m10s), `dev-224-m2` (`claude-haiku-5-5`, M2 4m37s + 2m26s, M3 1m53s), `rev-224-m2` (`gemini-4-neon`, 1m03s), `rev-224-m3` (`gemini-4-neon`, 42s).
- **Discovery value**: `searcher-224` caught early that `ubunatic.com/loom v0.2.18` exposes `TextInput` (single-line) rather than `TextEntry`, and flagged raw-TTY stdin contention for `CaptureUtteranceWithReader`, preventing rework.
- **Process lesson**: An inline `e.spawn` test caught an `e.mu` deadlock on the M2 tree before commit, resolved cleanly via a post-unlock `e.pending` queue; splitting `editor_render.go` kept `editor.go` under the 800-line lint limit.
- **Issue 224**: Closed after M3 verification, review (`none`), and live PTY verification.
