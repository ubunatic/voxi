# Live Mic-Loudness Meter: `audiolevel`, Ballistics, and Decoupled Redraw

Reference for the `audiolevel` package, its wiring into `internal/monitor`, and the
`spec/monitor.yaml`/`spec/actions.yaml` tuning around it. Read this before touching
`RenderLevelChar`, `ApplyBallisticsEased`, the paint/collect loop split in
`internal/monitor/monitor.go`, or the GNOME/PipeWire mic-indicator suppression tags —
several of the choices here were reached by measuring a real regression, not derived
from a formula you'd get right by guessing.

## 1. Purpose

`voxi monitor -w`'s `[s] voice & speed` box shows a live loudness glyph in its status
icon: a single colored, height-varying character that tracks the mic's input level in
real time, replacing the static `●`/`○` recording indicator (issue 084). Optionally
(`spec/monitor.yaml`'s `status_icon.always_show_loudness`, default `true`, issue 086)
it's shown even while idle, not just while actively recording — "is my mic even
picking up sound" without having to start dictating first.

## 2. Capture: `audiolevel` package (`audiolevel/audiolevel.go`)

A standalone package (no dependency on `internal/monitor` or `internal/eager`) so it
could in principle be reused by the sibling `harnez` project, which this was ported
from (`harnez/internal/usage/miclive.go`, `harnez/docs/MicIndicators.md`).

- **Capture backend**: `ParecCommand`/`PwRecordCommand` build a subprocess invocation
  (`parec` preferred, `pw-record` fallback) streaming raw PCM16LE mono at a low sample
  rate (`spec/monitor.yaml`'s `mic_level.sample_rate_hz`, default 8000 — a level meter
  needs a coarse amplitude reading, not fidelity). `RunCapture` reads fixed-size chunks
  (`mic_level.chunk_ms`, default 50ms) and feeds each through `AmplitudeFromPCM16LE`,
  which reuses `internal/audio.ComputeAudioRMS` but scales the result logarithmically
  in dBFS (`DefaultMinDBFS = -60`), not voxi's own `sparklineFloorRMS`/
  `sparklineCeilingRMS` linear calibration used by `voxi chunks list` — the two scales
  are calibrated against different reference points and are not drop-in compatible
  (see [ChunkDiagnostics.md](ChunkDiagnostics.md) for that other scale).
- **`Meter`**: mutex-guarded handoff between the background capture goroutine (the only
  writer, via `Update`) and any number of readers (`Snapshot`/`Tick`). `Manager` owns
  one capture goroutine's lifecycle (`StartManager`/`Stop`), auto-restarting the
  subprocess with backoff if it exits early (source unplugged, PipeWire restart).

## 3. Ballistics: why `ApplyBallisticsEased` exists, not just `ApplyBallistics`

`ApplyBallistics` (the original, ported-from-harnez function) is a textbook VU-meter
convention: **instant attack** on a rising level (snaps straight to the new, louder
target), **smooth exponential decay** on a falling one. That's the physically-accurate
choice for a dedicated meter display — but for voxi's compact single-glyph status icon,
it read as a distracting pop/jump rather than motion, confirmed by the user directly
comparing a zoomed screencast of the icon against Claude Code's own much smoother
animated UI indicator.

`ApplyBallisticsEased(prev, target, dt, attack, decay)` eases **both** directions —
the standard one-pole RC-charge formula,
`eased = target + (prev-target) * exp(-dt/tau)` (`tau` = `attack` when rising, `decay`
when falling) — never snapping except once within `DefaultDecayCutoff` of the target
(so it still settles in finite time rather than trailing asymptotically forever).
`ApplyBallistics` itself was **left untouched** (existing tests, existing behavior) —
`Meter.Update`/`Meter.Tick` were switched to call the new function instead, so any
other consumer wanting real-VU-meter instant attack still has it available.

Tuned via `spec/monitor.yaml`'s `mic_level.attack_ms`/`decay_ms` (60ms/100ms as
shipped) — short enough to feel responsive, long enough to always show intermediate
steps rather than a single-frame jump.

## 4. The actual "laggy" root cause: `Meter.Tick`, not ballistics tuning

Before eased ballistics were even the issue, the meter felt "laggy" for a more basic
reason: `Meter.Update` only advances `displayedLevel` once per captured audio chunk
(e.g. every 50ms). A redraw loop painting faster than that (30fps ≈ 33ms/frame) was
reading the *exact same* `Reading` from `Snapshot` for one or two frames in a row, then
jumping — a stair-stepped stutter, not smooth motion, regardless of how the ballistics
math itself was tuned.

