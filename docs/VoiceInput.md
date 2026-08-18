# Voice Input

`harnez tools` is an explicit boundary for optional operating-system capabilities. It is
separate from global harness `apply` and project `init`; neither installs packages or
services. For the multi-tier desktop companion, Mutter focus coordination, and Wayland
input architecture, see [VoiceInputArchitecture.md](VoiceInputArchitecture.md).


## Current MVP

- `harnez tools` performs a read-only listing and always succeeds when the catalog loads.
- `harnez tools status [voice-input]` performs read-only probes; a named capability exits
  nonzero unless it is ready, making it suitable for scripts.
- `harnez tools install voice-input --dry-run` prints payload/model sizes, destinations,
  privileges, persistent changes, injection choice, and privacy implications.
- User scope is the default and never requires `sudo`; system scope is explicit.
- The Voxtype 0.7.5 AVX2 and RPM artifacts and their official GitHub release SHA-256
  digests are pinned in a strictly decoded embedded YAML catalog with a companion JSON Schema.
- A checksum verifier is implemented for the future user-local recipe. It installs atomically
  only when the destination is absent, returns no-change for identical bytes, and refuses to
  overwrite any other existing file. The gated command does not invoke it yet.
- The Fedora 44 GNOME Wayland canary fully passed: microphone capture, local `base.en`
  transcription, the global GNOME toggle shortcut, and direct text injection at the
  focused cursor (correct on a German QWERTZ layout) via a user-level `dotool`+`dotoold`
  daemon — not `eitype` (portal-dialog authorization) or `ydotool` (no XKB awareness).

## Commands

```text
harnez tools
harnez tools status voice-input
harnez tools install voice-input --dry-run
harnez tools install voice-input --scope user     # default; recipe pending
harnez tools install voice-input --scope system   # explicit privilege; recipe pending
harnez tools voice-input mode                     # show the active mode (batch/streaming/eager/neither/inconsistent)
harnez tools voice-input mode eager                   # switch to continuous eager sentence streaming (issue 026)
harnez tools voice-input mode streaming               # switch to opt-in local streaming (issue 021)
harnez tools voice-input mode batch                   # switch back to default batch flow
harnez tools voice-input record status                # show active dictation status (idle or recording)
harnez tools voice-input record toggle                # universal toggle across active mode (batch/streaming/eager)
harnez tools voice-input record start                 # start active voice recording
harnez tools voice-input record stop                  # stop active voice recording
harnez tools voice-input resources                 # monitor daemon health, memory, GPU Vulkan accel, and processes
harnez tools voice-input resources --watch         # live updating resource monitor
harnez tools voice-input history list                 # list recent dictations (most recent first, sensitive)
harnez tools voice-input history copy <ID>            # copy transcript to clipboard via wl-copy
harnez tools voice-input history retype <ID>          # re-type transcript at cursor via dotool
harnez tools voice-input history clear                # wipe local history file
harnez tools voice-input history record               # stdin/stdout pass-through hook for Voxtype
harnez tools voice-input config get type-delay-ms     # read type_delay_ms from ~/.config/voxtype/config.toml
harnez tools voice-input config set type-delay-ms <MS> # edit type_delay_ms preserving comments
```

The command never uses cloud transcription or requests membership in the `input` group.
There is no uninstall command until ownership records can distinguish harnez-managed
files from user-owned files.

`voice-input mode` toggles between three mutually exclusive systemd user services:
- `voxtype.service` for batch mode (`base.en` Whisper, typed at end of utterance)
- `voxtype-streaming.service` for opt-in streaming (Parakeet ONNX, typed incrementally)
- `harnez-voice-eager.service` for continuous eager sentence streaming (rolling Whisper inference, 0 pause drops)

`harnez tools voice-input record toggle` acts as a universal toggle for all 3 modes, allowing a single global shortcut (`Super+X`) to control whichever mode is currently active.

## Security note: uinput access is not gated by voice input

`dotool` (and `ydotool`) write to `/dev/uinput` to synthesize keyboard input. On this
workstation `/dev/uinput` already carries a `udev` `uaccess` tag, which is systemd-logind's
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
the `uaccess` tag from `/dev/uinput` system-wide, which is out of scope for `harnez tools`
and would break other legitimate uses (Steam Input, accessibility tools) unless done
carefully.

## Hardware canary

