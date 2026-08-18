# Study: Omarchy 4.0.0 Voxtype Configuration, Keybindings & OS Integration

**Date**: 2026-08-18  
**Scope**: Omarchy 4.0.0 ("Quattro") Arch Linux distribution, Voxtype voice-to-text daemon (`peteonrails/voxtype`), Hyprland Wayland compositor integration, systemd user services, output drivers, status bars, and audio feedback.  
**Related Documents**: [docs/VoiceInput.md](../VoiceInput.md), [docs/VoiceInputArchitecture.md](../VoiceInputArchitecture.md), [docs/studies/2026-08-18-voxtype-popular-applications.md](2026-08-18-voxtype-popular-applications.md).

---

## 1. Executive Summary

**Omarchy** (the opinionated Arch Linux + Hyprland distribution created by DHH/Basecamp) incorporates **Voxtype** as its first-class, offline, AI-powered speech-to-text dictation engine. Unlike legacy Python/pip-dependent dictation setups (e.g. `hyprwhspr`), Voxtype is implemented in Rust, providing single-binary deployment, zero-latency push-to-talk, Whisper and Parakeet engine support, and native Wayland virtual keyboard injection.

In Omarchy 4.0.0 (the "Quattro" release cycle), the OS-level voice typing pipeline is integrated across four main layers:
1. **Daemon & Engine Configuration**: Managed via `~/.config/voxtype/config.toml` with Whisper (`base.en` / `large-v3-turbo`) and streaming Parakeet support.
2. **Keybindings & Compositor Hooks**: Managed via Hyprland bindings (`bindings.lua` / `bindings.conf`) providing dual push-to-talk (`F9`) and chorded toggle (`SUPER + CTRL + X` or `SUPER + code:49`).
3. **Daemon Lifecycle & Init**: Managed via a `systemd` user service (`voxtype.service`) activated on login.
4. **Desktop Indicators & Audio Feedback**: Integrated with Waybar via streaming JSON IPC (`voxtype status --follow --format json`), desktop notifications, and low-latency audio cue chimes.

---

## 2. Voxtype `config.toml` Settings & Schema

Omarchy deploys its default Voxtype configuration to `~/.config/voxtype/config.toml` (with system-level defaults falling back to `/etc/voxtype/config.toml`).

```toml
# ~/.config/voxtype/config.toml
# Omarchy 4.0.0 Default Configuration

[general]
log_level = "info"
state_file = "auto"              # Writes runtime state for status bar IPC

[engine]
default = "whisper"              # "whisper" or "parakeet"

[whisper]
model = "base.en"                # Default fast model; user switchable to "large-v3-turbo"
language = "en"                  # "auto" or ISO 639-1 code (e.g., "en", "de", "es")
temperature = 0.0
beam_size = 5
suppress_blank = true
threads = 4                      # Tuned to available CPU cores

[parakeet]
model = "parakeet-unified-en-0.6b"
streaming = false                # Enables real-time word-by-word streaming dictation

[audio]
sample_rate = 16000              # 16kHz required by Whisper/Parakeet
channels = 1                     # Mono audio
device = "default"               # PipeWire / PulseAudio default source
silence_threshold = 0.015        # Energy threshold for silence
max_duration_secs = 120          # Maximum continuous recording safeguard

[audio.vad]
enabled = true                   # Voice Activity Detection
threshold = 0.5                  # VAD activation threshold
silence_duration_ms = 700        # Automatic endpointing after silence

[audio.feedback]
enabled = true                   # Play acoustic chimes on start/stop
theme = "subtle"                 # Options: "default", "subtle", "mechanical"
volume = 0.6                     # Normalized 0.0 - 1.0

[output]
mode = "type"                    # "type" (virtual keys) or "clipboard" (copy to clipboard)
type_delay_ms = 2                # Inter-keystroke delay to prevent buffer overruns
driver_order = ["wtype", "dotool", "ydotool", "clipboard"]
dotool_xkb_layout = ""           # Inherits system layout if empty

[output.notification]
enabled = true
on_recording_start = false       # Suppressed to avoid notification spam
on_recording_stop = false
on_transcription_complete = false # Notifications only on error / fallback

[text]
strip_fillers = true             # Automatically strips "um", "uh", "like", "you know"
capitalize_sentences = true
auto_punctuation = true

[media]
pause_media = false              # Optional media pausing (requires playerctl)

[hotkey]
enabled = false                  # Disabled in config: keybindings are delegated to Hyprland compositor
```

