# 129 — dotoold keyboard layout must follow the active input source, not the install-time layout

**Status**: Open
**Priority**: P1 (High)
**Severity**: Major
**Category**: Bug
**Related**: [[081-install-and-run-a-persistent-dotoold-systemd-user-service]], [[128-settings-save-should-restart-the-daemon-or-auto-reload-changed-config]]

---

## 1. Problem & Motivation

Discovered live: with the GNOME input source switched to the English profile
(`us+mac-iso`, an attached Logitech MX Keys for Mac), dictated text comes out with
`y` and `z` swapped. The laptop's own keyboard is German, and GNOME lists the
sources `de+nodeadkeys`, `gb+mac` and `us+mac-iso`.

Root cause, verified on the live machine:

- `voxi` injects text as `type <text>` lines through `dotoold`/`dotoolc`
  (`internal/typing/typing.go`). `dotool` converts each character to a keycode with
  its own XKB layout, which it reads **once at startup** from `DOTOOL_XKB_LAYOUT`
  (and `DOTOOL_XKB_VARIANT`).
- `~/.config/systemd/user/dotoold.service` has `DOTOOL_XKB_LAYOUT=de`, substituted at
  install time from `localectl`'s "X11 Layout" (`Makefile:51`,
  `internal/install/install.go:170`). It is the physical laptop layout.
- GNOME interprets the injected keycodes with the **currently active input source**
  (`us`). The `Y` keycode that dotool picked for "z" on `de` is read as "y" on `us`.
- `DOTOOL_XKB_VARIANT` is never set, so `mac-iso` and `nodeadkeys` differences (dead
  keys, `<>` and `^` placement, umlauts, `@`, etc.) are also wrong.

[[081]] §2.2 required the layout to be "not hardcoded", and the install-time
detection satisfied that for a single-layout machine. It does not handle a session
that switches layouts, or a second keyboard with a different layout. GNOME has one
active source for all keyboards, so plugging in a keyboard does not change it.

## 2. Scope

Voxi should type with the layout the session is actually using at injection time.

- Detect the active input source (layout and variant). On GNOME read
  `org.gnome.desktop.input-sources` (`sources` and `current`). Do not use
  `mru-sources`, which is ordered by recent use, not by what is active now.
  Keep detection behind the existing `deps.Dependencies` boundary so it is testable.
- When the active layout or variant differs from what the running `dotoold` was
  started with, make dotool use the right one. Two candidate approaches; the choice
  is open:
  1. Restart `dotoold` with the new `DOTOOL_XKB_LAYOUT`/`DOTOOL_XKB_VARIANT`. This
     keeps the fast pipe path but adds a restart on every layout change.
  2. Send that injection through a standalone `dotool` process with the right
     environment. This is slower but needs no restart.
  Preferred starting point: approach 1, with the running daemon's layout tracked so
  restarts happen only on change.
- Keep the install-time `localectl` value only as a fallback when detection fails
  (non-GNOME session, missing `gsettings`).
- Non-GNOME compositors (KDE, Sway, Hyprland) are out of scope for the first cut;
  record what fallback applies and leave a note for follow-up.

Uncertainties to resolve during implementation:

- Whether GNOME signals input-source changes (a `dconf` watch) so detection can be
  event-driven instead of per-injection polling.
- Whether the layout can be switched mid-dictation, so a chunk typed right after a
  switch races the `dotoold` restart.
- How `voxi status` should report the layout dotoold is using versus the active one.

## 3. Milestones

- **M1 — Detection**: parse the active source (layout plus variant) from
  `gsettings`. Automated: table-driven unit tests covering the `sources` and
  `current` combinations seen here (`de+nodeadkeys`, `gb+mac`, `us+mac-iso`), a plain
  layout with no variant, and detection failure.
- **M2 — Apply**: on mismatch, get dotool onto the active layout (per the chosen
  approach) before injecting. Automated: a typing test with a fake dependency that
  asserts the injector environment or restart for a layout change, and no restart when
  unchanged.
- **M3 — Fallback and status**: fall back to the install-time layout when detection
  fails, and surface the layout in use in `voxi status`. Automated: tests for both.
- **M4 — Docs**: update `docs/VoiceInput.md` (the `DOTOOL_XKB_LAYOUT` section) and the
  install docs so they no longer describe the layout as an install-time-only setting.

## 4. Acceptance Criteria

- With the active source `us+mac-iso`, dictating "zebra yellow" types exactly that, and
  it still does after switching to `de+nodeadkeys` and back, without a manual
  `systemctl` restart.
- `DOTOOL_XKB_VARIANT` follows the active source.
- No detection support (or detection error) degrades to the install-time layout with a
  logged warning, never a hard failure.
- `make restart-service` was run after the change, since this touches the live daemon
  path.

## 5. Workaround Until Fixed

Restart `dotoold` with the active layout, for example
`DOTOOL_XKB_LAYOUT=us DOTOOL_XKB_VARIANT=mac-iso`. This breaks German typing until the
next manual change.