The Fedora 44 GNOME Wayland canary fully passed as of 2026-08-17. Microphone capture,
local transcription, the `Super+X` toggle, and direct text injection (verified in
a terminal and in Prime Agent's own input field) all work. GNOME text injection needed
a user systemd `dotoold` daemon (`DOTOOL_XKB_LAYOUT=de`) for the fast, reliable
`dotoolc` path, plus `language_to_layout = {}` in Voxtype's config to stop it
auto-forcing `layout=us` for English speech regardless of the physical keyboard layout.
The script can repeat the guided manual test; ordinary `go test` never runs it.

## Streaming (opt-in — see issue 021)

Local streaming partial-typing (Parakeet via ONNX Runtime, no GPU required) was proven
working on this same workstation as of 2026-08-17: words appear incrementally during
dictation via the existing `dotoolc` fast path, still fully offline. It is **not** the
default; switching to it requires a one-time setup and then `harnez tools voice-input
mode streaming`:

1. One-time setup (not automated by `harnez tools install` yet): install the
   `onnx-avx2` voxtype binary, download the streaming-capable model
   (`voxtype setup --download --model parakeet-unified-en-0.6b --quiet`, ~2.7GB), and
   create `~/.config/voxtype/config-streaming.toml` plus a
   `~/.config/systemd/user/voxtype-streaming.service` unit pointed at it (analogous to
   `voxtype.service`, but not enabled for auto-start). See issue 021's Findings section
   for the exact steps and the footguns hit along the way (a `voxtype setup --download`
   side effect that silently switches the live engine config, an undocumented
   streaming-timing constraint, and a PATH gap in ad hoc systemd units).
2. Day to day: `harnez tools voice-input mode streaming` / `... mode batch` / `...
   mode` (see Commands above) — no manual `systemctl`/two-terminal juggling needed.

Known upstream limitation (Voxtype 0.7.5, not a harnez bug): pauses in speech cause
multi-second output lag and occasionally drop words, because this streaming pipeline has
no VAD/end-of-utterance segmentation yet. See issue 021 for the debug-log analysis.
Enter-to-stop and deeper backtracking behavior remain open, see issue 021.

## History & Config Design Decisions (Issue 022)

### 1. Pass-Through History Capture (`history record`)
Voxtype's `[output.post_process]` configuration hook executes an external command with the
transcribed text on `stdin` and reads the replacement text from `stdout` before passing it to
the typing driver.

Rather than patching Voxtype upstream or running an intrusive background daemon, `harnez tools
voice-input history record` acts as a pass-through filter (a recording `cat`):
- Reads the transcript from `stdin`.
- Computes an 8-char SHA-256 ID, timestamps the entry, and appends it to
  `~/.local/share/harnez/voice-input/history.jsonl` (mode `0600`, capped at 20 entries).
- Writes the exact text back to `stdout` unchanged.

Wiring into `~/.config/voxtype/config.toml`:
```toml
[output.post_process]
command = "harnez tools voice-input history record"
```

### 2. Comment-Preserving In-Place Config Editing (`config set`)
Voxtype's own `voxtype config set` subcommand only supports the `engine` key and omits
`type_delay_ms`. To avoid destructive TOML round-trips that would strip user comments or
reformat spacing in `config.toml`, `harnez tools voice-input config set type-delay-ms <MS>`
uses an atomic, line-targeted regex replacement that modifies only the numeric delay in place.

## Debug & Tuning Harness (Build Tag: `debug`)

Experimental diagnostics and prototyping tools are gated behind Go's `//go:build debug` tag to keep standard release binaries clean. Build with `make build-debug` or `make install-debug`:

```text
harnez tools voice-input canary     # real-time streaming token observer & parameter tuner
harnez tools voice-input vad-probe  # prototype VAD-segmented sentence-by-sentence dictation
```

### 1. Streaming Canary (`canary`)
- Spawns an isolated Voxtype daemon with configurable chunk/context parameters.
- Intercepts streaming keystrokes via a virtual `dotool` pipe, measuring per-chunk delta timing and highlighting inter-word pause gaps.
- Empirically verified the upstream Parakeet streaming pause bug: natural 1–5s pauses saturate the context cache with silence, producing a 4–11s lag and dropping the initial post-pause word ("One").

### 2. VAD Sentence Dictation Probe (`vad-probe`)
- Continuously buffers 16kHz audio with a 250ms circular pre-roll buffer (eliminating initial consonant loss).
- Segments utterances on 600ms silence and transcribes completed sentences with Whisper in <300ms.

## Continuous Eager Sentence Streaming (Issue 026)

To bridge the gap between high-accuracy batch Whisper and low-latency streaming without suffering Parakeet's pause-loss bug, Issue 026 implements **Continuous Eager Sentence Streaming** in native Go orchestration (`harnez tools voice-input eager --daemon`):
- **Continuous Rolling VAD Capture**: Captures 16kHz PCM audio via `pw-record` with a `500ms` circular pre-roll buffer and `350ms` post-roll audio padding, preserving leading unstressed words (*"The"*, *"A"*) and trailing unvoiced consonants (*"cat"*, *"six"*).
- **GPU Hardware Acceleration**: Runs official release `voxtype-0.7.5-linux-x86_64-vulkan` leveraging local AMD Radeon Cezanne iGPU via Mesa RADV compute shaders (`/dev/dri/renderD128`). Reduces transcription latency from $>2.5\text{s}$ CPU compute to $<250\text{ms}$ GPU compute ($10\text{--}15\times$ faster than realtime).
- **Session Barrier & Kill Watcher**: Active context watcher terminates recording child processes in $<10\text{ms}$ on cancel; `sync.WaitGroup` session barriers prevent zombie processes and orphaned recording leaks.
- **Hallucination & Bias Rejection**: Sanitizes prompt biases (e.g. ESL subtitle triggers) and strips URLs or YouTube subscription markers from transcribed text before typing.

## Resource Monitor TUI (`resources --watch`)

`harnez tools voice-input resources --watch` provides an interactive, btop-styled terminal dashboard tracking voice subsystem health, performance, and hardware load with zero flicker:

```text
╭─ [s] voice & speed ─────────────────╮ ╭─ [h] hardware load ──────────────────╮
│ status:  ○ idle (eager / base.en)   │ │ cpu:   1.4%  [ ▂  ▅ ▂   ]  avg 4.5%  │
│ engine:  AMD Radeon Vulkan 1.4      │ │ gpu:  12.0%  [  ▂ █ ▂   ]  sys 0.74% │
│ speed:   10.5x realtime [████████]  │ │ mem:   6.1 MB daemon   1.3/8.0G VRAM │
│ lag:     0.22s · 8m 8s audio (118)  │ │ up:   55m 6s (PID 723612)            │
╰─────────────────────────────────────╯ ╰──────────────────────────────────────╯
╭─ [t] transcript feed ────────────────────────────────────────────────────────╮
│ 11:42:08  [0.22s]  "repeating that"                                          │
│ 11:42:06  [0.25s]  "I will make a little pause between each step..."          │
╰──────────────────────────────────────────────────────────────────────────────╯
╭─ [d] active daemons & health ────────────────────────────────────────────────╮
│ dotoold (PID 498151, 3.3 MB)  ·  harnez (PID 723612, 13.0 MB)  ·  ✓ clean     │
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

## Dedicated Modifier Daemon (`harnez-modifierd`)

To prevent synthetic keystrokes from clashing with held modifier keys (e.g. typing while the user is pressing `Ctrl`, `Alt`, `Super`, or `Shift`), `harnez` provides a dedicated physical modifier daemon:
- **Daemon (`harnez-modifierd` / `harnez tools daemon modifier-service`)**:
  - Discovers all physical keyboard devices via `/dev/input/event*` using `evdev` `EVIOCGBIT` ioctl.
  - Monitors physical modifier keys (`Left/Right Ctrl`, `Left/Right Alt`, `Left/Right Super`, `Left/Right Shift`).
  - Exports instantaneous modifier state as a 1-byte bitmask to `/run/harnez/modifiers` (`0644`).
  - **Zero Keylogging Guarantee**: Non-modifier keys are never inspected, recorded, or exported.
- **Client Reader (`ModifierReader`)**:
  - Single-byte file read (<10ns latency, zero IPC overhead).
  - Automatically gates `TypeText` in `internal/tools/voice_type.go` and `voice_eager.go`, pausing keystroke emission until all physical modifiers are released.
- **System Installation**:
  ```bash
  sudo make install-modifierd
  ```
  Installs `/usr/local/bin/harnez-modifierd` and enables the systemd service `/etc/systemd/system/harnez-modifierd.service` (`DeviceAllow=char-input r`, `SupplementaryGroups=input`).