Fix: `Meter.Tick(now, attack, decay)` continues the *same* easing curve against
wall-clock time, using the last window-reduced target (`lastTarget`, a new `Meter`
field set by `Update`), independent of when the next real audio sample arrives.
`internal/monitor/monitor.go`'s paint loop calls `Tick` every frame instead of
`Snapshot`. `Update` and `Tick` share one continuous decay/attack chain (both advance
`lastUpdate`), so a real sample arriving mid-tick continues smoothly rather than
resetting.

**Verification pattern used** (no real mic hardware needed): a fake `pw-record`
shell/Python script writing synthetic PCM16LE noise bursts to stdout, placed first on
`PATH`, run under `go build -race`. Confirmed 113/117 consecutive paint frames showing
a distinct color after the fix (vs. 83/117 before `Tick`, vs. long static holds before
either fix) — see the git history on `internal/monitor/monitor.go`/
`audiolevel/audiolevel.go` for the exact commits and measured numbers.

## 5. Rendering: truecolor gradient, not fixed color bands

`RenderLevelChar` maps level (0-100) to one of 8 Unicode block-height glyphs
(`▁▂▃▄▅▆▇█`, matching the sparkline runes used elsewhere) **and** a continuous 24-bit
ANSI truecolor gradient (`levelColor`: green → yellow → red, interpolated per-percent),
not a small number of fixed color bands. An earlier fixed-3-band version (green
<65%, yellow 65-90%, red ≥90%) looked frozen for over a second at a time in a real
zoomed screencast even while the underlying signal was genuinely moving — most natural
speech loudness sits well inside one band, so only the 8 discrete height steps carried
any visible motion; the continuous gradient gives near-per-percent visual resolution on
top of that.

Assumes the terminal supports 24-bit truecolor (`\x1b[38;2;R;G;Bm`) — true for GNOME
Terminal/Ptyxis/most modern terminals in 2026; no fallback to a reduced palette is
implemented.

## 6. Decoupled redraw: paint vs. collect vs. capture, and the `stty` cost

Three independent cadences, none blocking the others:

