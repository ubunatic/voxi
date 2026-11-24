# 225 — Clean up sample editor loom v0.16.0 widget workarounds after loom 458-461

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Refactor
**Related**: 224, [docs/SampleStore.md](../docs/SampleStore.md), `internal/sample/editor.go`, `internal/sample/editor_render.go`, `internal/sample/editor_media.go`, upstream `loom` `#458`–`#461`

---

## 1. Problem & Motivation

Commit `75d5388` upgraded `ubunatic.com/loom` from `v0.2.18` to `v0.16.0` and adopted `loom.TextEntry`, `loom.Split`, and `loom.Confirm` in the sample editor TUI (`internal/sample/`). Testing and inspecting `ubunatic.com/loom@v0.16.0` surfaced four upstream widget limitations (reported to `loom` and filed there as `#458`–`#461`), forcing local workarounds in `voxi` that should be removed once `loom` tags a release containing those fixes.

## 2. Technical Specification / Findings

Local workarounds currently active against `ubunatic.com/loom v0.16.0` (`go.mod`):

1. **`loom.Confirm` navigation & prompt clipping (`loom#458`)**:
   - `internal/sample/editor.go` (`beginConfirm`, `consumeConfirmKey`) passes `""` to `loom.NewConfirm` and checks `e.confirm.Answered()` directly because `v0.16.0` `Confirm.ConsumeKey` returns `Ignored` on `left`/`right`/`h`/`l` even while toggling selection.
   - `internal/sample/editor_render.go` (`drawConfirm`) wraps `e.status` via `wrapText` above `e.confirm.Draw` because `v0.16.0` `Confirm.Draw` does not clip or wrap long prompts.
2. **`loom.TextEntry` `Tab` consumption (`loom#459`)**:
   - `internal/sample/editor.go` (`editKey`) and `internal/sample/editor_media.go` (`addFormKey`) intercept `"tab"` and `"shift-tab"` before delegating to `TextEntry.ConsumeKey`, avoiding `v0.16.0` eating `Tab` when an unfocused/empty `Placeholder` path runs.
3. **`loom.Form` widget interoperability (`loom#460`)**:
   - Edit and add forms (`drawEditForm`, `editKey`, `addFormKey`) stay custom instead of adopting `loom.Form` because `v0.16.0` `Form` does not support `*loom.TextEntry` in `values()` and mouse height calculations.
4. **`loom.SearchBar` limitations (`loom#461`)**:
   - Live search (`beginSearch`, `searchKey`, `drawSearch`) uses a single-line `loom.TextEntry` instead of `loom.SearchBar` due to missing long-query clipping vs `Controls`, caret navigation, and optional `cmdBar`.

## 3. Implementation & Verification Plan

1. Wait until upstream `ubunatic.com/loom` releases fixes for `#458`–`#461`.
2. Bump `ubunatic.com/loom` in `go.mod` / `go.sum`.
3. Simplify `beginConfirm` / `consumeConfirmKey` / `drawConfirm` to pass the prompt directly to `loom.NewConfirm` and rely on `EventResult.Consumed` / built-in prompt layout, and re-evaluate adopting `loom.SearchBar` and `loom.Form` (or removing pre-`Tab` guards if no longer needed).
4. Re-run `go test -race -count=1 ./internal/sample/...`, `make lint`, `make test`, `make install`, and a live forked PTY check of `voxi sample editor`.
