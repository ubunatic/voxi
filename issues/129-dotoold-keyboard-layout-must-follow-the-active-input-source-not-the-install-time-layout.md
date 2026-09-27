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

## 6. Progress

- 2026-09-19, `5d5c2c3`: M1 (`internal/inputsource`) and M2 landed, using approach 2
  (standalone `dotool` with `DOTOOL_XKB_LAYOUT`/`DOTOOL_XKB_VARIANT` per injection when
  detection succeeds). `docs/VoiceInput.md` updated in part.
- Still open: live acceptance check and `make restart-service`.
- 2026-09-27, M2 canary: a disposable user unit started with
  `DOTOOL_XKB_LAYOUT=de` was given a unit-local drop-in with `us` and `mac-iso`,
  followed by `systemctl --user daemon-reload` and restart. The new process
  environment contained both updated variables. `systemctl --user set-property`
  rejected `Environment=` as an unknown assignment, and noninteractive
  `systemctl edit` required a TTY; direct drop-in creation worked. The disposable
  unit and drop-in were removed afterward. This validates per-unit configuration
  refresh without changing the live `dotoold` service.
- 2026-09-27, `c833a2a`: M2 delivered (layout sync via drop-in
  `dotoold.service.d/voxi-layout.conf` + restart on change, serialized with
  typing; 130 M1 guard test included). Reviewed by terra.

### M3 Pre-Work / Required Refinements

1. After restarting `dotoold`, wait for the new FIFO to be ready (bounded
   timeout) before `dotoolc` writes; add a test for restart-then-ready sequencing.
2. Fallback on detection failure must use the install-time layout, not the last
   persisted drop-in: remove or restore `voxi-layout.conf` accordingly, and
   test the warning plus fallback.
3. Cache the daemon source (and ideally the active source) so each chunk does
   not spawn `systemctl show` and `gsettings` synchronously.
4. Use conventional commit subjects, e.g. `fix(typing): ...`.

- 2026-09-27, M3: bounded FIFO readiness waits after daemon restart; detection
  failure removes the Voxi layout drop-in and restarts with the unit's
  install-time layout; active and daemon source reads are cached for 500 ms;
  `voxi status` reports the active source, dotoold layout, and fallback state.
  Fake based tests cover readiness ordering, fallback restoration/warning,
  caching, and status data. `go test ./internal/typing/ ./internal/inputsource/`
  passed; `go build ./cmd/voxi` passed. Live acceptance and service restart remain.
- 2026-09-27: M4 docs in `3b147f7`. Full `make test-q1` green; `make restart-service`
  done; `voxi status` reports active `de+nodeadkeys`, dotoold `de`, no fallback.
  Only open item: human live check (§4 "zebra yellow" us+mac-iso → de+nodeadkeys →
  us+mac-iso). Issue 130 is covered by this work (FIFO-first guard test in `c833a2a`).

### Live check 2026-09-27 (user) — layout correct, first chunk after a switch lost

- DE and EN both type "Yellow zebra" correctly: y/z mapping follows the source.
- Failure: the first dictated chunk after switching the input source is dropped
  (EN after DE, DE after EN); a mid-dictation chunk ("After switching") was also
  lost. Steady state without switching is instant and correct.
- Likely cause: the restart happens inside the injection path. The FIFO check
  passes before GNOME has registered the new uinput virtual keyboard, so the
  first keystrokes go nowhere.

### M5 Pre-Work / Required Refinements

1. Restart `dotoold` proactively when the input source changes (e.g. watch
   `gsettings monitor org.gnome.desktop.input-sources current` / `sources` in the
   agent), not at injection time, so the device is warm before the user speaks.
2. Keep the injection-time check as a safety net, but after any restart wait
   until the new virtual keyboard is usable (device settle, not just FIFO
   presence) before writing; measure the required settle time with a canary.
3. Test: source change triggers a restart without any injection; injection right
   after a restart waits for the settle condition.

- 2026-09-27, M5 canary (no key events sent): launched three disposable
  `dotoold` units on separate FIFOs and watched for each new `dotool keyboard`
  event node to appear and then for `gnome-shell` to hold that exact
  `/dev/input/eventN` descriptor. The kernel event node appeared after 95–111 ms;
  GNOME opened it after 876–1598 ms. The GNOME-owned descriptor is a stronger
  readiness signal than FIFO presence or a fixed sleep, and avoids typing into
  any window. It proves Mutter/libinput opened the device, not end-to-end key
  delivery. Use this signal with a 5 s bound; the observed maximum was about
  1.6 s.
- 2026-09-27, M5 implementation: the agent polls active XKB source changes and
  prewarms `dotoold` before injection. Injection retains its safety check and
  waits for GNOME to open the newly created keyboard (plus FIFO readiness) before
  writing. Fakes cover proactive restart without injection and immediate typing
  blocked until the device readiness signal. `make test-q1` passed with no
  `FAIL` lines; captured output is `/tmp/voxi-issue129-m5-test.log`.
