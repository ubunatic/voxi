# Installation Architecture

**Status:** Accepted  
**Date:** 2026-09-10  
**Related:** [Issue 104](../issues/104-add-voxi-install-command-for-complete-user-install-with-optional-privileged-setup.md), [Issue 081](../issues/081-install-and-run-a-persistent-dotoold-systemd-user-service.md), [Issue 089](../issues/089-voxi-modifierd-not-installed-on-this-dev-machine-modifier-gating-currently-inactive.md)

## Decision

Voxi has one discoverable application-level installation entry point:

```sh
make install             # bootstrap the CLI itself
voxi install             # complete user-scoped installation
voxi install --modifierd # optional system-scoped modifier gating
```

`voxi install` converges the non-privileged parts of the installation: the Voxi
binary, user service units, CrispASR, dotool/dotoold, and the enabled and active
`voxi-agent.service`. It does not invoke `sudo`, write `/usr/local`, or alter
system services. The `--modifierd` flag is an explicit privilege boundary for
installing and enabling `voxi-modifierd.service`.

The existing Make targets remain compatibility and development primitives.
`make install-all` also enables and starts the agent, preserving the behavior
needed by the Super+X workflow for users who continue to use Make directly.

## Why the boundary matters

The normal voice-input path should work entirely inside the logged-in user's
systemd session. Physical modifier monitoring is different: it reads evdev
devices and therefore needs a system service with root/device permissions. A
fresh install must not unexpectedly prompt for a password or imply that this
optional safety component is present.

Each installation phase reports independently. A failed privileged phase must
not be reported as a complete install, while a successful user phase remains
usable and can be retried later.

## Privileged install pitfall

The first live validation exposed a distinction that ordinary unit tests missed:
streaming a unit file through `sudo install /dev/stdin` is unreliable because
`sudo` and the child command compete for standard input after authentication.
The implementation now stages the canonical unit in the user's cache and gives
`sudo install` a regular source path. This keeps authentication input separate
from service-file input and is the pattern to use for future privileged copies.

## Verification

After the user-scoped install:

```sh
systemctl --user is-enabled voxi-agent.service
systemctl --user is-active voxi-agent.service
```

After the optional modifier install:

```sh
systemctl is-enabled voxi-modifierd.service
systemctl is-active voxi-modifierd.service
```

The live validation for this feature confirmed both services enabled and active
on the development machine. Automated coverage checks command construction,
privilege gating, staged service content, and error reporting.

## Session and workflow learnings

The formal issue-plus-sprint workflow paid for itself on this feature. Advisory
passes found the relevant Make targets and service boundaries; independent
review caught missing CrispASR/dotool coverage, modifier-binary handling, and a
weakened systemd unit before commit; live validation then found the sudo-stdin
failure. The main efficiency cost was ceremony and agent-thread exhaustion:
after the fixes, a second independent reviewer could not start, so local diff
review plus live checks supplied the final confidence gate.

For a narrow future ticket, use the lean fresh-handoff loop. Reserve the full
five-phase sprint for cross-cutting install, service, security, or architecture
changes where independent review can uncover integration failures.

