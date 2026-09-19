# Typing Layout Architecture

**Related:** [Issue 129](../issues/129-dotoold-keyboard-layout-must-follow-the-active-input-source-not-the-install-time-layout.md), [Issue 081](../issues/081-install-and-run-a-persistent-dotoold-systemd-user-service.md), [Issue 128](../issues/128-settings-save-should-restart-the-daemon-or-auto-reload-changed-config.md), [VoiceInput.md](VoiceInput.md), [InstallationArchitecture.md](InstallationArchitecture.md)

Why dictated text can come out with `y` and `z` swapped, and how voxi picks the
keyboard layout it types with. Also records the neighbouring "config change needs a
restart" pitfalls found in the same investigation.

## 1. The two-layout problem

Typing goes through two independent layout lookups:

1. **`dotool`** turns each character into a Linux keycode using its *own* XKB layout,
   read once at process start from `DOTOOL_XKB_LAYOUT` and `DOTOOL_XKB_VARIANT`.
2. **The compositor** (GNOME/Mutter) turns those keycodes back into characters using
   the session's *active input source*.

They only agree if both use the same layout. If dotool assumes `de` and GNOME is on
`us`, the keycode dotool chose for `z` (the `Y` position on QWERTZ) is read as `y`.
Symbols, dead keys and umlauts break the same way, and a missing variant
(`mac-iso`, `nodeadkeys`) breaks them further.

Consequences worth remembering:

- The correct layout is the **active input source**, not the physical keyboard and not
  the install-time locale. GNOME has one active source for all keyboards, so attaching
  an English keyboard to a German laptop changes nothing by itself. The user switches
  source (for example a profile `EN2` = `us+mac-iso`).
- The install-time `localectl` X11 layout (`Makefile`, `internal/install`) is baked into
  `dotoold.service` as `DOTOOL_XKB_LAYOUT`. It is right for a single-layout machine and
  wrong as soon as the session switches source. Issue 081 required "not hardcoded",
  which install-time detection satisfied only for a fixed layout.
- `mru-sources` in `org.gnome.desktop.input-sources` is ordered by recent use and is
  **not** the active source. Use `sources` indexed by `current`.

## 2. Current design

- `internal/inputsource.DetectActive` runs `gsettings get
  org.gnome.desktop.input-sources sources` and `current` through
  `deps.Dependencies.RunOutput`, picks the tuple at index `current`, and splits an XKB id
  `layout+variant` into `Source{Layout, Variant}`. Non-`xkb` sources (IBus and similar),
  malformed output and gsettings failure all return an error.
- `typing.TypeTextObserved` calls it on every injection. On success it runs a
  **standalone `dotool`** through `deps.RunStdinEnv` with `DOTOOL_XKB_LAYOUT` and,
  when present, `DOTOOL_XKB_VARIANT`. On error it falls back to the old path: the
  `dotoold` pipe if ready, else plain `dotool`, with the install-time layout.
- Modifier gating and `type_delay_ms` are unchanged and run before detection.

### Trade-offs of this choice

Issue 129 listed two approaches: restart `dotoold` on a layout change, or spawn a
standalone `dotool` per injection. The shipped one is the second.

| | Standalone `dotool` per injection (shipped) | Restart `dotoold` on change |
| :--- | :--- | :--- |
| Layout switch | Correct on the next injection, no race | Race between switch and restart |
| Latency | Process spawn plus `/dev/uinput` device setup per chunk | Keeps the fast FIFO path |
| State | Stateless | Must track the daemon's layout |

The cost is that on GNOME the persistent `dotoold` FIFO path is no longer used at all
(detection succeeds, so the pipe is never reached). The "<10 ms via named pipe" claim
in [VoiceInputArchitecture.md](VoiceInputArchitecture.md) now only holds on the
fallback path. Measure per-chunk spawn cost before deciding whether to move to the
restart approach; a `dotoold` restart only on detected change would restore it.

### Known gaps (issue 129 remaining scope)

- Detection runs two `gsettings` subprocesses per injection; a `dconf` watch could
  make it event-driven.
- The fallback silently uses the install-time layout. The issue asks for a logged
  warning and a `voxi status` line showing layout in use versus active (M3).
- Non-GNOME compositors (KDE, Sway, Hyprland) always take the fallback.
- Live verification ("zebra yellow" under `us+mac-iso`, then `de+nodeadkeys`, then back)
  and `make restart-service` are still owed before the issue can close.

## 3. Config-change pitfalls found alongside (issue 128)

- `voxi eager --daemon` is spawned by `voxi agent --daemon` and reads `UserSettings`
  **once at start**. Saving in `voxi settings` does not reach it. `make install` does
  not either, since the running unit keeps the old binary in memory.
- `voxi settings` now offers a `[y/N]` restart after a successful save, running
  `systemctl --user try-restart voxi-agent.service`.
- `try-restart` restarts only a running unit and succeeds silently on an inactive one.
  A plain `restart` would *start* a service the user had stopped. Wording must say
  "restart requested (only applies if running)", never "restarted".
- The restart can interrupt an active dictation, hence the prompt. The drain and
  delivery-ledger check from the ticket ([EagerDeliverySafety.md](EagerDeliverySafety.md))
  is not implemented; the prompt only mitigates it.
- The settings TUI's `Key` is an int enum (`KeyUnknown`..`KeyQuit`), so a rune such as
  `'y'` can never equal a `Key`. The confirmation is therefore a cooked-mode line read
  (`confirmRestart`), which also avoids raw-mode terminal state at the prompt.
- Render each restart outcome separately (declined, requested, failed). An early version
  printed "restarted" after the user declined.

## 4. Testing notes

- Detection is unit-testable through `deps.Dependencies.RunOutput`; table cases cover
  `de+nodeadkeys`, `gb+mac`, `us+mac-iso`, no variant and failure.
- Real typing behaviour depends on the live compositor and cannot be proven by
  `go test`. The acceptance check is manual, on the machine, with a Y/Z-sensitive
  phrase.
- Typing code is on the live daemon path: use `make restart-service`, not
  `make install`.
