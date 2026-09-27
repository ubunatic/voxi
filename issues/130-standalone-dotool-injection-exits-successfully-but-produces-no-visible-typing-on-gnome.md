# 130 — Standalone dotool injection exits successfully but produces no visible typing on GNOME

**Status**: Closed — Covered by issue 129: FIFO-first dotoolc path kept, guard test in c833a2a

---

Reserved placeholder ticket.
# 130 — standalone dotool injection exits successfully but produces no visible typing on GNOME

**Status**: Open
**Priority**: P1 (High)
**Severity**: Major
**Category**: Bug
**Related**: [[129-dotoold-keyboard-layout-must-follow-the-active-input-source-not-the-install-time-layout]]

## 1. Problem & Motivation

Commit `5d5c2c3` changed GNOME typing from the persistent `dotoold` FIFO path to a
standalone `dotool` process with per-injection `DOTOOL_XKB_LAYOUT` and
`DOTOOL_XKB_VARIANT` environment variables. On the live GNOME/Wayland system,
the subprocess exits successfully and creates a temporary virtual keyboard, but
no characters reach the focused window. Voxi therefore reports a successful
injection while typing nothing.

The previous persistent `dotoold`/`dotoolc` path works. It types reliably, but
needs its XKB layout and variant synchronized with GNOME's active input source;
otherwise an active `us+mac-iso` source with an install-time `de` daemon produces
`Yebra zellow` instead of `zebra yellow`. The layout synchronization work belongs
with issue 129.

## 2. Scope

- Do not use standalone `dotool` for the GNOME active-source path.
- Keep the persistent `dotoold` virtual device and `dotoolc` FIFO as the primary
  injection mechanism.
- Ensure process-level success is not presented as proof that Wayland received
  usable input; retain explicit injection success/failure and duration logging.
- Coordinate with issue 129 for dynamic `dotoold` layout/variant synchronization.
- Preserve the existing no-retry behavior after a FIFO submission begins.

## 3. Milestones

1. **M1 — Regression guard**: add tests proving GNOME detection does not select
   standalone `dotool` and that a ready FIFO selects `dotoolc`.
2. **M2 — Persistent path**: verify the live daemon types through `dotoold` and
   logs the selected path and outcome.
3. **M3 — Layout coordination**: implement or consume issue 129's persistent
   daemon layout synchronization without reintroducing standalone injection.

## 4. Acceptance Criteria

- With a ready `dotoold` FIFO, eager injection uses `dotoolc`, not standalone
  `dotool`.
- A live GNOME dictation produces visible text in the focused window.
- Injection logs include path, success/failure, error text when available, and
  duration.
- Layout switching remains correct through issue 129's `us+mac-iso` →
  `de+nodeadkeys` → `us+mac-iso` acceptance sequence.

## 5. Verification Guidance

1. Run `make restart-service`.
2. Dictate `zebra yellow` with a ready `dotoold` service and confirm exact text.
3. Confirm the journal contains `path=dotoolc` and an outcome line.
4. Confirm no standalone `dotool` process is spawned for the GNOME path.

## 6. Progress

- Reproduced live on 2026-09-19: standalone `dotool` exited successfully but
  typed nothing; persistent `dotoold` typed text successfully.
- The working tree was restored to the persistent FIFO-first path. Dynamic layout
  synchronization remains tracked by issue 129.
