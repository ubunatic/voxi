# 107 — Interactive Settings TUI for Feature Toggles and Configuration

**Status**: Closed
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Feature
**Related**: [106 LLM transcription cleanup defaults](106-configure-llm-transcription-cleanup-service-defaults-for-lmcoder-integration.md), [028 LLM cleanup hook](028-post-process-local-llm-cleanup.md), [043 Assistive config summary](043-assistive-config-summary-command.md), `internal/config/config.go`, `cmd/voxi/main.go`

---

## 1. Problem & Motivation

Configuring Voxi currently requires managing settings across multiple touchpoints:
- Editing `~/.config/voxtype/config.toml` (e.g. `type_delay_ms` via `voxi config set`).
- Specifying CLI flags on daemon launch (e.g. `--model`, `--history`).
- Configuring environment variables in `~/.config/voxi/env` or unit files (e.g. `OPENAI_BASE_URL`, `VOXI_CLEANUP_MODEL` for LLM transcription cleanup).

There is no interactive, single-pane mechanism for users to inspect active settings, toggle features like the LLM post-processing cleaner, switch ASR models, or tune typing delays.

## 2. Technical Specification

### 2.1 Command & Interface
Add a top-level command:
```bash
voxi settings
```

- **Renderer**: Uses `golang.org/x/term` in raw terminal mode to render a clean, lightweight curses-free menu (matching the dependency-free philosophy used in `voxi monitor`).
- **TTY Detection**: If stdout is not a terminal, `voxi settings` outputs current settings as formatted text or JSON instead of launching the interactive loop.

### 2.2 Navigation & Controls
- `↑` / `k` and `↓` / `j`: Move selection between setting rows.
- `Space` / `Enter`: Toggle boolean switches or cycle through select options.
- `s`: Save and apply changes.
- `q` / `Esc` / `Ctrl+C`: Exit without saving (or confirm on unsaved changes).

### 2.3 Core Settings (v1 Scope)
Keep the initial catalog focused on essential user switches:

1. **LLM Cleaner**: `[x] Enabled` / `[ ] Disabled`
2. **Cleanup Model**: Cycle through configured options (e.g. `qwen3-4b-instruct-2507-q4`, `smollm3-3b-instruct-q4`, `none`).
3. **ASR Model**: `cohere-transcribe-03-2026` (default), `whisper-large-v3-turbo`, `small.en`, `base.en`.
4. **Typing Delay (`type_delay_ms`)**: `0ms` (default), `1ms`, `5ms`, `12ms`.
5. **Dictation History**: `[x] Enabled` / `[ ] Disabled`.
6. **Modifier Gating**: `[x] Enabled` / `[ ] Disabled` (`voxi-modifierd` safety).

### 2.4 Persistence & Service Application
- Persist settings to `~/.config/voxi/config.yaml` / `~/.config/voxi/env` and `~/.config/voxtype/config.toml`.
- Write atomically using temporary files and standard user file permissions (`0644`).
- Upon saving, provide feedback on required service restarts (or optionally signal/restart `voxi-agent.service` via `systemctl --user restart`).

---

## 3. Implementation Plan

1. **Config Layer (`internal/config`)**:
   - Define a unified `UserSettings` struct with reader/writer methods supporting `~/.config/voxi/config.yaml` and `.env` formats.
2. **TUI Menu (`internal/settings`)**:
   - Implement terminal state handling, ANSI clear/draw helpers, key event reader, and item state toggling using `golang.org/x/term`.
3. **CLI Integration (`cmd/voxi`)**:
   - Register `voxi settings` command with help documentation.
4. **Testing**:
   - Unit tests for settings serialization, menu navigation logic, key event handling, and atomic file updates.

---

## 4. Acceptance Criteria

- [x] `voxi settings` launches an interactive terminal menu when run in a TTY.
- [x] Users can toggle LLM cleaner, select models, adjust typing delay, and toggle history/modifier gating.
- [x] Saving writes changes atomically to user configuration files.
- [x] Non-TTY invocations gracefully dump current configuration without crashing.
- [x] All unit tests pass and `make check` succeeds.

---

## 5. Non-Goals

- Complex multi-tab windowing layouts or heavy TUI framework dependencies (e.g. bubbletea/tview).
- Live real-time audio waveform/spectrogram tuning within the settings screen (handled by `voxi monitor`).
