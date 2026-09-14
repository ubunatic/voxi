# voxi — Standalone Linux Voice Input Engine

[![License: AGPL-3.0-or-later](https://img.shields.io/badge/License-AGPL--3.0--or--later-blue.svg)](LICENSE)

`voxi` is a high-performance, privacy-first voice input, continuous eager sentence streaming, and desktop typing engine for Linux/Wayland. It brings fast speech-to-text dictation directly to any focused window with no cloud transcription and hotkey-safe input injection. The default Cohere Transcribe model runs locally on the CPU via `crispasr` and cached GGUF weights, with optional Whisper models supported out of the box.

---

## Key Features & Architecture Highlights

- **Continuous Eager Sentence Streaming (`voxi eager` / `voxi agent`)**  
  Captures audio continuously with a circular pre-roll buffer (500ms default). Utterances are segmented on natural speech pauses (silence > 800ms) or rolling windows and transcribed eagerly in sub-second bursts with zero dropped words across pauses.

- **Physical Modifier Gating Safety (`voxi-modifierd`)**  
  Monitors physical modifier keys (Ctrl, Alt, Super, Shift) directly from the kernel via evdev `EVIOCGKEY` with sub-10ns release gating. Prevents accidental hotkey combinations (e.g. typing `w` while holding `Ctrl` closing tabs) while strictly guaranteeing zero non-modifier keylogging.

- **Direct Synthetic Keystroke Injection**  
  Injects keystrokes directly into active Wayland applications via `dotool`/`dotoold` with complete XKB layout awareness (e.g. German QWERTZ, Colemak, Dvorak) and atomic clipboard fallback (`wl-copy`).

- **Local CPU Speech-to-Text (`crispasr` + Cohere Transcribe 03-2026)**  
  Defaults to local CPU inference with Cohere Transcribe 03-2026 Q5_0 GGUF weights (downloaded once on first run to `~/.cache/voxi/models/`, then run completely offline). Optional Whisper models (`base.en`, `small.en`, `large-v3-turbo`) remain available via `--model`.

- **Spec-Driven Hallucination Filtering**  
  Automatically suppresses known silence artifacts, repetitive subtitle loops, and metadata noise using embedded model specifications ([`spec/models.yaml`](spec/models.yaml)).

- **Local Dictation Feedback & Replacement Rules (`voxi feedback`)**  
  Teach voxi custom corrections layered on top of shipped defaults: case-adaptive transcript replacements (`voxi feedback replacement add`), custom stop words, silence artifacts, and Whisper vocabulary prompting.

- **Acoustic Ring Buffer Diagnostics (`voxi chunks`)**  
  Inspect recently captured audio chunks in a colorized table with real-time factors (RTF), RMS levels, Braille energy sparklines, gate outcomes, and audio playback (`voxi chunks play`).

- **Btop-Style Resource & Latency Monitor (`voxi monitor --watch`)**  
  A rich real-time terminal dashboard displaying live audio RMS meters, transcription latency sparklines, CPU/GPU memory consumption, and daemon health.

- **GNOME Shell Companion Extension**  
  Top-bar panel indicator (`voxi@ubunatic.com`) providing quick recording toggle, runtime mode switching (batch, streaming, eager), typing speed adjustments, and recent dictation history popup.

---

## Quickstart & Installation

### Option A: Go Install from Source

With Go 1.26.5 or newer and a checkout:

```bash
go install ./cmd/voxi
~/go/bin/voxi install
# Optional system-wide physical modifier daemon (requires sudo):
~/go/bin/voxi install --modifierd
```

`voxi install` installs dependencies and activates the user services. `make install`
uses the same command after building the CLI.

### Option B: One-Line Release Script

Install `voxi` directly into `~/.local/bin` and configure user services:

```bash
# Standard user-level install (no root/sudo needed)
curl -fsSL https://codeberg.org/ubunatic/voxi/raw/branch/main/scripts/install.sh | bash

# (Optional) Include privileged physical modifier daemon setup
curl -fsSL https://codeberg.org/ubunatic/voxi/raw/branch/main/scripts/install.sh | bash -s -- --modifierd
```

Ensure `~/.local/bin` (and `~/go/bin` for Go-installed helpers) is in your `$PATH`.

---

## Prerequisites & System Dependencies

When running `voxi install`, user-level dependencies (`crispasr`, `dotool`, `dotoold`) are automatically fetched and configured. Ensure standard host tools are present:

| Dependency | Purpose | Package / Source |
|---|---|---|
| **Go 1.26.5+** | Compiling from source | `golang` / `go` |
| **`dotoold` / `dotool`** | Hotkey-safe Wayland synthetic typing | Auto-installed by `voxi install` / [git.sr.ht/~geb/dotool](https://git.sr.ht/~geb/dotool) |
| **`wl-clipboard`** | Clipboard operations (`wl-copy`) & fallback | `wl-clipboard` |
| **`crispasr`** | Default eager ASR engine (Cohere Transcribe 03-2026, CPU) | Auto-installed by `voxi install` / [CrispASR](https://github.com/CrispStrobe/CrispASR) |
| **`whisper.cpp` / `voxtype`** *(optional)* | Local Whisper inference (Vulkan/CPU); only needed if selecting Whisper `--model` | [whisper.cpp](https://github.com/ggerganov/whisper.cpp) / [voxtype](https://github.com/peteon/voxtype) |
| **Audio Capture** | 16kHz mono audio recording | `pipewire-pulse` (`parec`) or `alsa-utils` (`arecord`) |
| **evdev Access** | Physical modifier key monitoring | `voxi-modifierd` service (root/systemd via `voxi install --modifierd`) |

---

## Getting Started

### 1. Verify the Agent Service

Verify that the unified background voice agent is active:

```bash
systemctl --user is-enabled voxi-agent.service
systemctl --user is-active voxi-agent.service
```

### 2. Start Dictating

Toggle recording from the terminal, GNOME Shell extension, or a desktop shortcut:

```bash
# Toggle recording on/off
voxi record toggle

# Check current recording state
voxi record status
```

### 3. GNOME Global Shortcut (Super+X)

On GNOME, configure the standard global shortcut:

```bash
voxi shortcut setup    # bind Super+X to the installed voxi binary
voxi shortcut setup -f # proceed if GNOME reports an existing assignment
voxi shortcut status   # inspect shortcut binding status
voxi shortcut remove   # remove only the Voxi-owned binding
```

### 4. Custom Transcript Corrections

Add deterministic case-adaptive word and phrase replacements for misheard project or domain names:

```bash
voxi feedback replacement add Voxy voxi
voxi feedback replacement add "harness project" "harnez project"
voxi feedback replacement list
voxi feedback replacement remove Voxy
```

---

## GNOME Shell Extension Setup

A companion extension is included in `contrib/gnome-shell-extension` (`voxi@ubunatic.com`):

```bash
# Link the extension into GNOME Shell extensions directory
mkdir -p ~/.local/share/gnome-shell/extensions
ln -s "$(pwd)/contrib/gnome-shell-extension" ~/.local/share/gnome-shell/extensions/voxi@ubunatic.com

# Enable the extension
gnome-extensions enable voxi@ubunatic.com
```

*Note: On Wayland sessions, log out and log back in or restart your session if GNOME Shell needs to discover the newly linked extension.*

---

## CLI Reference & Usage

### Installer
```bash
voxi install               # Configure user binaries, dependencies, and systemd user services
voxi install --modifierd   # Also install privileged system-wide modifier daemon (requires sudo)
```

### Mode Control
```bash
voxi mode                  # Show current active mode
voxi mode eager            # Continuous eager streaming (default)
voxi mode batch            # Batch dictation (record-then-transcribe)
voxi mode streaming        # Incremental streaming
```

### Recording Control
```bash
voxi record toggle         # Toggle recording on/off
voxi record start          # Start recording
voxi record stop           # Stop recording & finalize
voxi record status         # Check current recording state
```

### Direct Eager Streaming
```bash
# Run continuous sentence streaming directly in terminal
voxi eager --type --history                 # default: local Cohere Transcribe
voxi eager --type --history --model small.en # optional Whisper backend
```

### Resource & Latency Monitor
```bash
voxi monitor               # Single snapshot report
voxi monitor --watch       # Live interactive dashboard (btop-style HUD)
# Aliases: voxi top, voxi stats, voxi resources
```

### Chunk Diagnostics
```bash
voxi chunks list           # Show table with RTF, RMS, LEVEL sparkline, and gate status
voxi chunks show <INDEX>   # Print detailed diagnostics for a chunk
voxi chunks play <INDEX>   # Replay captured audio of a chunk
```

### Feedback & Overrides
```bash
voxi feedback status                        # Overview of all active local overrides
voxi feedback replacement add <HEARD> <FIX> # Exact word/phrase replacement
voxi feedback replacement list              # List active replacements
voxi feedback stop-word add <PATTERN>       # Add custom hallucination stop-word regex
voxi feedback silence-artifact add <PHRASE> # Add whole-phrase silence discard rule
voxi feedback vocabulary add <TERM>         # Add Whisper decoder prompt term
```

### Dictation History
```bash
voxi history list          # List recent dictation entries
voxi history copy <ID>     # Copy entry text to clipboard
voxi history retype <ID>   # Retype entry into active window
voxi history clear         # Clear all history entries
```

### Benchmarking Models
```bash
voxi bench --models small.en,large-v3-turbo --backends cpu,gpu
```

---

## Contributing

Contributions are welcome! Please refer to [CONTRIBUTING.md](CONTRIBUTING.md) for coding standards, conventional commit guidelines, and spec validation details.

---

## License

This project is licensed under the **GNU Affero General Public License v3.0 or later** ([AGPL-3.0-or-later](LICENSE)).
