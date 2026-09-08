# 088 — Elevate OS scheduling priority for the transcription-critical path

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Enhancement
**Category**: Enhancement
**Related**: [internal/eager/eager.go](../internal/eager/eager.go), [systemd/voxi-agent.service](../systemd/voxi-agent.service), [084 Add Live Mic Input-Level Meter and Volume Display to `voxi monitor`](084-add-live-mic-input-level-meter-and-volume-display-to-voxi-monitor.md)

---

## 1. Background

While recording a demo screencast on 2026-09-08 (`docs/NeovimTest.md` was
the recording script; the resulting clip is `website/voxi-demo.mp4`), the
user had `voxi monitor -w` running side-by-side with Neovim while
dictating and *felt* that transcription was a bit slower than usual,
plausibly due to system load. **This is a subjective, felt-latency report
from one session, not a measured benchmark** — no timing numbers were
captured, and the load itself was partly self-inflicted by screen
recording (real CPU/GPU cost) plus the monitor's own live mic-loudness
meter and redraw loop (added earlier this session, see 084 and follow-on
ballistics/perf commits `c665775`, `32b7586`, `6e53f3a`). That "recording
+ `voxi monitor -w` open simultaneously" combination is itself a second,
independent contention source worth naming as a specific reproducible
scenario, distinct from generic "other processes on the system."

Not in scope here: `voxi-modifierd` was separately found to be
not-installed on this machine during the same session — that's an
unrelated, already-reported gap, not a scheduling-priority issue.

## 2. Request

Investigate giving voxi's transcription-critical path (the ASR inference
subprocess — `crispasr` by default, or `voxtype` for Whisper-alternative
models, invoked via `exec.CommandContext` in `internal/eager/eager.go`
around the `transcribeBinPath`/`cmdArgs` call, currently launched with no
priority handling of any kind) elevated OS scheduling priority, so that
background system load doesn't cause dictation transcription to lag
behind other work competing for the CPU.

## 3. Current State

- `internal/eager/eager.go` launches the transcription subprocess via
  plain `exec.CommandContext(transcribeCtx, transcribeBinPath, cmdArgs...)`
  — no `nice`, `ionice`, `SCHED_*`, or cgroup weight is applied anywhere
  in the launch path today.
- `voxi-agent.service` (`systemd/voxi-agent.service`), the systemd --user
  unit that runs the eager daemon (and therefore spawns the transcription
  subprocess as its child), has no `Nice=`, `CPUWeight=`, `IOWeight=`, or
  other resource-control directive set — it runs at default priority
  under `SCHED_OTHER` like any other user process.
- `voxi monitor -w`'s own mic-level meter capture and render loop (084)
  run as separate processes/goroutines competing for the same CPU during
  exactly the scenario that prompted this report.

## 4. Open Questions — mechanism (not decided here)

Several Linux mechanisms could address this; each has real tradeoffs, and
this ticket does not commit to one. Pick during implementation:

- **`nice`/`renice` (CPU scheduling priority)**: simplest, but the
  inference process already gets fair `SCHED_OTHER` scheduling under
  normal load. A meaningfully negative nice value needs `CAP_SYS_NICE` or
  privileges most desktop processes don't have, and being too aggressive
  here risks starving other desktop/UI work — a tradeoff, not a free win.
- **`ionice` (I/O scheduling class/priority)**: more relevant if the
  actual contention is disk I/O (model weight loading, swap pressure)
  rather than CPU — worth checking which is actually the bottleneck
  before reaching for this.
- **systemd unit directives (`Nice=`, `CPUWeight=`, `IOWeight=` in
  `voxi-agent.service`)**: likely the cleanest mechanism, since the
  transcription engine already runs as a child of a systemd-managed
  daemon in the common install path — declarative, no code change needed
  at every subprocess launch site. Doesn't help for any invocation path
  that bypasses the service (if one exists).
- **cgroups directly**: more control but more complexity than the
  systemd-directive equivalent above; probably only worth it if unit
  directives prove insufficient.

## 5. Open Question — default vs. opt-in

Raising the transcription path's priority is a tradeoff against overall
desktop responsiveness, not a strictly-better change for every user/setup
— should this be:
- a shipped default (accepting some desktop-responsiveness cost for
  consistently-fast dictation), or
- an opt-in flag/config (e.g. a `spec/`-driven toggle), leaving default
  behavior unchanged until a user opts in?

Not decided here; the answer plausibly depends on which mechanism from
§4 is chosen (e.g. a `CPUWeight=` bump is a much gentler default than a
negative `Nice=`).

## 6. Non-Goals

- Not implementing any of the above — this ticket is investigate-then-
  implement, mechanism undecided.
- Not about `voxi-modifierd` being uninstalled (separate, already-known
  gap).
- Not claiming a measured performance regression — the motivating report
  is anecdotal, felt latency during one specific recording session.