- **Capture** (`audiolevel`'s own goroutine): reads chunks at `mic_level.chunk_ms`
  (~20Hz), computes RMS/ballistics, entirely decoupled from anything below.
- **Collect** (`internal/monitor/monitor.go`'s `collectTicker`): the slow stuff —
  `systemctl show`, `/proc` walks, GPU sysfs reads, agent status RPC — runs on its own
  ticker paced by the user-facing `--interval` flag (default 1s), writing into a
  mutex-guarded cache. Never runs on the paint path.
- **Paint** (`paintTicker`, `spec/monitor.yaml`'s `paint.fps`, default 30): only ever
  reads the collect cache plus `micMgr.Tick(...)` — never triggers or waits on a data
  fetch, so a slow `systemctl` call can't stall or corrupt a redraw, and the meter can
  redraw far faster than `--interval` without extra data-source load or affecting
  CPU/GPU polling rate.

**A real, measured cost found *while verifying* the above**: `getTerminalWidth()` used
to shell out to `stty -F /dev/tty size` on every call to `PrintVoiceResourceReport` —
harmless at the monitor's original ~1fps, but at 30fps this was spawning ~150
subprocesses over 5 seconds. Measured via `/proc/<pid>/stat` utime+stime: ~8.2% of a
CPU core, roughly 3x more expensive than the mic meter's own audio processing (~2.8%).
Fixed by (a) querying via `golang.org/x/term.GetSize` (a direct `TIOCGWINSZ` syscall,
already a dependency in this repo — `internal/chunks/color.go`,
`internal/devsample/lineedit.go` — for `IsTerminal`/`MakeRaw`/`Restore`) instead of a
subprocess, and (b) caching the result, refreshed only on an actual terminal resize
(`SIGWINCH`) rather than every frame. Total CPU for the watch loop dropped from ~55
ticks to ~8 ticks over the same 5s measurement window. **The same anti-pattern was
found in harnez's own `internal/usage/watch.go`** (which even has a comment noting a
syscall-based approach is "more reliable than shelling out to `stty`" and then falls
back to shelling out to `stty` anyway) — filed as harnez issue 286 to promote
`golang.org/x/term` in that project's Go conventions doc, not just fixed locally here.

## 7. Input safety: debouncing dotool-injected keystrokes

Unrelated to the meter itself but discovered and fixed in the same work: `voxi
monitor -w`'s raw-mode `/dev/tty` key reader (`s`/`h`/`t`/`d`/`a`/`q` toggles,
resolved via `spec/actions.yaml` — see [Spec.md](Spec.md)) treated every single byte
as a deliberate keypress. `dotool` injects keystrokes into whatever window has OS
focus — if that happens to be the monitor's own terminal while the user dictates, every
letter of the sentence got fed into the toggle dispatcher, instantly quitting on a
stray `q` or spamming section toggles. Fixed with a 150ms debounce (`monitor.go`): a
keypress fires only after that much silence follows it; two or more bytes arriving
faster than that are dropped as burst noise. Verified with a pty harness feeding
`"quit disaster"` at ~5ms/char (same order of magnitude as dotool's injection rate) —
the monitor stayed alive and no section toggled, while a genuine isolated keypress
after a pause still worked.

## 8. Privacy-indicator suppression: confirmed vs. inferred

Both `ParecCommand` and `PwRecordCommand` tag their capture stream with
`application.id=org.gnome.VolumeControl` and `node.virtual=true`
(`SuppressApplicationID` in `audiolevel.go`), meant to exempt the meter from desktop
"microphone in use" indicators the way GNOME's own Settings→Sound input meter and
`pavucontrol` do.

- **GNOME**: confirmed by directly reading gnome-shell's current upstream source
  (`js/ui/status/volume.js`'s `InputStreamSlider._maybeShowInput()`) — it hardcodes
  exactly `['org.gnome.VolumeControl', 'org.PulseAudio.pavucontrol']` as the skip-list,
  checked against `application.id` only. `pw-record`'s `-P`/`--properties` flag
  (confirmed present on 1.6.2, confirmed live via `pw-dump` to actually land the tag on
  the resulting PipeWire node) closed the gap where the `pw-record` fallback path
  previously carried no suppression tags at all.
- **Not yet confirmed**: whether this machine's actual (possibly Ubuntu-patched) GNOME
  Shell version still honors that allowlist unmodified, and whether the top-bar
  indicator dot genuinely stays dark during a real session — a `pw-dump` showing the
  property landed on the node is not the same as a human watching the actual indicator.
  KDE's `node.virtual=true` exemption is inherited from harnez, never independently
  canaried by either project. Tracked in
  [issue 087](../issues/087-voxi-monitor-lights-up-gnome-mic-in-use-indicator-even-while-idle-suppression-tags-don-t-work-unimplemented.md).

## 9. Spec-driven tuning

All of §3/§4/§6's tunables live in `spec/monitor.yaml` (schema:
`spec/schemas/monitor.schema.json`, loader: `spec.LoadMonitor`/`spec.MonitorSpec`), not
hardcoded Go constants — see [Spec.md](Spec.md) for why. `internal/monitor/monitor.go`
caches the parsed spec once via `sync.OnceValue` (`loadedMonitorSpec`).

| Field | Controls |
|---|---|
| `paint.fps` | Screen redraw cadence (§6) |
| `mic_level.sample_rate_hz` / `chunk_ms` | Raw capture rate/chunking (§2) |
| `mic_level.window_ms` | Rolling-window smoothing before ballistics |
| `mic_level.attack_ms` / `decay_ms` | Eased-ballistics time constants (§3) |
| `status_icon.always_show_loudness` | Idle-state icon behavior (§1, issue 086) |

## 10. Where the code lives

| Concern | File |
|---|---|
| Capture, `Meter`/`Manager`, ballistics | `audiolevel/audiolevel.go` |
| Monitor wiring: capture lifecycle, key debounce, collect/paint loop split, terminal-width cache | `internal/monitor/monitor.go` |
| Status-icon glyph/gradient rendering, box titles/footer from spec | `internal/monitor/render.go` |
| `mic_level`/`paint`/`status_icon` tuning | `spec/monitor.yaml`, `spec/monitor.go` |
| Monitor hotkeys (single source of truth for the key debounce's dispatch table) | `spec/actions.yaml`, `spec/actions.go` |

Tests: `audiolevel/audiolevel_test.go` (ballistics both directions, `Tick`/`Update`
continuity, `PwRecordCommand` suppression-tag args), `spec/monitor_test.go` (spec
validation, derived duration/byte-size helpers), `internal/monitor/monitor_test.go`.

## 11. Related issues

[084](../issues/084-add-live-mic-input-level-meter-and-volume-display-to-voxi-monitor.md)
(original feature request, closed here), 
[086](../issues/086-detailed-view-always-show-live-mic-loudness-even-when-not-recording.md)
(`status_icon.always_show_loudness`, closed),
[087](../issues/087-voxi-monitor-lights-up-gnome-mic-in-use-indicator-even-while-idle-suppression-tags-don-t-work-unimplemented.md)
(privacy-indicator suppression, open — GNOME-source-confirmed, live-canary pending).