### 2.1. Key Parameter Breakdown

* **`[output].driver_order = ["wtype", "dotool", "ydotool", "clipboard"]`**:
  - **`wtype`**: Primary driver for Wayland compositors (Hyprland). Provides native UTF-8 and unicode symbol injection without touching clipboard state.
  - **`dotool`**: Cross-platform fallback that uses `/dev/uinput` without requiring a background daemon.
  - **`ydotool`**: Background daemon-based fallback (`ydotoold`).
  - **`clipboard`**: Safe terminal fallback; if typing fails or for mixed-script multi-lingual input, text is pushed to `wl-copy`.
* **`[hotkey].enabled = false`**:
  - In Omarchy, Voxtype's internal Linux `evdev` event listener is deliberately disabled in favor of compositor-level bindings. This avoids needing `input` group permissions or raw `/dev/input/event*` handles while running in a Wayland environment.
* **`[text].strip_fillers = true`**:
  - Automatically cleans speech artifacts before text injection.

---

## 3. Keybindings & Recording Triggers

Omarchy exposes voice typing through multiple binding strategies to support both push-to-talk and toggle workflows.

### 3.1. Hyprland Configuration

In Omarchy 4.0.0 (which migrated binding architectures to Lua-based definitions in `~/.config/hypr/bindings.lua` alongside legacy `bindings.conf`), Voxtype is registered as follows:

#### Push-to-Talk (Hold F9)
```ini
# ~/.config/hypr/bindings.conf (or bindings.lua)
# Push-to-Talk on dedicated function key
bind = , F9, exec, voxtype record start
bindr = , F9, exec, voxtype record stop
```

#### Chorded Toggle (`SUPER + CTRL + X` or `SUPER + code:49`)
```ini
# Toggle dictation session
bind = SUPER CTRL, X, exec, voxtype record toggle --auto-submit
bind = , ESCAPE, exec, voxtype record cancel
```

In `bindings.lua`:
```lua
-- ~/.config/hypr/bindings.lua
o.bind("SUPER + code:49", "Toggle voice dictation", "voxtype record toggle --auto-submit", { release = true })
o.bind("NONE + F9", "Push to talk voice dictation", "voxtype record start", { release = false })
o.bind("NONE + F9", "Stop push to talk voice dictation", "voxtype record stop", { release = true })
```

### 3.2. Resolving "Sticky Modifier" Race Conditions

A common issue in Wayland tiling window managers is the "sloppy finger" modifier accumulation bug: if a user presses `SUPER + CTRL + X`, begins dictating, and releases the `SUPER` or `CTRL` key *after* typing begins, the synthetic keystrokes injected by `wtype` can inherit the active modifier masks, resulting in accidental shortcut executions (e.g., closing windows with `Ctrl+W`).

Omarchy addresses this in two ways:
1. **Key Release Handling (`bindr` / `--auto-submit`)**: Releasing the trigger key issues `voxtype record stop`, which introduces a ~10-20ms modifier settling window before dispatching `wtype`.
2. **Dedicated Single Key**: Primary recommendation is standardizing on a single unmodified key (`F9` or dedicated extra mouse button) for push-to-talk.

---

## 4. Systemd Service Integration

Voxtype is managed as a `systemd` user service, ensuring background availability, low-memory persistence, and instant response when recording starts.

### 4.1. Unit File: `~/.config/systemd/user/voxtype.service`

```ini
[Unit]
Description=Voxtype Voice Typing Daemon
Documentation=https://github.com/peteonrails/voxtype
PartOf=graphical-session.target
After=graphical-session.target pipewire.service

[Service]
Type=simple
ExecStart=/usr/bin/voxtype daemon
Restart=on-failure
RestartSec=3s
Nice=-5

# Environment & IPC
Environment=RUST_LOG=info
Environment=XDG_RUNTIME_DIR=%t

# Hardening / Sandbox
ProtectSystem=full
ProtectHome=read-only
ReadWritePaths=%h/.config/voxtype %h/.local/share/voxtype %t

[Install]
WantedBy=graphical-session.target
```

