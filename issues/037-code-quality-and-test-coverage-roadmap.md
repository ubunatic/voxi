# 037: Comprehensive Code Quality, Test Coverage, and Modularization Plan

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Moderate
**Category**: Code Quality / Tech Debt
**Related**: [036 Modularize monitor collector and TUI renderer](036-modularize-monitor-collector-and-renderer.md), [internal/eager/eager.go](../internal/eager/eager.go), [contrib/gnome-shell-extension/extension.js](../contrib/gnome-shell-extension/extension.js), [cmd/voxi](../cmd/voxi/)

---

## 1. Problem & Motivation

The `harnez assess voxi` analysis identified several maintainability, modularity, and test-ratio risks across the codebase:

```text
Feasibility: Attention Needed (elevated maintenance risks or token density)
Code (Go): 24 files, 5,429 LOC
Tests:     15 files, 1,259 LOC (~17% test ratio)
```

Key areas requiring attention include:
1. **Low Test Coverage in Critical Paths**: `cmd/voxi` contains 3 code files with no test coverage; `internal/audio` and `internal/asr` have thin coverage.
2. **Oversized Engine Monolith (`internal/eager/eager.go` >500 LOC)**: Bundles VAD segmentation, streaming state machines, typing injection, and feedback filtering.
3. **GNOME Shell Extension Monolith (`extension.js` >500 LOC)**: Mixes D-Bus communication, UI panel items, menu layout, and event handlers.
4. **Agent Context Token Weight**: Several documents (`docs/VoiceInput.md`, `issues/021-fluent-streaming-typing.md`) exceed 4k tokens and add unnecessary load when scanned by AI coding agents.

## 2. Technical Specification & Work Items

This umbrella improvement ticket tracks four focused initiatives:

### Work Item 1: `internal/eager/` Decomposition
- Separate VAD state coordination from typing dispatch and output emission.
- Extract synthetic typing / injection logic into `internal/eager/emitter.go` or `stream.go`.
- Keep public API contracts unchanged.

### Work Item 2: Test Ratio Improvement & CLI Test Suite
- Add CLI regression tests in `cmd/voxi/` covering flag validation, subcommand routing, and exit codes.
- Introduce mock/synthetic audio buffer tests for `internal/audio/` and `internal/asr/` to lift overall project test coverage above 25%.

### Work Item 3: GNOME Shell Extension Modularization (Deprioritized — P4)
- User does not currently use the GNOME Shell extension; the built-in OS recording indicator is sufficient. Defer until extension usage resumes.
- Split `contrib/gnome-shell-extension/extension.js` into modular GJS files:
  - `client.js`: D-Bus / IPC messaging with the daemon.
  - `indicator.js`: Panel status icon & animations.
  - `menu.js`: History & settings dropdown UI.

### Work Item 4: Agent Token Diet & Document Archiving
- Archive completed legacy design proposals (e.g. `issues/021-fluent-streaming-typing.md`) into `docs/studies/` or condense active issues.
- Streamline `docs/VoiceInput.md` by breaking out sub-topics into reference docs.

## 3. Verification Plan

1. Run `harnez assess voxi` and verify:
   - Go test ratio increases from 17% to $\ge 25\%$.
   - No Go or JS files trigger the `>500 LOC` warning.
2. Run `go test ./...` in `voxi` to ensure full test suite passes.
3. Run `make install` and verify CLI subcommands and continuous eager typing work without regression.
