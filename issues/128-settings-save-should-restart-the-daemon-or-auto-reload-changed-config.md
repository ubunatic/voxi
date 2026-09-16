# 128 — Settings save should restart the daemon or auto-reload changed config

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: UX / Architecture
**Related**: [[113-wire-saved-asr-history-and-modifier-settings-into-runtime]], [[126-use-lmcoder-openai-compatible-transcription-api-as-asr-backend-target]], [[107-interactive-settings-tui-for-feature-toggles-and-configuration]]

---

## 1. Problem & Motivation

Discovered live while verifying [[126]]'s `openai-transcribe` engine: after changing the
ASR model in `voxi settings` (e.g. Cohere -> the new `openai-transcribe-gemini` model)
and saving, dictation kept using the *old* model — chunks kept going to the previous
engine even though `~/.config/voxi/config.yaml` and `env` were updated correctly.

Root cause: `voxi-agent.service` runs `voxi agent --daemon`, which spawns a child
`voxi eager --daemon` process (`internal/agent/eager_backend.go`). That child reads
`UserSettings` (and therefore `ASRModel`) exactly once, at `Start()`, and passes it as a
fixed `--model` argument. Nothing observes the config files changing afterward, so a
running daemon keeps whatever model (and any other setting baked into that one read) it
started with indefinitely — only `systemctl --user restart voxi-agent.service` picks up
a change. `internal/settings/render.go`'s `RenderSaveSuccess` already prints this restart
instruction after every save, so the gap is not undocumented, but it is still a manual
step a user can easily miss (as happened live in this session), and the currently
running config silently diverges from the saved one with no indication in `voxi status`
or `voxi settings` that they differ.

Note this ticket is distinct from [[113]]: 113 is about settings values never being read
by the runtime at all (a wiring gap, now partially closed for `ASRModel` by [[126]]'s
work this session, still open for `dictation_history`/`modifier_gating`). This ticket
assumes the values *are* wired and asks how a change should take effect without a user
remembering to restart a systemd unit by hand.

## 2. Scope

Two candidate approaches (pick one, or a small combination):

- **Restart-on-save**: `voxi settings`'s save path (`internal/settings/tui.go`,
  `command.go`) shells out to `systemctl --user restart voxi-agent.service` itself after
  `config.SaveUserSettings` succeeds (best-effort; don't hard-fail the save if the
  service isn't running, e.g. non-daemon/dev usage), replacing the current print-only
  `RenderSaveSuccess` instructions with actually doing it (or making it an explicit
  yes/no prompt in the TUI if a mid-dictation restart could drop an in-flight chunk).
- **Live reload**: the daemon (`internal/agent`, `internal/eager`'s daemon loop) watches
  its config files (or a SIGHUP-style internal signal from `voxi settings` after save)
  and re-resolves `UserSettings`/the active model between recording sessions, without a
  process restart — more invasive, touches `EagerChildBackend` and/or however
  `runEagerDaemon` is structured, but avoids losing daemon state (mode, in-flight
  telemetry) across a restart.
- Out of scope: closing [[113]]'s remaining wiring gaps (history/modifier gating actually
  being read) — this ticket is purely about propagating a *already-wired* setting change
  into a running process.

## 3. Acceptance Criteria

- Changing the ASR model (or another daemon-relevant setting) via `voxi settings` and
  saving takes effect for the next recording session without the user manually running
  `systemctl --user restart voxi-agent.service`.
- If the chosen approach is restart-on-save, verify it doesn't drop or corrupt an
  in-flight recording/delivery (check `internal/eager`'s drain/delivery-ledger logic
  before triggering the restart).
- `voxi settings`/`voxi status`/diagnostics should not claim a setting is active when the
  running daemon is still using a stale value (if any window exists where that's still
  possible under the chosen approach, document it explicitly).

## 4. Verification Guidance

- Live-verify with a real `voxi-agent.service` instance: change the ASR model, save,
  and confirm (e.g. via `ps aux | grep 'voxi eager'` showing the new `--model`, or a
  fresh `voxi chunks list` entry's `model`/`engine` fields) that the *next* dictation
  session actually uses the new value — not just that the config file changed.
- If restart-on-save is chosen, verify behavior when triggered mid-recording (does it
  wait for the current utterance to finish, or interrupt it?).
