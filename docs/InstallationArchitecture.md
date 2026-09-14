# Installation Architecture

**Status:** Accepted  
**Date:** 2026-09-10  
**Related:** [Issue 104](../issues/104-add-voxi-install-command-for-complete-user-install-with-optional-privileged-setup.md), [Issue 114](../issues/114-fix-out-of-checkout-modifierd-build-fallback.md), [Issue 081](../issues/081-install-and-run-a-persistent-dotoold-systemd-user-service.md), [Issue 089](../issues/089-voxi-modifierd-not-installed-on-this-dev-machine-modifier-gating-currently-inactive.md)

## Decision

Voxi has one application-level installation entry point after acquiring the CLI:

```sh
go install ./cmd/voxi    # from a source checkout
~/go/bin/voxi install    # complete user-scoped installation
make install             # build locally, then run voxi install
voxi install --modifierd # optional system-scoped modifier gating
```

`voxi install` converges the non-privileged parts of the installation: the Voxi
binary, user service units, CrispASR, dotool/dotoold, and the enabled and active
`voxi-agent.service`. It does not invoke `sudo`, write `/usr/local`, or alter
system services. The `--modifierd` flag is an explicit privilege boundary for
installing and enabling `voxi-modifierd.service`.

The release download script also invokes `voxi install` after placing the
prebuilt binaries. `make install-all` is a compatibility alias for `make install`;
`make install-modifierd` invokes `voxi install --modifierd`. The former
`make install-system` target was removed because it only copied a binary and
did not configure a working installation. `make install-debug` remains a
diagnostic-only binary replacement; it does not set up services or restart
the agent. See [VoiceInput.md](VoiceInput.md).

`make test-install-podman` tests a local `go install ./cmd/voxi` followed by
both CLI install modes from outside the checkout, including a versioned remote
`go install` of `voxi-modifierd`. It also tests `make install` from a writable
source copy, then tests release download plus automatic `voxi install` in a
separate container. Service commands are recorded by stubs;
the test verifies generated units and installed files without mounting host
input devices or starting a real systemd manager.

The remaining distribution and real-systemd coverage is tracked in
[issue 116](../issues/116-test-installer-portability-across-distributions-and-real-systemd.md).

The test has two distinct provenance paths: the Go/Make case builds the current
checkout, while the curl case downloads the latest published release. A passing
curl case does not validate unreleased source. Conversely, the source case does
not prove that the published archive contains the same code. The systemctl and
sudo stubs verify requested commands and resulting files, not actual service
liveness or behavior across distributions.

### Container test pitfalls

- The test image must provide the version of Go required by `go.mod`; Debian
  Bookworm's `golang-go` package was too old for this repository's Go 1.26.5
  source. The image now uses `golang:1.26-bookworm`.
- The Go image defaults to `/go/bin`, so the test sets `GOBIN` explicitly to
  `~/go/bin`. It runs the installed CLI from outside the checkout to exercise
  the remote modifier-helper fallback instead of accidentally using local
  source. That fallback uses versioned `go install`, not `go build path@latest`.
- On SELinux enforcing hosts, a read-only source bind mount was denied until
  the Podman run disabled container labeling for that mount. The test never
  mounts host input devices. Go VCS stamping is disabled only inside this
  bind-mounted test because the checkout appears to have a different owner.

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

## Zero `input` Group Requirement & Security Model

Adding the desktop user to the Linux `input` group is a common anti-pattern in Linux
voice-typing tools, but it creates a standing security vulnerability: any user-level
process could read raw `/dev/input/event*` devices and log all keystrokes.

Voxi avoids this entirely through its system service architecture:
1. **Isolated Daemon**: `voxi-modifierd.service` runs as a locked-down system service
   under systemd with `SupplementaryGroups=input` and `DeviceAllow=char-input r`.
2. **Atomic State File**: The daemon continuously reads physical modifier keys
   (Ctrl, Alt, Super, Shift) and writes a single 1-byte bitmask to `/run/voxi/modifiers`
   (mode `0755`/`0644`). Non-modifier keystrokes are never recorded.
3. **Unprivileged User Access**: The user-space Voxi engine (`voxi`, `voxi-agent.service`,
   `dotool`) reads `/run/voxi/modifiers` without needing `sudo`, root, or `input` group
   membership.

## Verification & Diagnostic Probes

Verify individual unit states:

```sh
systemctl --user is-enabled voxi-agent.service
systemctl --user is-active voxi-agent.service
systemctl is-enabled voxi-modifierd.service  # only after --modifierd
systemctl is-active voxi-modifierd.service   # only after --modifierd
```

Or run the end-to-end diagnostic suite:

```sh
voxi settings --test
```

`voxi settings --test` automatically probes:
- ASR engine binary availability (`crispasr` / `voxtype`) and GPU render node presence.
- LLM post-processing cleaner HTTP reachability and model responses.
- Keystroke injection via `dotool` and `type_delay_ms` configuration.
- Modifier daemon status via `/run/voxi/modifiers` and systemd unit health.
- Dictation history storage permissions.
- Background `voxi-agent.service` lifecycle.

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
