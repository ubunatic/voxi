# 088 — Elevate OS scheduling priority for the transcription-critical path

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Enhancement
**Category**: Enhancement
**Related**: [internal/eager/eager.go](../internal/eager/eager.go), [systemd/voxi-agent.service](../systemd/voxi-agent.service), [084 Add Live Mic Input-Level Meter and Volume Display to `voxi monitor`](084-add-live-mic-input-level-meter-and-volume-display-to-voxi-monitor.md), [052 CPU/GPU priority research (closed, folded in here)](052-cpu-gpu-priority-under-load.md)

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

## 7. Investigation section (folded in from closed issue 052)

052 asked the same underlying question — CPU/GPU scheduling priority for
responsive dictation under load — against the same launch path
(`voxi-agent.service`), before this ticket's more concrete, incident-driven
framing existed. Closed as a duplicate track rather than run in parallel;
its research questions and probes are preserved here as the investigation
material for §4/§5 above, still undecided/not started.

### 7.1 CPU priority & scheduling
- systemd user-service directives: `Nice=-10`/`-5` (needs `LimitNICE`/PAM
  permission in the user session), `CPUSchedulingPolicy=rr|fifo` vs
  `batch|other`, `LimitNICE=`. Check permission limits for unprivileged
  user services under standard systemd (Ubuntu/Fedora default
  `/etc/security/limits.conf` or polkit).
- Process inheritance: does `exec.CommandContext`'s child (`voxtype`/
  `crispasr` and its OpenMP/pthread pool) cleanly inherit nice/scheduling
  class? Can Go set `SysProcAttr` or call `syscall.Setpriority` explicitly
  before exec?
- cgroups v2: does the systemd user slice support `CPUWeight=`/
  `StartupCPUWeight=` for a proportional CPU-share guarantee under
  contention?

### 7.2 GPU priority & compute preemption
- Vulkan/RADV (AMD, the host GPU's compute path): `VK_EXT_global_priority`/
  `VK_EXT_global_priority_query` (`HIGH`/`REALTIME` queue priority;
  `REALTIME` typically needs `CAP_SYS_NICE` or DRM permissions `HIGH` may
  not). Check Mesa/RADV env vars or DRI render-node scheduling controls.
- DRM/kernel GPU scheduling: `amdgpu_sched`'s priority rings (low/normal/
  high/real-time) — can an unprivileged client request a high-priority DRM
  context?
- Does upstream `whisper.cpp`/`voxtype`/`crispasr` expose any queue-priority
  or compute-stream configuration? Does a resident warm-model context (050/
  051) reduce GPU bus contention/pipeline stalls from buffer reloads under
  memory-bandwidth pressure, independent of scheduling priority?

### 7.3 Audio capture priority (PipeWire/ALSA)
- Under high CPU load, capture-side xruns can drop audio before
  transcription even starts. Does Voxi's capture path get real-time
  priority via `rtkit`? `PIPEWIRE_LATENCY` tuning? How to verify the
  capture path is protected against xruns under load, independent of the
  transcription-process priority this ticket otherwise targets.

### 7.4 Canary probes carried over
- `Nice=-5`/`-10` in `voxi-agent.service`, `systemctl --user daemon-reload`,
  check journal for permission errors; verify child nice levels via
  `ps -eo pid,ni,comm | grep -E 'voxi|voxtype|crispasr'`.
- Synthetic load benchmark: `stress-ng --cpu 8 --io 4` (or a GPU compute
  loop) concurrent with `voxi feedback sample play`/transcribe on a fixed
  corpus (e.g. `kt-sentences-plus-silence`); compare RTF/latency at default
  vs. elevated priority — this is the measurement §5's default-vs-opt-in
  decision should be gated on, not a felt-latency report.
- `vulkaninfo | grep VK_EXT_global_priority` on the host GPU; check whether
  an unprivileged process can actually open a high-priority context.
- Document any required `/etc/security/limits.d/` ceiling if `LimitNICE`
  needs raising beyond the default.

## 8. Sprint Disposition (2026-09-10)

No scheduler change is justified yet. Existing telemetry has useful latency
values but no CPU/GPU-load marker or priority correlation, and the live service
is currently at `Nice=0` with `LimitNICE=0`. The next step is a controlled
idle-versus-loaded canary with fixed utterances and telemetry/process snapshots;
test `CPUWeight` before attempting negative nice values. Keep this issue open.
