# 081 — Install and Run a Persistent dotoold Systemd User Service

**Status**: Open
**Priority**: P0 (Critical)
**Severity**: Critical
**Category**: Feature
**Related**: [078 install-crispasr target](078-add-make-install-crispasr-target-for-the-crispasr-binary.md), [080 surface typing failures](080-surface-eager-typing-transcription-failures-beyond-private-telemetry.md), [048 injection fallback chain](048-typing-injection-fallback-chain.md), [029 single voxi agent orchestration](029-single-voxi-agent-mode-orchestration.md)

---

## 1. Problem & Motivation

`voxi record toggle` originally failed outright this session with
"dotool not found on PATH" — traced partly to issue 077 (voxtype-vs-crispasr
engine gating) and partly to a separate gap: `dotool` (the typing-injection
binary) was never installed on this dev machine at all. That binary-install
gap is fixed and unrelated to this ticket: `install-dotool`
(`go install git.sr.ht/~geb/dotool@latest`) was added and folded into
`install-all` in commit `4372dd0` this session, because `dotool` has no OS
package on Debian/Ubuntu (confirmed via web search this session) or Fedora
(only a third-party COPR repo, `smallcms/dotool`, not an official package;
Arch only has an AUR package). Do not touch `install-dotool` as part of this
ticket.

After that fix, a live retest still showed **no typed output**, even though
`voxi telemetry query --view events --session <id> --format json` reported
`typing_completed: success: true` — no error at all. Manual reproduction
this session confirmed this is not a voxi bug: running
`echo "type hello" | dotool` directly in a terminal on this machine also
types literally nothing.

Root cause, per this repo's own hardware canary
(`docs/VoiceInput.md:155-163`, Fedora 44 GNOME Wayland, passed 2026-08-17):

> GNOME text injection needed a user systemd `dotoold` daemon
> (`DOTOOL_XKB_LAYOUT=de`) for the fast, reliable `dotoolc` path, plus
> `language_to_layout = {}` in Voxtype's config to stop it auto-forcing
> `layout=us` for English speech regardless of the physical keyboard layout.

The raw, one-shot `dotool` binary creates and tears down a fresh
`/dev/uinput` virtual keyboard device on every invocation; Mutter/libinput
on GNOME Wayland apparently doesn't reliably pick up an ephemeral uinput
device fast enough for that one-shot path to land. The persistent `dotoold`
daemon (one stable virtual keyboard device, kept running) plus the
`dotoolc` client (writes to a pipe the daemon reads) is the only path this
project's own canary actually validated as working on GNOME Wayland.

Client-side code already prefers this path automatically —
`internal/typing/typing.go` (`TypeText`, lines 53-85) calls
`dotoolDaemonReady(dotoolPipePath(...))` and routes through `dotoolc` first,
falling back to raw `dotool` only if the daemon pipe isn't ready:

```go
if dotoolDaemonReady(dotoolPipePath(d.Getenv)) {
    if err := d.RunStdin(ctx, commands, "dotoolc"); err == nil {
        return nil
    }
}
if _, err := d.LookPath("dotool"); err != nil {
    return fmt.Errorf("dotool not found on PATH: %w", err)
}
```

So no client-code change is needed — this ticket is purely about ensuring
`dotoold` is actually *running* as a persistent service so that fast path
gets used. There is currently no `dotoold` systemd unit anywhere in
`systemd/`, and no running `dotoold` process on this dev machine.

This is currently the last known blocker on real live dictation working
end-to-end on this dev machine: speech transcription is confirmed correct
and working (Cohere path, issue 078), but typing injection is confirmed
broken, and confirmed broken *silently* — telemetry's `typing_completed:
success: true` does not mean anything typed; it only means the raw `dotool`
subprocess exited 0, which it does even when its ephemeral uinput device
was invisible to the compositor.

**Related but out of scope**: issue 080 (Open, P0/Critical) is a separate,
already-filed ticket about *surfacing* typing failures beyond private
telemetry (logging/journal visibility when `TypeText` errors). This ticket
is not about visibility — it is about making the daemon-backed typing path
actually present and running so typing *works* via the tested `dotoolc`
path in the first place. Do not fold 080's scope in here or duplicate it.

## 2. Findings and Required Changes

### 2.1 New systemd unit: `systemd/dotoold.service`

Follow the style of the two existing units, `systemd/voxi-agent.service` and
`systemd/voxi-eager.service`:

```ini
[Unit]
Description=Voxi Voice Input Agent
PartOf=graphical-session.target
After=graphical-session.target
Conflicts=voxtype.service voxtype-streaming.service voxi-eager.service harnez-voice-eager.service