### 4.2. Management CLI Commands
Omarchy provisions Voxtype using the built-in systemd setup tool:
```bash
# Enable and start the service
systemctl --user daemon-reload
systemctl --user enable --now voxtype.service

# Inspection & logs
systemctl --user status voxtype
journalctl --user -u voxtype -f
```

---

## 5. Visual Indicators & Waybar Integration

Omarchy provides visual feedback across two channels: Waybar status bar widgets and desktop notifications.

### 5.1. Waybar Custom Module (`custom/voxtype`)

Waybar connects to Voxtype's JSON status stream:

```json
// ~/.config/waybar/config
"custom/voxtype": {
    "exec": "voxtype status --follow --format json",
    "return-type": "json",
    "format": "{icon} {}",
    "format-icons": {
        "idle": "",
        "recording": "",
        "transcribing": "󰑮",
        "error": ""
    },
    "tooltip": true,
    "on-click": "voxtype record toggle",
    "on-click-right": "voxtype setup model"
}
```

### 5.2. Waybar Styling (`style.css`)

```css
/* ~/.config/waybar/style.css */
#custom-voxtype {
    padding: 0 10px;
    margin: 0 4px;
    border-radius: 6px;
    background-color: rgba(30, 30, 46, 0.5);
    color: #cdd6f4;
}

#custom-voxtype.recording {
    background-color: #f38ba8;
    color: #11111b;
    animation: blink 1s infinite alternate;
}

#custom-voxtype.transcribing {
    background-color: #fab387;
    color: #11111b;
}

#custom-voxtype.error {
    background-color: #eba0ac;
    color: #11111b;
}
```

---

## 6. Installation Scripts & Menu Integration

Omarchy integrates Voxtype into its centralized TUI installer and desktop menu system:

### 6.1. Menu Flow
* **Path**: `Omarchy Menu` -> `Install` -> `AI` -> `Dictation`
* Triggers the underlying installer script (`omarchy-install-dictation` or `voxtype setup`).

### 6.2. Hardware & GPU Acceleration
Omarchy configures GPU offloading (Vulkan / CUDA) via Voxtype's setup commands:
```bash
# Download default Whisper model
voxtype setup --download

# Enable GPU acceleration
voxtype setup gpu --enable

# Configure systemd user daemon
voxtype setup systemd
```

---

## 7. Comparative Assessment for Harnez Integration

| Dimension | Omarchy Voxtype Implementation | Harnez Voice Pipeline Equivalent |
|---|---|---|
| **Daemon Architecture** | Rust daemon (`voxtype daemon`) with IPC socket | Standalone CLI (`harnez tools voice-input`) + Voxtype engine |
| **Output Method** | Virtual keyboard injection (`wtype` > `dotool` > `ydotool`) | Virtual keystrokes (`wtype` / `dotool`) + optional clipboard fallback |
| **Compositor Target** | Hyprland (Wayland) | Hyprland, Sway, GNOME Shell (GJS Companion), KDE Plasma |
| **Status Stream** | JSON line stream via `voxtype status --follow` | Waybar JSON module + GNOME TopBar extension |
| **Model Selection** | Local Whisper `base.en` / `large-v3-turbo` / Parakeet | Whisper (`base.en` default) / Parakeet / Ollama post-processing |
| **Audio Feedback** | Built-in sound theme (`subtle`, `mechanical`) | PipeWire / PulseAudio low-latency cue chimes |

---

## 8. References & Documentation

* **Voxtype Repository**: [https://github.com/peteonrails/voxtype](https://github.com/peteonrails/voxtype)
* **Omarchy Repository**: [https://github.com/basecamp/omarchy](https://github.com/basecamp/omarchy)
* **Voxtype Documentation**: [https://voxtype.io](https://voxtype.io)
* **Harnez Desktop Studies**: [docs/studies/2026-08-18-voxtype-popular-applications.md](2026-08-18-voxtype-popular-applications.md)
