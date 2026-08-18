# voxi — Standalone Linux Voice Input Engine

`voxi` is a high-performance voice input, continuous eager streaming, and desktop typing engine for Linux/Wayland.

## Key Features

- **Continuous Eager Sentence Streaming**: Rolling Whisper inference with circular pre-roll audio buffers and natural pause segmentation (`voxi eager`).
- **Physical Modifier Gating Safety**: The `voxi-modifierd` daemon reads physical modifier state via evdev `EVIOCGKEY` with sub-10ns release gating, preventing accidental shortcut triggering while guaranteeing zero non-modifier keylogging.
- **Direct Synthetic Keystroke Injection**: Seamless injection via `dotool`/`dotoold` with XKB layout awareness and atomic clipboard fallback (`wl-copy`).
- **Btop-Style Resource Monitor**: Real-time terminal dashboard (`voxi monitor --watch`) with audio level meters, transcription latency sparklines, CPU/VRAM usage, and AMD GPU Vulkan acceleration monitoring.
- **GNOME Shell Companion**: Top bar extension (`voxi@ubunatic.com`) providing quick recording toggle, mode switcher, type delay slider, and dictation history popup.

## Installation

```bash
# User binaries (~/go/bin)
make install
make install-user-services

# Optional: System modifier daemon service
make install-modifierd
```

## Usage

```bash
# Show or switch runtime modes (batch, streaming, eager)
voxi mode
voxi mode eager

# Recording control
voxi record toggle
voxi record status

# Dictation history
voxi history list
voxi history copy <ID>
voxi history retype <ID>
voxi history clear

# Resource & latency monitor
voxi monitor --watch
```

## License

AGPL-3.0-or-later