[Service]
Type=simple
ExecStart=%h/go/bin/voxi agent --daemon
Restart=on-failure
RestartSec=3
Environment=XDG_RUNTIME_DIR=%t
Environment=PATH=%h/go/bin:%h/.local/bin:/usr/local/bin:/usr/bin:/bin

[Install]
WantedBy=graphical-session.target
```

Both existing units share `Type=simple`, `PartOf=graphical-session.target` +
`After=graphical-session.target`, `Restart=on-failure` / `RestartSec=3`, an
explicit `Environment=PATH=...` line, and `WantedBy=graphical-session.target`
— `dotoold.service` should match that shape. Unlike `voxi-agent.service`
and `voxi-eager.service` (which `Conflicts=` each other as mutually
exclusive alternative dictation modes the user picks one of), `dotoold` is
not an alternative to anything — it is an unconditional dependency needed
regardless of which mode is active, so it should carry no `Conflicts=`
line against those units.

`ExecStart` needs the actual `dotoold` binary path and invocation (verify
at implementation time — `dotoold` ships as part of the same
`git.sr.ht/~geb/dotool` module `install-dotool` already installs, so it
should land at `%h/go/bin/dotoold` alongside `dotool`/`dotoolc`; confirm
this and record the exact binary layout in this ticket before writing the
unit).

### 2.2 Keyboard layout: `DOTOOL_XKB_LAYOUT` must not be hardcoded

The hardware canary used `DOTOOL_XKB_LAYOUT=de` for a German physical
keyboard on that specific test machine — this is machine-specific and must
not be hardcoded into a unit file voxi ships to all users. It needs to be a
Makefile/install-time or documented user-configurable value.

Checked whether voxi has its own existing layout-detection or
`language_to_layout`-style convention to reuse, per the hint at
`docs/VoiceInput.md:161`: `grep -rn "language_to_layout"` across the repo
shows **no voxi-owned convention** — the only two hits are both external:
`docs/VoiceInput.md:161` and
`docs/studies/2026-08-18-voxtype-popular-applications.md:279`, both
describing `language_to_layout = {}` as a *Voxtype* config key
(`~/.config/voxtype/config.toml`), not anything voxi's own Makefile,
systemd units, or Go code define or read. So there is nothing to reuse
directly for `DOTOOL_XKB_LAYOUT` — implementation will need to introduce
its own mechanism (e.g. an install-time prompt/env var substituted into the
installed unit file, or a `dotoold`-specific config voxi reads and exports
before starting the daemon). Record the chosen mechanism in this ticket
when implemented.

### 2.3 Where to wire the install: `install-all`, not `install-user-services`

Read both existing targets before deciding:

```makefile
install-all: ⚙️ install install-user-services install-crispasr install-dotool  # install user binaries, systemd user services, and default engine + typing-injection deps
	go install ./cmd/voxi-modifierd

install-user-services: ⚙️  # install systemd user service units
	mkdir -p $(HOME)/.config/systemd/user
	cp systemd/voxi-agent.service systemd/voxi-eager.service $(HOME)/.config/systemd/user/
	systemctl --user daemon-reload
