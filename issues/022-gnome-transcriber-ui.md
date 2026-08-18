# GNOME transcriber UI: history, retype, and typing-speed controls

- **Status:** Complete — CLI plumbing, tests, canary probes, and GNOME Shell companion extension implemented


## Context

Issue 020 delivers headless voice-input dictation (a systemd user service plus a GNOME
custom keyboard shortcut, no visible UI). This issue tracks an optional GNOME Shell
extension that gives dictation a small, inspectable surface: recent transcripts, a way
to retype or copy any of them again, and a typing-speed control — without turning voice
input into a heavier, always-visible app.

This depends on issue 020's typing primitive (`dotool`/`dotoold`, see
`docs/VoiceInput.md`) and should reuse it rather than build a second injection path. It
is unrelated to issue 021's streaming-typing work except that both want a reusable
"type this text now" primitive; check issue 021's implementation before duplicating one
here. For the overall multi-tier design, see the ADR in [docs/VoiceInputArchitecture.md](../docs/VoiceInputArchitecture.md).


## Features

- **Typing speed**: expose Voxtype's existing `type_delay_ms` (delay between typed
  characters) as a UI control instead of requiring manual `config.toml` edits. Read the
  current value from `~/.config/voxtype/config.toml` (or the resolved `voxtype config`
  output) and write changes back the same way Voxtype's own `voxtype config set`
  subcommand does — preserving comments and other settings, per its documented
  behavior. Voxtype's daemon must be restarted for a changed value to take effect
  (documented limitation, not something this UI can work around); the UI should say so
  when it changes the setting, not simulate a live change that isn't real yet.
- **Recent transcripts**: keep a short, local, in-memory (or small on-disk) history of
  the last N transcriptions from this session, most recent first. No cloud sync; this is
  a convenience buffer, not a searchable archive, unless a later revision of this issue
  decides otherwise.
- **Copy button** per history entry: copies that entry's text to the clipboard. Simple,
  low risk — this already works today via Voxtype's own clipboard fallback path, so this
  is a thin UI wrapper, not new typing-injection logic.
- **Retype button** per history entry: re-types that entry's exact text into whatever
  application the user was in *before* opening this UI, as if it had just been
  dictated. This is the interesting one — see below.

## Why "retype" is the hard part

Clicking "retype" while the extension's own panel/popup has input focus means the
synthetic keystrokes would land in the extension's own UI, not the target application,
unless focus is returned to the target app first. This is the same class of problem
already hit twice in issue 020's canary: `eitype`/`ydotool`/`dotool` typing races
against Wayland compositor and device-registration timing when done naively.

A synchronous "close panel, hope focus returns instantly, then type" approach is not
robust — GNOME Shell UI teardown and window-manager focus restoration are not guaranteed
to complete before the next line of extension code runs. Two more robust directions to
evaluate:

1. **Capture-then-restore focus explicitly**, rather than relying on incidental focus
   return. Record the focused window (via Mutter's `Meta.Display` focus-window API, e.g.
   `global.display.get_focus_window()` at panel-open time) before showing the UI. On
   "retype," close the panel, then explicitly reactivate that captured window (e.g. via
   its `Meta.Window.activate()`), and only *after* GNOME Shell/Mutter confirms the focus
   change (a focus-changed signal, not a fixed sleep) invoke the typing primitive. A
   fixed `sleep()` before typing is exactly the kind of race that caused false "typed
   successfully" failures in issue 020's canary and should be avoided here from the
   start.
2. **Fall back safely when the captured window is gone** (closed, workspace switched,
   etc.) by disabling "retype" for that entry (or falling back to copy-only) rather than
   typing into whatever unrelated window happens to have focus. Silently typing
   dictation history into the wrong window is a worse outcome than a disabled button.

Whichever approach is chosen, add an explicit settle/confirmation signal before the
first synthetic keystroke, and log (at least at debug level) which window received the
retype, so failures are diagnosable the way issue 020's `journalctl` logs were essential
for debugging the original typing pipeline.

## Scope boundaries

- This is a GNOME Shell extension, GNOME-only; no cross-desktop abstraction is in scope
  unless a later revision of this issue says otherwise.
- Reuse the `dotool`/`dotoold` typing primitive from issue 020 as-is; do not reintroduce
  `eitype` (portal-dialog friction, explicitly rejected) or raw `ydotool` (no XKB layout
  awareness, wrong characters on non-US layouts — see `docs/VoiceInput.md`'s "Security
  note" and layout sections) inside this extension.
- No new standing privilege escalation (e.g. `input`-group membership) introduced by
  this UI; if a future revision needs it, it must be an explicit, separate ask, not a
  side effect of adding a history panel.
- History storage is local-only and should be treated as sensitive (it may contain
  anything the user has ever dictated); do not write it anywhere more persistent or
  more widely readable than necessary, and provide a way to clear it.

- **CLI / Backend plumbing (complete & tested)**:
  - `voice_config.go` & `harnez tools voice-input config {get,set} type-delay-ms [VAL]`: comment-preserving regex editor for `type_delay_ms` in `~/.config/voxtype/config.toml`.
  - `voice_history.go` & `harnez tools voice-input history {list,clear,record,copy,retype}`: local JSONL history store at `~/.local/share/harnez/voice-input/history.jsonl` (0600 permissions, capped at 20 entries). `record` subcommands acts as a pass-through filter for Voxtype's `[output.post_process]`.
  - `voice_type.go`: `BuildDotoolCommands`, `dotoolDaemonReady` (non-blocking open check), `TypeText` (fast `dotoolc` pipe with fallback to cold `dotool`), and `CopyText` (`wl-copy`).
  - Unit tests in `voice_config_test.go`, `voice_history_test.go`, `voice_type_test.go`, and `command_test.go`.
- **Canary Tooling & Nested Shell Verification (implemented)**:
  - `scripts/canary_nested/main.go`: automated probe spawning nested GNOME Shell (`--devkit` / `--nested`), launching an editor, testing synthetic `dotoold` typing, issuing `Ctrl+S`, validating file persistence, and cleaning up process groups with `SIGTERM`/`SIGKILL`.
  - `scripts/canary_ext/main.go`: probe for symlinking and testing extension activation in nested compositor.
  - **GNOME 50 Finding**: GNOME 50 uses `--devkit` instead of `--nested`. Without `/usr/libexec/mutter-devkit` present on the host distribution, devkit runs headless without mounting the top panel or initializing user extensions.
- **GNOME Shell extension (`contrib/gnome-shell-extension/`, complete)**:
  - Metadata UUID `voice-input@harnez.ubunatic.com` supporting GNOME Shell 45–50 ESM.
  - Top bar status icon with `audio-input-microphone-symbolic`.
  - Mode toggle header (Batch Whisper vs Streaming Parakeet via `harnez tools voice-input mode`).
  - Typing speed slider & presets (`type_delay_ms` via `harnez tools voice-input config {get,set} type-delay-ms`).
  - Recent transcripts list with Copy and Retype buttons, plus Clear history button.
  - Mutter window focus coordination (`global.display.get_focus_window()`, `win.activate()`, `notify::focus-window` handshake before typing injection) eliminating focus races.

