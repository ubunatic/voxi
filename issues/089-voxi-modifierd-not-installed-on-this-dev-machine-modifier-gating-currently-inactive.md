# 089 — `voxi-modifierd` not installed on this dev machine (modifier gating currently inactive)

**Status**: Closed — installed and verified live on 2026-09-10: voxi-modifierd enabled/active; voxi-agent enabled/active; modifier state available
**Priority**: P2 (Medium)
**Severity**: Bug (safety feature inactive, local environment)
**Category**: Operations / Local Environment
**Related**: [internal/modifiers/modifiers.go](../internal/modifiers/modifiers.go), [Makefile](../Makefile) (`install-modifierd` target), [docs/LiveMicMeter.md](../docs/LiveMicMeter.md) (found during the same session)

---

## 1. Problem

A live audit (dispatched while working on the demo recording, 2026-09-08) found
`voxi-modifierd` — the kernel-level evdev modifier-gating daemon that's the core
safety guarantee behind the "Physical Modifier Gating Safety" feature the project
advertises (website hero, feature card) — is **not installed or running** on this
development machine at all:

- `systemctl status voxi-modifierd.service` → "Unit could not be found" (no unit at
  `/etc/systemd/system/voxi-modifierd.service`)
- `systemctl is-enabled` → `not-found`
- `journalctl -u voxi-modifierd` → zero log entries, ever
- No running process
- `/run/voxi/modifiers` (its state file, `DefaultModifierStatePath` in
  `internal/modifiers/modifiers.go`) does not exist; `/run/voxi/` isn't even created
- Confirmed live: `voxi monitor -s s` shows `mods: off`, consistent with the missing
  state file

The `voxi-modifierd` binary itself exists (`~/go/bin/voxi-modifierd`, built via a
plain `go build`/`go install`), but `sudo make install-modifierd` — which installs the
systemd unit and does `systemctl enable --now` — was apparently never actually run.

## 2. Practical Implication

Right now, physical modifier-key gating is **not active** on this machine. Dictation
typing-injection has no kernel-level protection against hotkey combos (e.g. Super+X)
leaking individual keystrokes into the focused window while the modifier is held down.
This was visibly captured in `website/voxi-demo.mp4` (the `mods: off` status is plainly
visible in the recording) — not just a theoretical gap.

## 3. Scope

This is filed as a **local environment / operational** issue, not a code bug — nothing
in `internal/modifiers` or the daemon itself was found broken; the daemon was simply
never installed on this box. Filed so it doesn't get silently lost, and to prompt a
decision on whether `make install-all`/onboarding docs should more strongly surface
that `install-modifierd` is a *separate, easy-to-forget* `sudo` step (the main
`make install`/`make install-user-services` path doesn't touch it).

## 4. Historical Next Steps

1. Run `sudo make install-modifierd` on this machine and re-verify (`systemctl status`,
   `voxi monitor` showing `mods:` as `neutral`/active rather than `off`).
2. Consider whether `docs/`/README/website quickstart should call out more prominently
   that the optional modifier-gating daemon install step is easy to skip/forget, since
   it's the one step requiring `sudo` and a separate systemd **system** (not user)
   service — already noted as "(Optional)" in the quickstart's install command block,
   but "optional" undersells that it's the actual safety mechanism behind a headline
   feature.
3. No code change was anticipated; this issue is now closed after installation
   and live re-verification on this machine.

## 5. Closure Evidence (2026-09-10)

The previously missing system service is now enabled and active. The user
agent is also enabled and active, so the post-login workflow has both the user
control plane and the physical modifier gate running. The consolidated
installation path is documented in
[`docs/InstallationArchitecture.md`](../docs/InstallationArchitecture.md).
