# Voice Input

`voxi` is a standalone Linux/Wayland voice input, continuous eager sentence streaming, and
desktop typing engine. It installs and runs independently (`make install`) — there is no
package catalog or `--dry-run`/`--scope` installer; see [Installation](#installation) below.
For the multi-tier desktop companion, Mutter focus coordination, and Wayland input
architecture, see [VoiceInputArchitecture.md](VoiceInputArchitecture.md). For per-model
hallucination filtering, GPU requirements, and CPU fallback, see
[Spec-Driven Development](Spec.md) and `spec/models.yaml` directly. For CPU vs GPU
transcription speed numbers, see [BenchBaseline.md](BenchBaseline.md).

> **Note on history:** `voxi` was extracted from a larger harness project (`harnez`) where
> it originated as `harnez tools voice-input ...`. Commands, binary names
> (`harnez-modifierd` → `voxi-modifierd`), and state paths (`/run/harnez/modifiers` →
> `/run/voxi/modifiers`) below are current for the standalone project. `voxi` still reads
> the legacy `/run/harnez/modifiers` path as a fallback for one release cycle.

## Current status

- No tagged release yet — install from `main` via `make install` (see
  [Installation](#installation)).
- ASR backends are separate, independently installed binaries that `voxi` orchestrates but
  does not vendor: `crispasr` (Cohere Transcribe, the default eager engine) and `voxtype`
  (Whisper batch/streaming, and eager when an explicit Whisper `--model` is selected).
  `voxtype` is not required for the default eager path; see spec/models.yaml's `engine`
  field per model and issue 077.
- The Fedora 44 GNOME Wayland canary fully passed: microphone capture, local `base.en`
  transcription, the global GNOME toggle shortcut, and direct text injection at the
  focused cursor (correct on a German QWERTZ layout) via a user-level `dotool`+`dotoold`
  daemon — not `eitype` (portal-dialog authorization) or `ydotool` (no XKB awareness).
- Never uses cloud transcription or requests membership in the `input` group.

## Installation

```sh
git clone https://codeberg.org/ubunatic/voxi
cd voxi
make install                 # user binaries (~/go/bin)
make install-user-services   # systemd user services (voxi-agent.service, compatibility eager unit)
sudo make install-modifierd  # optional: system-wide voxi-modifierd daemon
```

There is no uninstall command yet; `make uninstall` removes installed binaries and the
system modifier daemon but does not track ownership of files it did not create.

## Commands

```text
voxi mode                     # show the active mode (batch/streaming/eager/neither/inconsistent)
voxi mode eager                   # switch to continuous eager sentence streaming (issue 026)
voxi mode streaming               # switch to opt-in local streaming (issue 021)
voxi mode batch                   # switch back to default batch flow
voxi record status                # show active dictation status (idle or recording)
voxi record toggle                # universal toggle across active mode (batch/streaming/eager)
voxi record start                 # start active voice recording
voxi record stop                  # stop active voice recording
voxi eager [--type] [--history] [--daemon] [--model <name>]  # run eager dictation directly
voxi monitor                  # print daemon health, memory, GPU Vulkan accel, and processes
voxi monitor --watch          # live updating resource monitor (aliases: top, resources, stats)
voxi telemetry query          # correlated Eager chunk lifecycles (text; add --format json)
voxi telemetry stats          # aggregate latency, backlog, audio, and outcome statistics
voxi bench [--models list] [--backends cpu,gpu] [--record] [--file wav] [--json path]
                               # CPU vs GPU RTF/speedup per model, see BenchBaseline.md
voxi history list                 # list recent dictations (most recent first, sensitive)
voxi history copy <ID>            # copy transcript to clipboard via wl-copy
voxi history retype <ID>          # re-type transcript at cursor via dotool
voxi history clear                # wipe local history file
voxi history record               # stdin/stdout pass-through hook for Voxtype
voxi config get type-delay-ms     # read type_delay_ms from ~/.config/voxtype/config.toml
voxi config set type-delay-ms <MS> # edit type_delay_ms preserving comments
voxi daemon modifier-service      # run voxi-modifierd directly (normally a system service)
```

Until issue 029 is implemented, `voxi mode` toggles between these mutually exclusive
systemd user services:
- `voxtype.service` for batch mode (`base.en` Whisper, typed at end of utterance)
- `voxtype-streaming.service` for opt-in streaming (Parakeet ONNX, typed incrementally)
- `voxi-eager.service` for continuous eager sentence streaming (rolling Whisper inference, 0 pause drops)

`voxi record toggle` acts as a universal toggle for all 3 modes, allowing a single global
shortcut (`Super+X`) to control whichever mode is currently active.

### Eager telemetry queries

Eager mode appends private, transcript-free events to
`${XDG_DATA_HOME:-~/.local/share}/voxi/eager-telemetry.jsonl`. Inspect correlated chunks
with `voxi telemetry query`, raw rows with `--view events`, or microphone/capture
lifecycles with `--view sessions`. `--session`, `--chunk`, `--event`, `--success`,
`--silence`, and `--post-deactivation` provide exact filters; `--since` and `--until`
accept RFC3339 timestamps with an explicit timezone and are inclusive. Use `--path` for
offline fixtures and `--format json` for stable machine-readable output. The default
`--max-events 100000` is a hard memory bound; narrow the time range or raise it
explicitly when necessary.

`voxi telemetry stats` reports robust latency percentiles, post-deactivation backlog,
audio duration/bytes, probable-silence chunks, transcript word counts, stage outcomes,
and transcription real-time factor where both audio and timing data exist. Missing
stages remain absent in JSON and count as partial lifecycles. Malformed and unsupported
newer-schema rows are skipped but reported in every result. Timestamps retain their
recorded offsets; derived durations compare absolute instants. Neither command modifies
the database or exposes transcript text.

Typing completion has one important boundary: the synchronous `dotool` fallback has
returned, but `dotoolc` completion only means its command stream was accepted by the
pipe. The external daemon provides no per-keystroke acknowledgement.

### Single-agent migration (issue 029)

The target design is one always-enabled `voxi-agent.service`. It owns the control socket,
selected mode, recording state, and backend lifecycle; `voxi mode` and `voxi record` talk to
the agent instead of switching systemd services. `make install-user-services` installs the
agent unit alongside the current eager compatibility unit. The first usable migration slice
runs eager through an agent-owned child `voxi eager --daemon`; batch and streaming remain on
the legacy service path until their backend adapters are implemented.

## Model selection & CPU/GPU behavior

Every model `voxi` can drive through Voxtype (`base.en`, `small.en`, `large-v3-turbo`) is
declared in `spec/models.yaml` — the single source of truth, schema-checked against
`spec/schemas/models.schema.json` and embedded into the binary. Each model owns its own
list of hallucination stop-word patterns (phrases Whisper reliably invents on silence,
e.g. "thanks for watching"), filtered out before anything is typed.

A model may declare `requires_gpu: true` when it is measurably sub-realtime on CPU
(`large-v3-turbo` does — see BenchBaseline.md). When GPU is required and no GPU render
node is present, `voxi eager` does not hard-fail: it substitutes that model's
`cpu_fallback` (currently `small.en`, also the default model) and prints a notice, rather
than either running too slow to keep up with live speech or refusing to start.

Run `voxi bench` to measure RTF (real-time factor) and speedup for every configured model
on both backends against a fixed, checksum-verified reference clip (downloaded on demand,
never committed — see `internal/bench`), or against `--record`ed live audio / a `--file`.

## Security note: uinput access is not gated by voice input

`dotool` (and `ydotool`) write to `/dev/uinput` to synthesize keyboard input. On a typical
desktop `/dev/uinput` already carries a `udev` `uaccess` tag, which is systemd-logind's
standard mechanism for granting the active local desktop session read/write access to
input devices (the same mechanism used for `/dev/dri`, `/dev/snd`, webcams, Steam Input,
and accessibility tools). This means **any process running as the logged-in user already
has direct, silent, kernel-level keyboard/mouse injection capability, independent of
whether voice input or its typing backend is installed.** A start/stop toggle for
`dotoold` (e.g. a GNOME Quick Settings button) would not close this: `systemctl --user`
requires no privilege beyond the same user account, so anything that could abuse the
running daemon could equally re-enable it or bypass it via `/dev/uinput` directly. Do not
build or ship such a toggle as a security control; it provides no real boundary and only
gives false comfort. The one mechanism here that requires genuine per-use human consent is
`eitype`, which routes through Wayland's XDG RemoteDesktop portal — rejected in this setup
because of the recurring authorization dialog, a deliberate convenience-over-consent
tradeoff the user accepted knowingly. Meaningfully closing this gap would require removing
the `uaccess` tag from `/dev/uinput` system-wide, which is out of scope for `voxi` and
would break other legitimate uses (Steam Input, accessibility tools) unless done
carefully.

## Hardware canary

The Fedora 44 GNOME Wayland canary fully passed as of 2026-08-17. Microphone capture,
local transcription, the `Super+X` toggle, and direct text injection (verified in
a terminal and in Prime Agent's own input field) all work. GNOME text injection needed
a user systemd `dotoold` daemon for the fast, reliable `dotoolc` path, plus
`language_to_layout = {}` in Voxtype's config to stop it auto-forcing `layout=us` for
English speech regardless of the physical keyboard layout. The script can repeat the
guided manual test; ordinary `go test` never runs it.

As of issue 081, the `dotoold` daemon setup above is no longer manual: `make
install-dotoold` (folded into `install-all`) installs the `dotoold`/`dotoolc` scripts
shipped alongside `dotool`, installs `systemd/dotoold.service`, and runs `systemctl
--user enable --now dotoold.service`. The keyboard layout (`DOTOOL_XKB_LAYOUT`) is no
longer hardcoded to `de` — it is auto-detected per machine from `localectl status`'s
X11 Layout at install time, overridable with `make DOTOOL_XKB_LAYOUT=<layout>
install-dotoold`. This closed the last gap where telemetry reported `typing_completed:
success: true` even though nothing was typed: raw one-shot `dotool` exits 0 on an
ephemeral `/dev/uinput` device the compositor never reliably picks up, while the
persistent `dotoold` daemon (one stable device, fed via the `dotoolc` pipe) is the path
this canary actually validated.

## Streaming (opt-in — see issue 021)

Local streaming partial-typing (Parakeet via ONNX Runtime, no GPU required) was proven
working on this same workstation as of 2026-08-17: words appear incrementally during
dictation via the existing `dotoolc` fast path, still fully offline. It is **not** the
default; switching to it requires a one-time setup and then `voxi mode streaming`:

1. One-time setup (not automated by `voxi`/`make install` yet): install the
   `onnx-avx2` voxtype binary, download the streaming-capable model
   (`voxtype setup --download --model parakeet-unified-en-0.6b --quiet`, ~2.7GB), and
   create `~/.config/voxtype/config-streaming.toml` plus a
   `~/.config/systemd/user/voxtype-streaming.service` unit pointed at it (analogous to
   `voxtype.service`, but not enabled for auto-start). See issue 021's Findings section
   for the exact steps and the footguns hit along the way (a `voxtype setup --download`
   side effect that silently switches the live engine config, an undocumented
   streaming-timing constraint, and a PATH gap in ad hoc systemd units).
2. Day to day: `voxi mode streaming` / `voxi mode batch` / `voxi mode` (see Commands
   above) — no manual `systemctl`/two-terminal juggling needed. During the issue 029
   migration, the selected mode will move into the agent rather than systemd enablement.

Known upstream limitation (Voxtype 0.7.5, not a voxi bug): pauses in speech cause
multi-second output lag and occasionally drop words, because this streaming pipeline has
no VAD/end-of-utterance segmentation yet. See issue 021 for the debug-log analysis.
Enter-to-stop and deeper backtracking behavior remain open, see issue 021.

## History & Config Design Decisions (Issue 022)

### 1. Pass-Through History Capture (`history record`)
Voxtype's `[output.post_process]` configuration hook executes an external command with the
transcribed text on `stdin` and reads the replacement text from `stdout` before passing it to
the typing driver.

Rather than patching Voxtype upstream or running an intrusive background daemon,
`voxi history record` acts as a pass-through filter (a recording `cat`):
- Reads the transcript from `stdin`.
- Computes an 8-char SHA-256 ID, timestamps the entry, and appends it to
  `~/.local/share/voxi/history.jsonl` (mode `0600`, capped at 20 entries).
- Writes the exact text back to `stdout` unchanged.

Wiring into `~/.config/voxtype/config.toml`:
```toml
[output.post_process]
command = "voxi history record"
```

### 2. Comment-Preserving In-Place Config Editing (`config set`)
Voxtype's own `voxtype config set` subcommand only supports the `engine` key and omits
`type_delay_ms`. To avoid destructive TOML round-trips that would strip user comments or
reformat spacing in `config.toml`, `voxi config set type-delay-ms <MS>` uses an atomic,
line-targeted regex replacement that modifies only the numeric delay in place.

## Debug & Tuning Harness (Build Tag: `debug`)

Experimental diagnostics and prototyping tools are gated behind Go's `//go:build debug` tag to keep standard release binaries clean. Build with `make build-debug` or `make install-debug`:

```text
voxi canary     # real-time streaming token observer & parameter tuner
voxi vad-probe  # prototype VAD-segmented sentence-by-sentence dictation
```

### 1. Streaming Canary (`canary`)
- Spawns an isolated Voxtype daemon with configurable chunk/context parameters.
- Intercepts streaming keystrokes via a virtual `dotool` pipe, measuring per-chunk delta timing and highlighting inter-word pause gaps.
- Empirically verified the upstream Parakeet streaming pause bug: natural 1–5s pauses saturate the context cache with silence, producing a 4–11s lag and dropping the initial post-pause word ("One").

### 2. VAD Sentence Dictation Probe (`vad-probe`)
- Continuously buffers 16kHz audio with a 250ms circular pre-roll buffer (eliminating initial consonant loss).
- Segments utterances on 600ms silence and transcribes completed sentences with Whisper in <300ms.

## Continuous Eager Sentence Streaming (Issue 026)

To bridge the gap between high-accuracy batch Whisper and low-latency streaming without suffering Parakeet's pause-loss bug, Issue 026 implements **Continuous Eager Sentence Streaming** in native Go orchestration (`voxi eager --daemon`):
- **Continuous Rolling VAD Capture**: Captures 16kHz PCM audio via `pw-record` with a `500ms` circular pre-roll buffer and `350ms` post-roll audio padding, preserving leading unstressed words (*"The"*, *"A"*) and trailing unvoiced consonants (*"cat"*, *"six"*).
- **GPU Hardware Acceleration**: Runs official release `voxtype-0.7.5-linux-x86_64-vulkan` leveraging local AMD Radeon Cezanne iGPU via Mesa RADV compute shaders (`/dev/dri/renderD128`). Reduces transcription latency from $>2.5\text{s}$ CPU compute to $<250\text{ms}$ GPU compute ($10\text{--}15\times$ faster than realtime) — see BenchBaseline.md for the full per-model CPU vs GPU table.
- **Session Barrier & Kill Watcher**: Active context watcher terminates recording child processes in $<10\text{ms}$ on cancel; `sync.WaitGroup` session barriers prevent zombie processes and orphaned recording leaks.
- **Hallucination & Bias Rejection**: Filters model-specific hallucination stop-words (see `spec/models.yaml`) and strips URLs from transcribed text before typing.

## Resource Monitor TUI (`monitor --watch`)

`voxi monitor --watch` provides an interactive, btop-styled terminal dashboard tracking voice subsystem health, performance, and hardware load with zero flicker:

```text
╭─ [s] voice & speed ─────────────────╮ ╭─ [h] hardware load ──────────────────╮
│ status:  ○ idle (eager / small.en)  │ │ cpu:   1.4%  [ ▂  ▅ ▂   ]  avg 4.5%  │
│ engine:  AMD Radeon Vulkan 1.4      │ │ gpu:  12.0%  [  ▂ █ ▂   ]  sys 0.74% │
│ speed:   10.5x realtime [████████]  │ │ mem:   6.1 MB daemon   1.3/8.0G VRAM │
│ lag:     0.22s · 8m 8s audio (118)  │ │ up:   55m 6s (PID 723612)            │
╰─────────────────────────────────────╯ ╰──────────────────────────────────────╯
╭─ [t] transcript feed ────────────────────────────────────────────────────────╮
│ 11:42:08  [0.22s]  "repeating that"                                          │
│ 11:42:06  [0.25s]  "I will make a little pause between each step..."          │
╰──────────────────────────────────────────────────────────────────────────────╯
╭─ [d] active daemons & health ────────────────────────────────────────────────╮
│ dotoold (PID 498151, 3.3 MB)  ·  voxi (PID 723612, 13.0 MB)  ·  ✓ clean       │
╰──────────────────────────────────────────────────────────────────────────────╯
 [s]peed ●  [h]ardware ●  [t]ranscript ●  [d]aemons ●  │  [a]ll  [q]uit
```

### Interactive Letter Hotkeys:
- **`s`**: Toggle **`[s]peed`** & status panel.
- **`h`**: Toggle **`[h]ardware`** load panel (live CPU & GPU load sparklines, VRAM usage, and uptime).
- **`t`**: Toggle **`[t]ranscript`** live sentence feed.
- **`d`**: Toggle **`[d]aemons`** process list & health checks.
- **`a`**: Enable **`[a]ll`** panels.
- **`q`** / **`Esc`**: **`[q]uit`** monitor immediately.

### Anti-Overflow Guarantee:
- Dynamically queries terminal columns (`stty size` / `getTerminalWidth()`) and scales 2-column top boxes to match the terminal window.
- Clamps every line with ANSI-aware truncation (`TruncateLineANSI`) ensuring 100% pixel-perfect vertical border alignment (`│`) without text wrapping.

## Dedicated Modifier Daemon (`voxi-modifierd`)

To prevent synthetic keystrokes from clashing with held modifier keys (e.g. typing while the user is pressing `Ctrl`, `Alt`, `Super`, or `Shift`), `voxi` provides a dedicated physical modifier daemon:
- **Daemon (`voxi-modifierd` / `voxi daemon modifier-service`)**:
  - Discovers all physical keyboard devices via `/dev/input/event*` using `evdev` `EVIOCGBIT` ioctl.
  - Monitors physical modifier keys (`Left/Right Ctrl`, `Left/Right Alt`, `Left/Right Super`, `Left/Right Shift`).
  - Exports instantaneous modifier state as a 1-byte bitmask to `/run/voxi/modifiers` (`0644`).
  - **Zero Keylogging Guarantee**: Non-modifier keys are never inspected, recorded, or exported.
- **Client Reader (`ModifierReader`, `internal/modifiers`)**:
  - Single-byte file read (<10ns latency, zero IPC overhead).
  - Reads `/run/voxi/modifiers` first, falling back to the legacy `/run/harnez/modifiers` path.
  - Automatically gates typing in `internal/eager` and `internal/typing`, pausing keystroke emission until all physical modifiers are released.
- **System Installation**:
  ```bash
  sudo make install-modifierd
  ```
  Installs `/usr/local/bin/voxi-modifierd` and enables the systemd service `/etc/systemd/system/voxi-modifierd.service` (`DeviceAllow=char-input r`, `SupplementaryGroups=input`).