```

`install-user-services` today only **copies unit files and reloads the
daemon** — it never runs `systemctl --user enable` or `--now` for either
unit. That is consistent with `voxi-agent.service`/`voxi-eager.service`
being mutually-exclusive alternative modes (`Conflicts=` each other): the
user is expected to `systemctl --user enable --now` whichever one they
actually want, by hand, after install. Simply adding `dotoold.service` to
that `cp` line would reproduce exactly the bug this ticket exists to fix —
the unit would be copied to disk but never started, and typing would still
silently fall back to the broken one-shot `dotool` path.

`install-all`'s own doc-comment already frames its last two prerequisites
(`install-crispasr install-dotool`) as "default engine + typing-injection
deps" — added this session (`c57fba5`/`90a9715` for `install-crispasr`,
`4372dd0` for `install-dotool`) specifically because they are unconditional
runtime dependencies of the default path, not opt-in alternatives, mirroring
issue 078's own correction (§6, "Correction" note): leaving a default-path
dependency out of `install-all` "meant a fresh default `install-all` still
lacked its required dependency — the same class of bug this ticket exists
to close." `dotoold` is exactly that same class of dependency: unconditional
for reliable GNOME Wayland typing, not a mode the user opts into.

**Decision**: add a new `install-dotoold` target (binary/unit install +
`systemctl --user daemon-reload` + `enable --now dotoold.service`, following
`install-modifierd`'s existing `enable --now` pattern for a service that
must actually run) and fold it into `install-all`'s prerequisite line
alongside `install-crispasr install-dotool`, not into
`install-user-services`. `install-user-services` remains reserved for the
copy-only, user-chooses-and-enables-manually convention it already
establishes for the mutually-exclusive `voxi-agent`/`voxi-eager` pair.

## 3. Acceptance Criteria

- `systemd/dotoold.service` exists, matches the style conventions above
  (`Type=simple`, `PartOf=`/`After=graphical-session.target`,
  `Restart=on-failure`/`RestartSec=3`, explicit `Environment=PATH=...`,
  `WantedBy=graphical-session.target`), and carries no `Conflicts=` against
  `voxi-agent.service`/`voxi-eager.service`.
- `DOTOOL_XKB_LAYOUT` is configurable per machine, not hardcoded to `de` or
  any other single layout — implementation records the chosen mechanism
  (install-time prompt, Makefile var, or a new voxi-owned config key) in
  this ticket, since no existing `language_to_layout`-style convention was
  found to reuse (see §2.2).
- A new `install-dotoold` (or equivalently named) Makefile target installs
  and enables+starts `dotoold.service`, follows `docs/Make.md`'s phony
  sentinel + self-documenting help-line conventions, and is folded into
  `install-all`'s prerequisite list (justification in §2.3).
- Running `make install-all` on a machine with no prior `dotoold` results in
  a running, enabled `dotoold.service` (`systemctl --user status
  dotoold.service` shows `active (running)`, `enabled`).
- A real, live `voxi record toggle` + a spoken utterance actually produces
  typed output in the focused window — not just
  `voxi telemetry query --view events ... typing_completed: success: true`,
  which already lies: it was `true` this session even when nothing was
  typed, because raw `dotool` exits 0 on an ephemeral uinput device the
  compositor never picked up. Telemetry success is necessary but not
  sufficient verification for this ticket.

## 4. Verification and Delivery

1. Confirm the exact `dotoold` binary path/invocation shipped by
   `git.sr.ht/~geb/dotool` (record in this ticket).
2. Implement `systemd/dotoold.service` and the `install-dotoold` Makefile
   target per §2, wired into `install-all`.
3. Run `make install-all` on this dev machine (which currently has no
   `dotoold` unit or process) and confirm `dotoold.service` is active and
   enabled.
4. Run `make check` / `go test ./...` to confirm no regressions to
   unrelated Makefile targets or Go packages.
5. Live end-to-end check required (this ticket's actual point): a real
   `voxi record toggle` with a spoken utterance must produce visible typed
   output in a focused window, on top of the already-passing
   transcription. Do not close this ticket on telemetry success alone.

## 5. Non-Goals

- The `install-dotool` binary-install target itself (commit `4372dd0`) —
  already done, unchanged, not touched by this ticket.
- Surfacing typing-failure errors beyond telemetry (journal/stdout
  visibility for `TypeText` errors) — that is issue 080's separate, already
  filed scope; do not duplicate it here.
- Any change to `internal/typing/typing.go`'s `TypeText`/`dotoolDaemonReady`
  logic — it already prefers `dotoolc` correctly when the daemon pipe is
  ready; nothing there needs to change.
- The injection fallback chain (`wtype`/clipboard-paste) proposed in issue
  048 — orthogonal, unimplemented, unrelated to getting the primary
  `dotoold`/`dotoolc` path running.

## 6. Open Question (flagged, not resolved here)

Should `dotoold.service` be scoped only to Wayland/GNOME sessions, or
installed unconditionally on every platform `voxi` targets? Voxi's own
project summary already scopes it to "Linux/Wayland" specifically, so
unconditional install is probably correct — but worth a sanity check at
implementation time (e.g. does anything break running `dotoold` on a
non-GNOME Wayland compositor, or on X11 if that's ever supported) before
shipping it unconditionally in `install-all`.
