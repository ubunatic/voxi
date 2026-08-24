# Single Voxi Agent for Mode Orchestration

- **Status:** In Progress / Architecture Design; docs, systemd unit, and install slice started
- **Related Issues:** 021 (Fluent Streaming), 026 (Continuous Eager Sentence Streaming), 027 (Continuous Listening & Turn-Taking), 028 (Local LLM Post-Process Hook)

## Context

Voxi currently switches voice-input behavior by starting and stopping mutually exclusive
systemd user services:

- `voxtype.service` for batch dictation.
- `voxi-eager.service` for continuous eager sentence streaming.
- `voxtype-streaming.service` for the experimental Parakeet streaming path.

This was quick to build because it reused Voxtype's daemon directly, but it leaks service
manager state into normal product behavior. A system or session update can bring a previously
enabled service back, even if the last interactive choice was eager mode.

The immediate guardrail is to make `voxi mode` persist systemd enablement for the selected
mode. The longer-term design is one always-enabled Voxi agent that owns voice-input state and
switches backends internally.

## Goal

Replace multi-service mode orchestration with a single long-running `voxi-agent.service`:

```text
voxi-agent.service
  owns one control socket
  owns persistent selected mode
  owns recording state
  exposes status, set-mode, record start/stop/toggle
  runs exactly one backend at a time
```

Systemd should only need to keep the agent alive. Mode switching should become a Voxi runtime
command, not a `systemctl` start/stop workflow.

## Proposed Backend Model

Define a small backend interface:

```text
Start(ctx)
Stop(ctx)
Record(action)
Status()
```

Only one backend may be running or recording at a time. Mode changes must cancel the active
backend cleanly before starting another one.

### Batch Backend

Preferred direction:

```text
voxi-agent records WAV
voxi-agent runs voxtype --model <model> transcribe <wav>
voxi-agent filters/normalizes transcript
voxi-agent types via dotool
```

This avoids running `voxtype daemon` for batch mode. Voxtype remains the ASR inference CLI,
while Voxi owns hotkeys, recording state, output, and lifecycle.

### Eager Backend

Move the existing `internal/eager` daemon behavior behind the agent:

```text
voxi-agent receives control command
eager backend records rolling chunks
voxtype transcribes finalized phrase chunks
Voxi filters and types committed text
```

The eager backend should stop owning its own top-level daemon socket once the agent owns the
control plane.

### Streaming Backend

Treat streaming as transitional:

- v1 may disable streaming or wrap `voxtype -c config-streaming.toml daemon` as a child
  process.
- Long term, streaming should use a direct primitive/API so Voxi owns recording and commit
  policy like the other backends.

Do not block the first agent milestone on a full Parakeet rewrite.

## Design Requirements

- One installed/enabled user service: `voxi-agent.service`.
- One control socket used by CLI, GNOME extension, and record commands.
- One persisted selected mode, stored in Voxi-owned config/state rather than systemd
  enablement.
- `voxi mode` talks to the agent with `set-mode`, then reports the agent's mode/status.
- `voxi record` talks to the agent, regardless of active backend.
- Backend transitions are serialized; switching during active recording must either refuse
  with a clear error or stop/finalize recording first.
- Failed backend startup must leave the agent alive and report an actionable status.
- No two components may own microphone capture or typing output at the same time.

## Migration Plan

1. Add `voxi agent --daemon` with a control socket and status command.
2. Move current eager daemon control into the agent while preserving existing eager behavior.
3. Implement batch as a Voxi-owned record/transcribe/type backend using `voxtype transcribe`.
4. Change `voxi mode` and `voxi record` to talk to the agent.
5. Add `systemd/voxi-agent.service` and install it from `make install-user-services`.
6. Keep compatibility commands for `voxi-eager.service` during migration, then remove or
   deprecate the separate eager service.
7. Revisit streaming once batch and eager are stable behind the common agent.

### Migration notes

The first migration slice installs `systemd/voxi-agent.service` and keeps
`voxi-eager.service` available as the compatibility unit. `make install-user-services`
reloads the user manager but does not enable either unit. The initial runtime agent owns the
systemd service and starts `voxi eager --daemon` as a child process for eager mode; batch and
streaming remain state-only placeholders until their backend adapters land. The eager unit
can be deprecated after restart/login testing proves the agent-owned child path is reliable.

The unit intentionally uses the user-local binary at `%h/go/bin/voxi`, matching the existing
eager service and `make install` workflow. It inherits the user runtime directory and an
explicit PATH so child tools such as `pw-record`, `voxtype`, and `dotool` resolve the same way
under systemd as they do from the shell.

## Acceptance Criteria

- [ ] `voxi-agent.service` can be installed, enabled, and restarted as the only voice-input
      user service.
- [ ] `voxi mode eager` and `voxi mode batch` switch backend state without invoking
      `systemctl` for mode changes.
- [ ] `voxi record start/stop/toggle/status` works through the agent for eager and batch.
- [ ] Eager remains the default selected mode after login/session restart/system update.
- [ ] Batch mode does not require `voxtype.service`.
- [ ] Batch invokes Voxtype with global flags before the subcommand:
      `voxtype --model <model> transcribe <wav>`.
- [ ] Backend crash/startup failure leaves the agent running with useful status output.
- [ ] Tests cover mode transitions, active-recording switch behavior, and failed backend
      startup.
