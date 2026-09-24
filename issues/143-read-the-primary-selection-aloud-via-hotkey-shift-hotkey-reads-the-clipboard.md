# 143 — Read the primary selection aloud via hotkey, Shift+hotkey reads the clipboard

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Feature
**Related**: 141 (TTS MVP: `voxi say`, the queue owned by `voxi monitor -w`), `internal/shortcut` (Super+X GNOME shortcut setup)

---

## 1. Idea

The user selects text on screen with the mouse, which puts it in the Linux
**primary selection** (the middle-click paste buffer). A global hotkey then reads
that selection aloud. The **same hotkey with Shift** reads the **clipboard**
(Ctrl+C) instead.

## 2. /goal

- Hotkey: voxi reads the current primary selection through the existing TTS
  queue.
- Shift+hotkey: voxi reads the current clipboard.

Done when both work on GNOME Wayland on this machine, and `voxi shortcut setup`
(or its equivalent) installs both bindings. An empty selection or clipboard
gives a short, clear message instead of silence. With no monitor running, the
141 gate applies (a clear "start `voxi monitor -w`" message). The user confirms
both live.

## 3. Notes / Uncertainties

- `wl-paste` is installed: `wl-paste --primary` reads the selection and
  `wl-paste` reads the clipboard. Canary first: check that both work when
  called from a GNOME custom shortcut, where there is no focused terminal.
  GNOME Wayland can restrict clipboard access for unfocused clients (see
  `@docs/Canary.md`).
- Choose the key. Super+X is dictation. Pick a free combination, check it for
  conflicts the way `voxi shortcut setup` does for Super+X, and confirm it with
  the user.
- Should pressing the hotkey while TTS is speaking replace the queue or add
  to it? 141 chose to queue. Revisit this with the user, because "read this
  now" may mean interrupt.
- Before starting, re-check the live code and the latest state of 141/142.

## 4. Decisions (user, 2026-09-24)
- **Keys:** `Super+Y` reads the primary selection; `Super+Shift+Y` reads the
  clipboard.
- **Interrupt:** the hotkey stops current playback, clears the queue, and reads
  the new text straight away. This differs from `voxi say`, which queues.
