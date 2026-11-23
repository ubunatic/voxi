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
- **M2 (Playback, Add Mic/Chunks & Docs)**: Dispatched to `dev-224` (resume).
