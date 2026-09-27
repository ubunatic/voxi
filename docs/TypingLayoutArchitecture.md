# Typing Layout Architecture

**Related:** [Issue 129](../issues/129-dotoold-keyboard-layout-must-follow-the-active-input-source-not-the-install-time-layout.md), [Issue 081](../issues/081-install-and-run-a-persistent-dotoold-systemd-user-service.md), [Issue 128](../issues/128-settings-save-should-restart-the-daemon-or-auto-reload-changed-config.md), [VoiceInput.md](VoiceInput.md), [InstallationArchitecture.md](InstallationArchitecture.md)

Why dictated text can come out with `y` and `z` swapped, and how Voxi keeps
`dotoold` aligned with the desktop keyboard layout.

## 1. The two-layout problem

Typing goes through two independent layout lookups:

1. `dotool` converts each character to a Linux keycode using its XKB layout,
   read when `dotoold` starts from `DOTOOL_XKB_LAYOUT` and
   `DOTOOL_XKB_VARIANT`.
2. The compositor converts that keycode to a character using the active input
   source.

They agree only when both use the same layout and variant. If dotool assumes
`de` while GNOME is on `us`, the keycode for `z` can be interpreted as `y`;
symbols, dead keys, and umlauts can also differ.

The correct setting is the active input source, not the physical keyboard or
install-time locale. GNOME has one active source for all keyboards, so connecting
another keyboard does not change the source. In
`org.gnome.desktop.input-sources`, Voxi selects `sources[current]`; `mru-sources`
is only an ordering by recent use and does not identify the current selection.

## 2. Runtime behavior

- `internal/inputsource.DetectActive` reads the GNOME source and returns its XKB
  layout and optional variant. Voxi caches source reads for 500 ms to avoid
  launching `gsettings` for every eager chunk.
- When the persistent `dotoold` FIFO is ready, typing compares the detected
  source with the daemon's configured layout. If they differ, Voxi writes
  `~/.config/systemd/user/dotoold.service.d/voxi-layout.conf`, runs
  `systemctl --user daemon-reload`, and restarts `dotoold.service` with the new
  `DOTOOL_XKB_LAYOUT` and `DOTOOL_XKB_VARIANT`.
- Voxi waits up to 1.5 seconds for the restarted daemon's FIFO to become ready
  before sending the `dotoolc` command. Typing operations are serialized across
  synchronization and injection.
- If source detection fails, Voxi logs a warning, removes
  `voxi-layout.conf`, reloads systemd, and restarts the daemon with the
  install-time layout from its unit. It then waits for the FIFO and continues
  typing when it is ready.
- `voxi status` displays the active source, the layout configured for `dotoold`,
  and whether fallback is active. It also reports detection or layout warnings.
- Non-GNOME desktops are outside the first-cut detection support. If Voxi cannot
  detect an XKB source (including unsupported source types), it uses the
  install-time unit layout and reports fallback in the warning and status.

The unit's base `DOTOOL_XKB_LAYOUT` comes from `localectl` at install time and
can be overridden with `make DOTOOL_XKB_LAYOUT=<layout> install-dotoold`. It is
the fallback layout; GNOME source changes are applied through the per-user
`voxi-layout.conf` drop-in.

## 3. Switch timing

The source cache can lag a desktop switch by up to 500 ms. A switch immediately
after detection can also race a chunk while Voxi restarts `dotoold`. Typing is
serialized within Voxi, and the new daemon FIFO is checked before injection, but
the compositor can still change its source independently. Live acceptance should
check a Y/Z-sensitive phrase before and after switching between
`us+mac-iso` and `de+nodeadkeys`.

## 4. Related config-change behavior (issue 128)

- `voxi eager --daemon` reads `UserSettings` once at start. Saving settings does
  not reload that process; `voxi settings` offers a restart prompt.
- `systemctl --user try-restart voxi-agent.service` restarts only a running
  service. A plain `restart` would start a service the user had stopped.
- Restarting can interrupt dictation. The settings prompt mitigates this, but
  the drain and delivery-ledger check in
  [EagerDeliverySafety.md](EagerDeliverySafety.md) is not implemented.

## 5. Testing

Source parsing is covered by table-driven tests for `de+nodeadkeys`, `gb+mac`,
`us+mac-iso`, plain layouts, and detection errors. Typing tests use fake
dependencies to check restart-on-change, unchanged-layout behavior, FIFO
readiness, fallback restoration and warnings, caching, and status data. Real
keystroke delivery still requires the live compositor; use the acceptance
sequence above and `make restart-service` after changing typing code.
