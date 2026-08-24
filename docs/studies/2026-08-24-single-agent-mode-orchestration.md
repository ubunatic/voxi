# Single Agent Mode Orchestration

**Date:** 2026-08-24  
**Branch:** `feat/single-voxi-agent`  
**Feature Issue:** [Issue 029: Single Voxi Agent for Mode Orchestration](../../issues/029-single-voxi-agent-mode-orchestration.md)

## Context

The workstation unexpectedly returned to Voxtype batch mode after a system update, and the
batch config had also drifted to `large-v3-turbo`. Voxi's eager default was still
`small.en`, but the runtime owner was `voxtype.service`, so the intended eager default was
bypassed.

The immediate fix made `voxi mode` persist the selected service with systemd enablement. That
restored eager after restarts, but it exposed a broader design problem: normal mode selection
was coupled to multiple user services.

## Decision

Move toward one always-enabled `voxi-agent.service` that owns:

- the control socket,
- selected mode,
- recording status,
- backend lifecycle,
- and record/mode commands.

The first production slice is intentionally transitional. The agent owns the systemd service
and starts the existing `voxi eager --daemon` implementation as a child process. This keeps
real eager dictation working while avoiding a premature rewrite of eager capture,
transcription, and typing.

```text
systemd
  -> voxi-agent.service
       -> voxi agent --daemon
            -> child: voxi eager --daemon
```

Batch and streaming are still placeholders behind the agent. They must not be treated as
fully migrated until they own real recording/transcription paths.

## What Changed

- Added `internal/agent` with a serialized control plane and Unix socket protocol.
- Added `voxi agent --daemon`, `voxi agent status`, `voxi agent set-mode`, and
  `voxi agent record`.
- Updated `voxi mode` and `voxi record` to use the agent when reachable, falling back to the
  legacy service/Voxtype behavior otherwise.
- Added `systemd/voxi-agent.service` with conflicts against legacy voice services.
- Updated installation and voice-input docs for the staged migration.
- Enabled the new local setup: `voxi-agent.service` active/enabled, legacy voice services
  inactive/disabled.

## Verification

Automated checks:

```sh
go test ./...
go test -race ./internal/agent
systemd-analyze verify --user systemd/voxi-agent.service systemd/voxi-eager.service
make install
```

Runtime canary:

```text
voxi agent status  -> eager (idle)
voxi mode          -> eager (voxi agent, idle)
voxi record start  -> Recording started
voxi record status -> recording
voxi record stop   -> Recording stopped
```

The user confirmed recording works through the new setup.

## Agentic Workflow Notes

Three model-specific development agents were used:

- `gpt-5.6-terra` implemented the control-plane slice.
- `gpt-5.6-luna` handled docs, systemd, and install wiring.
- `gpt-5.6-sol` performed read-only architecture review.

This split worked well because write scopes were mostly disjoint. The review agent found the
most important risks before the runtime cutover: stale eager sessions can type after stop,
multiple microphone owners can run during migration, batch hotkey ownership is unresolved,
and the initial issue text had the wrong Voxtype argument order.

The practical adjustment was to avoid migrating eager internals immediately. Running the
existing eager daemon as an agent-owned child gives one-service operation now and keeps the
hard cancellation/session rewrite for a separate slice.

## Remaining Risks

- Eager still has internal cancellation hazards: some transcription and typing work uses
  background contexts and needs session-generation protection before direct backend
  integration.
- Batch does not yet work through the agent without `voxtype.service`; it needs a Voxi-owned
  record/transcribe/type backend.
- Streaming remains experimental and should stay behind a later adapter.
- GNOME and monitor integrations may need structured agent status instead of parsing human
  CLI text.
- The migration still needs restart/login canaries to prove no legacy service comes back.

## Next Slice

Implement the real batch backend behind the agent:

```text
record audio in Voxi
write WAV
run voxtype --model <model> transcribe <wav>
filter transcript
type via dotool
record history
```

Keep eager direct-backend extraction separate, because it needs stricter cancellation,
bounded shutdown, and stale-output prevention.
