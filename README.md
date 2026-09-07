# voxi — Standalone Linux Voice Input Engine

[![License: AGPL-3.0-or-later](https://img.shields.io/badge/License-AGPL--3.0--or--later-blue.svg)](LICENSE)

`voxi` is a high-performance, privacy-first voice input, continuous eager sentence streaming, and desktop typing engine for Linux/Wayland. It brings fast, sub-second speech-to-text dictation directly to any focused window with zero cloud dependencies and hotkey-safe input injection.

---

## Key Features & Architecture Highlights

- **Continuous Eager Sentence Streaming (`voxi eager` / `voxi agent`)**  
  Captures audio continuously with a circular pre-roll buffer. Utterances are segmented on natural speech pauses (silence > 800ms) or rolling windows and transcribed locally via Whisper with zero dropped words across pauses.

- **Physical Modifier Gating Safety (`voxi-modifierd`)**  
  Monitors physical modifier keys (Ctrl, Alt, Super, Shift) via kernel evdev `EVIOCGKEY` with sub-10ns release gating. Prevents accidental hotkey combinations (e.g. typing `w` while holding `Ctrl` closing tabs) while strictly guaranteeing zero non-modifier keylogging.

- **Direct Synthetic Keystroke Injection**  
  Injects keystrokes directly into active Wayland applications via `dotool`/`dotoold` with complete XKB layout awareness (e.g. German QWERTZ, Colemak, Dvorak) and atomic clipboard fallback (`wl-copy`).

- **Spec-Driven Hallucination Filtering**  
  Automatically filters out Whisper silence artifacts, repetitive hallucination loops, and metadata logs using embedded model specifications ([`spec/models.yaml`](spec/models.yaml)).

- **Btop-Style Resource & Latency Monitor (`voxi monitor --watch`)**  
  A rich real-time terminal dashboard displaying live audio RMS meters, transcription latency sparklines, GPU Vulkan / CPU memory consumption, and daemon status.

- **GNOME Shell Companion Extension**  
  Top-bar panel indicator (`voxi@ubunatic.com`) providing quick recording toggle, runtime mode switching (batch, streaming, eager), typing speed adjustments, and recent dictation history popup.

---

## Prerequisites & System Dependencies

Ensure the following tools and packages are installed on your Linux system:

| Dependency | Purpose | Package / Source |
|---|---|---|
| **Go 1.22+** | Compiling `voxi` and `voxi-modifierd` | `golang` / `go` |
| **`dotoold` / `dotool`** | Hotkey-safe Wayland synthetic typing | [git.sr.ht/~geb/dotool](https://git.sr.ht/~geb/dotool) |
| **`wl-clipboard`** | Clipboard operations (`wl-copy`) & fallback | `wl-clipboard` |
| **`crispasr`** | Default eager ASR engine (Cohere Transcribe 03-2026, CPU); first use lazily downloads ~1.66 GiB of GGUF weights into `~/.cache/voxi/models/`, then runs offline | [CrispASR](https://github.com/CrispStrobe/CrispASR) |
| **`whisper.cpp` / `voxtype`** *(optional)* | Local Whisper inference (Vulkan/CPU); only needed if you explicitly select a Whisper `--model`, or for legacy batch/streaming modes | [whisper.cpp](https://github.com/ggerganov/whisper.cpp) / [voxtype](https://github.com/peteon/voxtype) |
| **Audio Capture** | 16kHz mono audio recording | `pipewire-pulse` (`parec`) or `alsa-utils` (`arecord`) |
| **evdev Access** | Physical modifier key monitoring | `voxi-modifierd` service (root/systemd) |

> [!TIP]
> Make sure `dotoold` is running in your user session or started automatically via your compositor/systemd.

---

## Quickstart

### 1. Build and Install

```bash
git clone https://codeberg.org/ubunatic/voxi.git
cd voxi

# Install user binaries to ~/go/bin
make install

# Install systemd user service units
make install-user-services

# (Optional) Install system physical modifier daemon
sudo make install-modifierd
```

Ensure `~/go/bin` is in your `$PATH`.

### 2. Start the Agent Service

Start the unified background voice agent:

```bash
systemctl --user enable --now voxi-agent.service
```

### 3. Start Dictating

Toggle recording from the terminal, GNOME Shell extension, or a custom desktop shortcut:

```bash
# Toggle recording on/off
voxi record toggle

# Check status
voxi record status
```

Start speaking, toggle recording off (or pause in eager mode), and watch your words typed directly into the active application.

---

## GNOME Shell Extension Setup

A companion extension is included in `contrib/gnome-shell-extension` (`voxi@ubunatic.com`):

```bash
# Link the extension into GNOME Shell extensions directory
mkdir -p ~/.local/share/gnome-shell/extensions
ln -s "$(pwd)/contrib/gnome-shell-extension" ~/.local/share/gnome-shell/extensions/voxi@ubunatic.com

# Enable the extension (or toggle via GNOME Extensions app)
gnome-extensions enable voxi@ubunatic.com
```

*Note: On Wayland sessions, log out and log back in or restart your session if GNOME Shell needs to discover the newly linked extension.*

---

## CLI Reference & Usage

### Mode Control
```bash
# Show current mode
voxi mode

# Switch mode (eager = continuous streaming, batch = record-then-transcribe)
voxi mode eager
voxi mode batch
voxi mode streaming
```

### Recording Control
```bash
voxi record toggle   # Toggle recording
voxi record start    # Start recording
voxi record stop     # Stop recording & finalize
voxi record status   # Check current recording state
```

### Direct Eager Streaming
```bash
# Run continuous sentence streaming directly in terminal
voxi eager --type --history --model small.en
```

### Resource & Latency Monitor
```bash
# Single snapshot
voxi monitor

# Live interactive dashboard (btop-style)
voxi monitor --watch
# Aliases: voxi top, voxi stats, voxi resources
```

### Dictation History
```bash
voxi history list           # List recent dictation entries
voxi history copy <ID>      # Copy an entry to clipboard
voxi history retype <ID>    # Retype an entry into active window
voxi history clear          # Delete all history entries
```

### Benchmarking Models
```bash
# Benchmark Whisper models across CPU vs GPU backends
voxi bench --models small.en,large-v3-turbo --backends cpu,gpu
```

---

## Contributing

Contributions are welcome! Please refer to [CONTRIBUTING.md](CONTRIBUTING.md) for coding standards, conventional commit guidelines, and spec validation details.

---

## License

This project is licensed under the **GNU Affero General Public License v3.0 or later** ([AGPL-3.0-or-later](LICENSE)).
