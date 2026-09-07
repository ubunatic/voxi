# 079 — Automate Standard Super+X Keybinding Setup

**Status**: Closed — implemented and verified
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Feature
**Related**: [Voice input documentation](../docs/VoiceInput.md), [standalone extraction and migration study](../docs/studies/2026-08-18-standalone-voxi-extraction-and-migration.md)

---

## 1. Problem & Motivation

Voxi treats `voxi record toggle` as the universal recording control and
documents `Super+X` as its standard global shortcut, but the repository does
not provide a repeatable way to configure that binding. Neither `make install`
nor the GNOME Shell companion extension currently installs it. The working
GNOME setup recorded during the standalone migration was created manually and
executes the machine-specific absolute path
`/home/uwe/go/bin/voxi record toggle`.

This leaves fresh installs and migrations with an undocumented manual desktop
configuration step, and it can silently become stale when the binary location
changes. Provide an ergonomic, repeatable setup surface from the repository or
Voxi application so the documented standard workflow works without hand-editing
GNOME custom shortcuts.

## 2. Scope and Open Decisions

The implementation should first determine the most appropriate user-facing
surface. Candidates include a Voxi CLI setup command, a Make target, GNOME
extension integration, or a small combination of these. The ticket deliberately
does not prescribe one before the relevant GNOME/Wayland mechanisms and lifecycle
tradeoffs are tested.

The chosen design must address:

- configuring `Super+X` to invoke `voxi record toggle` for the current user;
- resolving the installed executable without embedding a developer-specific
  home directory or relying on a desktop session's incomplete `PATH`;
- detecting an existing `Super+X` assignment or an existing Voxi shortcut
  before changing settings;
- making repeated setup idempotent;
- reporting conflicts clearly and requiring an explicit, safe resolution path
  instead of silently overwriting another application or user binding; and
- defining what owns cleanup or restoration when Voxi is uninstalled, the
  extension is disabled, or the user requests removal of the binding.

Open questions to resolve during implementation:

- Should shortcut setup be explicit/opt-in, or part of a broader install flow?
- Is GNOME's custom-keybinding settings API the stable target, or can the
  companion extension register the accelerator with a cleaner lifecycle?
- Which non-GNOME desktops, if any, can be supported by the same interface?
- If Voxi replaces a binding with explicit user approval, can and should it
  preserve enough state to restore that exact prior binding safely?

## 3. Acceptance Criteria

- A documented repository- or application-level command configures the standard
  `Super+X` global shortcut for a supported desktop without manual Settings UI or
  `gsettings` editing.
- The installed shortcut invokes the effective Voxi binary and
  `record toggle`; it contains no hard-coded `/home/uwe` path and works from the
  desktop launch environment.
- Running setup more than once leaves one valid Voxi binding and does not create
  duplicate custom-shortcut entries.
- Setup detects both an accelerator conflict and an existing Voxi binding. It
  does not overwrite unrelated user configuration without explicit consent, and
  produces an actionable message when it cannot proceed safely.
- A supported removal/reset path deletes only configuration owned by Voxi or
  safely restores prior state when the implementation can prove that state is
  the one Voxi replaced.
- Unsupported desktops or unavailable configuration mechanisms fail clearly and
  leave existing shortcut settings unchanged.
- `README.md` and `docs/VoiceInput.md` describe setup, conflict behavior,
  supported environments, and removal; the GNOME extension instructions no
  longer imply that its installation alone creates a shortcut unless it does.
- Automated tests cover command construction, idempotency, conflict detection,
  and non-destructive removal wherever the design permits isolation from a live
  desktop session.

## 4. Verification Guidance

1. Canary the selected GNOME/Wayland mechanism before building the final setup
   surface, including how it behaves when `Super+X` is already occupied.
2. Test on a clean user configuration: run setup twice, inspect the resulting
   binding, and verify that pressing `Super+X` toggles recording.
3. Test with an unrelated command already bound to `Super+X`; verify that Voxi
   reports the conflict and does not mutate the existing binding unless the user
   explicitly chooses a supported replacement path.
4. Move or reinstall the Voxi binary through a supported install location and
   verify the shortcut does not retain the developer-specific path from the
   migration study.
5. Exercise removal/reset and confirm unrelated custom shortcuts are unchanged.
6. Run `make check` and any focused CLI or extension tests added by the chosen
   implementation.

## 5. Non-Goals

- Changing the semantics of `voxi record toggle` or recording modes.
- Claiming universal compositor support without verified configuration paths.
- Silently taking over `Super+X` from an existing desktop or application action.
- Prescribing CLI, Makefile, or extension ownership before the canary establishes
  the safest lifecycle.

## 6. Implementation & Verification

Implemented an explicit, opt-in `voxi shortcut setup` / `voxi shortcut remove`
surface backed by GNOME's custom-keybinding GSettings schemas. The shortcut owns a
fixed relocatable settings path, resolves the installed `voxi` executable to an
absolute path, detects custom and built-in accelerator conflicts, detects prior Voxi
entries, and refuses ambiguous replacement or removal. The GNOME extension and normal
install lifecycle deliberately do not own the shortcut.

The retained `scripts/canary-gnome-shortcut.sh` inspects the live GNOME shortcut list
read-only, then proves write/read behavior against an isolated keyfile backend. On the
development GNOME Wayland session it read back the expected name, command, and
`<Super>x` binding while a before/after comparison confirmed the live custom shortcut
list was unchanged. Automated coverage verifies construction, idempotency, custom and
built-in conflicts, existing Voxi detection, unsupported desktops, and non-destructive
removal. `go test ./internal/shortcut`, `make check`, and `make install` passed on
2026-09-07. A physical `Super+X` toggle was not exercised because doing so would require
installing the binding into the user's real desktop configuration; the isolated schema
canary and existing voice-input hardware canary cover those two mechanisms separately.

---

Reserved placeholder ticket.
