# 104 — Add `voxi install` command for complete user install with optional privileged setup

**Status**: Closed — implemented and verified: user install activates `voxi-agent.service`; optional `--modifierd` path is explicitly privileged and stages its service file before `sudo install`
**Priority**: P1 (High)
**Severity**: Major
**Category**: Feature
**Related**: [081 persistent dotoold service](081-install-and-run-a-persistent-dotoold-systemd-user-service.md), [089 modifier daemon installation gap](089-voxi-modifierd-not-installed-on-this-dev-machine-modifier-gating-currently-inactive.md), [Makefile](../Makefile)

---

## 1. Problem & Motivation

Voxi's complete installation workflow is currently spread across Makefile
targets: `install`, `install-user-services`, `install-crispasr`,
`install-dotool`, `install-dotoold`, and the separately remembered
`install-modifierd` step. This makes a fresh setup easy to perform only
partially and leaves users with no single, discoverable application command
that explains what has and has not been installed.

The operational consequence was reproduced after login: `voxi-modifierd.service`
was active, but `voxi-agent.service` was loaded and disabled/inactive, so the
documented Super+X recording workflow did not work. The current working-tree
Makefile change adds `systemctl --user enable --now voxi-agent.service` to
`install-all`; that change is a separate fix and must be preserved, not folded
into or reverted by this issue.

Provide a user-facing `voxi install` command that consolidates the normal
per-user installation workflow and makes its result explicit. Operations that
write system locations or require `sudo`—notably installation of the
system-wide `voxi-modifierd` service—must not be implicit or mandatory. A
fresh user install should be useful without prompting for root credentials;
the modifier daemon should be an explicitly requested optional component.

## 2. Scope and Technical Findings

The implementation should first map the existing Makefile targets and their
side effects to CLI operations rather than duplicating undocumented values in
two places. The command should cover the normal user-scoped prerequisites and
services already represented by `install-all`, including the Voxi binary,
user service units, default transcription/typing dependencies, and the
enabled-and-started `voxi-agent.service` behavior now required by the
workflow.

The privileged path should be separately gated by an explicit option or
subcommand (for example, `voxi install --modifierd`; choose and document the
final interface). Without that opt-in, the command must not invoke `sudo`,
write `/usr/local`, install `/etc/systemd/system/voxi-modifierd.service`, or
change system services. If the optional path is requested, it must explain
the privilege requirement and report failures without claiming a complete
install.

The design must define behavior for:

- repeated/idempotent invocation;
- non-GNOME or non-systemd environments and missing dependencies;
- user service enablement versus merely copying/reloading units;
- partial failure and actionable status output;
- whether existing Make targets become compatibility wrappers or remain the
  implementation primitives; and
- uninstall or cleanup ownership, without broadening this issue into a
  destructive uninstall redesign.

The existing `install-all` Makefile change is out of scope except that the
new command must preserve equivalent agent activation behavior. Do not
modify the Makefile as part of filing or implementing this ticket unless a
later implementation plan explicitly requires a compatibility adjustment.

## 3. Acceptance Criteria

- `voxi install` is an implemented, documented CLI entry point with concise
  help describing the user-scoped default install and the optional privileged
  modifier-daemon path.
- The default command performs the complete non-privileged workflow currently
  required for a working installation: user binary/dependencies, user service
  installation and reload, and enabling/starting `voxi-agent.service`.
- The default command never invokes `sudo` and never writes system-wide
  locations or systemd system units.
- The optional modifier-daemon operation is explicitly gated, installs and
  enables `voxi-modifierd.service` only when requested, and clearly reports
  privilege or platform failures.
- The command is safe to run repeatedly: already-installed binaries, units,
  and services converge without duplicate configuration or unnecessary
  destructive changes.
- The command reports each major phase and its result, including skipped
  optional privileged work, so a user can distinguish a complete user install
  from an install missing modifier gating.
- Existing Make targets remain usable or are intentionally reduced to
  documented compatibility wrappers; their semantics are not silently
  changed as an incidental part of the CLI work.
- Automated tests cover argument parsing, default versus privileged gating,
  command construction, idempotency/error reporting, and the no-`sudo`
  guarantee using injectable command/filesystem boundaries where needed.
- Documentation explains the supported Linux/session assumptions, the
  optional safety benefit of `voxi-modifierd`, and verification commands for
  the resulting user and system services.

## 4. Verification Guidance

1. On a clean user installation, run `voxi install` and verify the binary,
   dependencies, user units, and `systemctl --user is-enabled --quiet
   voxi-agent.service` plus `is-active` checks.
2. Confirm the default execution trace contains no `sudo` and leaves
   `/etc/systemd/system/voxi-modifierd.service` untouched.
3. Run the command a second time and verify it succeeds without duplicate
   units or configuration and without needlessly restarting unrelated user
   services.
4. Request the explicit modifier-daemon option on a supported machine,
   authenticate only for that path, and verify `voxi-modifierd.service` is
   installed, enabled, and active; test the denied/missing-`sudo` path too.
5. Log out/in or restart the user manager and verify the agent remains active
   and Super+X reaches `voxi record toggle`; verify modifier state separately
   when the optional daemon was installed.
6. Run `make check` and the focused CLI/install tests. Confirm the pre-existing
   Makefile diff remains unchanged and is not included in the issue commit.

## 5. Non-Goals

- Making system-wide modifier-daemon installation mandatory.
- Replacing the existing Makefile workflow without a compatibility plan.
- Redesigning the modifier daemon, transcription engines, typing injection, or
  desktop shortcut behavior.
- Automatically changing unrelated desktop or system configuration.

## 6. Closure Evidence (2026-09-10)

- `cmd/voxi install` and `internal/install` implement the user-scoped default
  workflow and the explicitly gated `--modifierd` path.
- `internal/install/install_test.go` covers default no-`sudo` behavior,
  privileged command construction, phase failures, and help text.
- `make install-all` enables and starts `voxi-agent.service`; the equivalent
  behavior is also part of `voxi install`.
- Live verification reported both `voxi-agent.service` and
  `voxi-modifierd.service` as enabled and active.
- The original privileged install failure caused by piping a unit through
  `sudo install /dev/stdin` was fixed by staging the unit in the user cache
  before invoking `sudo install`.
