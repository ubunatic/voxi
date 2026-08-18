# Case Study: GNOME Shell Voice Input Companion & Devkit Testing Harness

**Date**: 2026-08-18  
**Scope**: GNOME 45–50 companion extension, live nested devkit canary testbed, Mutter window focus coordination, and zero-leak recording toggle  
**Feature Issue**: [Issue 022: GNOME transcriber UI](../../issues/022-gnome-transcriber-ui.md)  
**Status**: Implemented, verified in nested session, committed to `main` (`c4b4465`)

---

## 1. Header & Context

Following the headless voice-input foundation established in Issue 020 (systemd user daemon, local Whisper/Parakeet transcription, and `dotoold` input injection), Issue 022 introduces a lightweight graphical companion surface:
- Top-bar status indicator with dynamic recording and idle icons.
- One-click instant stop on active recording indicator.
- Fast dictation mode toggle (Batch Whisper vs. Streaming Parakeet).
- History popup with instant clipboard copy and Mutter-coordinated window retyping.
- In-place typing delay configuration slider (`type_delay_ms`).

Developing GNOME Shell extensions typically carries high developer friction: testing a single JS change often requires logging out of the desktop session or restarting GNOME. This session established a headless/nested development testbed using GNOME 50's `mutter-devkit` architecture and built the companion extension end-to-end.

---

## 2. Executive Summary

- **GNOME Shell Companion Extension** (`contrib/gnome-shell-extension/`):
  - Target: ESM GNOME Shell 45–50 (`metadata.json`, `extension.js`, `stylesheet.css`).
  - Implements top-bar indicator button, popup menu with mode selector, typing delay slider with presets (`0ms`, `15ms`, `30ms`, `60ms`, `100ms`), and recent transcript list.
  - Zero-lag optimistic UI transitions and background file polling (`/run/user/1000/voxtype/state`).
- **Canary Tooling & Testbed** (`scripts/canary_ext/` & `scripts/canary_nested/`):
  - Isolated D-Bus session (`dbus-run-session`) running nested GNOME Shell (`gnome-shell --devkit --wayland`).
  - Automatic symlink management, live error streaming to stdout, interactive notification trigger, and reliable process-group cleanup on exit.
- **Go CLI Recording Primitives & Tests** (`internal/tools/voice_record.go`):
  - Subcommands `harnez tools voice-input record {toggle, start, stop, status}`.
  - Full table-driven unit tests.

---

## 3. What Worked Well

1. **Decoupled 4-Tier Architecture**:
   - The extension relies solely on the `harnez` CLI as its integration surface rather than coupling directly to Voxtype or kernel uinput sockets.
2. **Mutter Focus Window Coordination**:
   - To prevent focus race conditions when retyping from the popup menu, the extension captures `global.display.get_focus_window()` on menu open. On click, it closes the popup, reactivates the target window, awaits focus confirmation, and only then triggers `dotool` injection.
3. **Canary-First Fast Iteration**:
   - The nested shell canary (`scripts/canary_ext/main.go`) enabled rapid debugging of GJS JavaScript runtime quirks in seconds without disrupting the developer's primary desktop session.

---

## 4. Honest Post-Mortem (Failures, Bugs & Near-Misses)

### 1. The GJS C-Binding Callback Signature (`TypeError: At least 3 arguments required`)
- **Mistake**: Called `proc.communicate_utf8_async(null, cancellable)` expecting a standard modern Promise return.
- **Root Cause**: GJS exposes GIO C async bindings requiring `(stdin, cancellable, callback)` with a separate `communicate_utf8_finish(res)` call.
- **Fix**: Wrapped `communicate_utf8_async` in a clean Promise with proper 3-argument signature.

### 2. GJS Tuple Return Destructuring (`TypeError: stdoutBytes.trim is not a function`)
- **Mistake**: Destructured `const [stdoutBytes, stderrBytes] = await ...` from `communicate_utf8_finish`.
- **Root Cause**: In GJS, `communicate_utf8_finish` returns `[ok, stdoutString, stderrString]` where element 0 is a boolean status flag. Attempting `.trim()` on `ok` caused silent subprocess failures.
- **Fix**: Corrected destructuring to `const [ok, stdoutStr, stderrStr] = ...` and validated with `gjs` runtime script.

### 3. Missing Virtual Method Override (`Class StWidget doesn't implement event`)
- **Mistake**: Attempted to override `this._indicator.vfunc_event = ...` on the instantiated button object.
- **Root Cause**: In GJS GObject bindings, virtual methods cannot be dynamically monkey-patched on instances.
- **Fix**: Cleanly intercepted `this._indicator.menu.toggle()` to achieve the 1-click stop behavior without messing with internal widget vfuncs.

### 4. Forgetting `make install` After Go Code Changes
- **Mistake**: Added new CLI subcommands (`record status`) in Go, but forgot to rebuild the local binary in `~/go/bin`, causing the extension to fail with "unknown command".
- **Fix**: Rebuilt binary with `make install` and codified the rule into `AGENTS.md` to ensure automatic adherence.

---

## 5. Quality & Invariants Audit

| Invariant / Metric | Status | Evidence / Assessment |
| :--- | :--- | :--- |
| **Layer Separation** | Passed | Extension only invokes `harnez` CLI; no hardcoded voxtype dependencies in UI layer. |
| **Idempotency** | Passed | Repeated canary runs create and tear down symlinks and temp dirs cleanly. |
| **Lint & Static Checks** | Passed | Added `node -c` and `gnome-extensions pack` checks to `scripts/lint.sh`. |
| **Unit Test Coverage** | Passed | Table-driven tests for recording control and status in `voice_record_test.go`. |
| **Process Teardown** | Passed | Process-group SIGKILL (`-pgid`) prevents orphaned nested D-Bus/shell processes. |

---

## 6. Key Learnings & Evergreen Upstream

1. **GJS Subprocess Pattern**:
   Always use the 3-argument callback pattern with `[ok, stdout, stderr]` destructuring for GJS subprocess execution.
2. **GNOME 50 Nested Development**:
   GNOME 50 requires `/usr/libexec/mutter-devkit` to render graphical nested test windows under `--devkit`.
3. **Always Run `make install`**:
   Whenever Go CLI primitives are added or modified, run `make install` immediately to update `~/go/bin/harnez`.

---

## 7. File & Diff Summary

- **Created**:
  - `contrib/gnome-shell-extension/extension.js`
  - `contrib/gnome-shell-extension/metadata.json`
  - `contrib/gnome-shell-extension/stylesheet.css`
  - `internal/tools/voice_record.go`
  - `internal/tools/voice_record_test.go`
  - `issues/024-gnome-typing-feedback-icon.md`
  - `issues/025-voice-input-volume-animation.md`
  - `scripts/canary_ext/main.go`
  - `scripts/canary_nested/main.go`
- **Modified**:
  - `internal/tools/command.go`
  - `internal/tools/tools.go`
  - `docs/VoiceInput.md`
  - `docs/VoiceInputArchitecture.md`
  - `issues/022-gnome-transcriber-ui.md`
  - `issues/README.md`
  - `scripts/lint.sh`
  - `AGENTS.md`
