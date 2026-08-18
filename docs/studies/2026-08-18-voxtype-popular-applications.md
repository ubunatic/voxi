# Study: Voxtype Ecosystem, Desktop Integrations & Power-User Workflows

**Date**: 2026-08-18  
**Scope**: Linux Wayland/X11 compositors (Hyprland, Sway, GNOME, KDE, River), status bars (Waybar, Polybar), editor workflows (Obsidian, Neovim), LLM post-processing (Ollama), meeting pipelines, and engine setups (Whisper, Parakeet, Soniox).  
**Primary References**: [docs/VoiceInput.md](../VoiceInput.md), [docs/VoiceInputArchitecture.md](../VoiceInputArchitecture.md), [docs/studies/2026-08-18-gnome-shell-companion-and-devkit-testing.md](2026-08-18-gnome-shell-companion-and-devkit-testing.md).

---

## 1. Executive Summary

[Voxtype](https://github.com/peteonrails/voxtype) (developed by *peteonrails*) has emerged as the standard offline, privacy-first voice-to-text dictation daemon for Linux desktop environments. Written in Rust and engineered specifically for modern Wayland compositors (with X11 fallback), Voxtype bridges speech recognition engines directly into active application windows via virtual keyboard injection.

This study compiles community integration patterns, power-user configurations, desktop extensions, LLM post-processing pipelines, and architectural workarounds across the Linux ecosystem.

---

## 2. Desktop Environment & Compositor Integrations

The primary user interaction model is **Push-to-Talk (PTT)** or **Toggle** dictation. Different compositors implement this via native keybinding hooks or user-space input drivers.

### 2.1. Hyprland
Hyprland users leverage `bind` (press) and `bindr` (release) pairs to create zero-latency push-to-talk bindings.

```ini
# ~/.config/hypr/hyprland.conf

# Push-to-Talk on Super + V
bind = SUPER, V, exec, voxtype record start
bindr = SUPER, V, exec, voxtype record stop

# Visual feedback: subtle border color change while recording
# Triggered via custom submap or hook script
```

**Power-User Pattern — Hyprland Dictation Submap**:
For continuous or modal dictation, users define a dedicated Hyprland submap that locks out regular keybindings and displays a visual indicator border until dismissed with `Escape` or `Enter`:

```ini
bind = SUPER, D, submap, dictation
submap = dictation
bind = , catchall, exec, voxtype record toggle
bind = , return, exec, voxtype record stop; hyprctl dispatch submap reset
bind = , escape, exec, voxtype record stop; hyprctl dispatch submap reset
submap = reset
```

### 2.2. Sway & i3
Sway and i3 use `--no-repeat` on press and `--release` on release to ensure clean push-to-talk boundaries without key repeat spam.

```text
# ~/.config/sway/config
bindsym --no-repeat $mod+v exec voxtype record start
bindsym --release $mod+v exec voxtype record stop
```

### 2.3. GNOME Shell (Wayland)
GNOME Shell does not natively expose key-release bindings to external desktop shortcut handlers. Community setups split into two approaches:

1. **Evdev Hotkey Mode (Built-in)**: Requires user membership in the `input` group or a dedicated udev rule to listen directly to raw keyboard scancodes (`[hotkey] key = "F9"`).
2. **GJS Companion Extension + Custom Shortcut (Privilege-Safe)**:
   - Uses standard GNOME keyboard shortcuts (`Super+Ctrl+X`) to toggle `harnez tools voice-input record toggle` or `voxtype record toggle`.
   - Runs an in-process GJS extension in the top bar with dynamic recording/idle icons and Mutter window focus coordination (see [VoiceInputArchitecture.md](../VoiceInputArchitecture.md)).

### 2.4. KDE Plasma & River
- **KDE Plasma**: Configured via Custom Shortcuts triggering `voxtype record toggle` combined with the `dotool` backend (since KWin Wayland restricts arbitrary synthetic input).
- **River**: Uses `riverctl map` with the `-release` flag:
  ```bash
  riverctl map normal Super V spawn 'voxtype record start'
  riverctl map -release normal Super V spawn 'voxtype record stop'
  ```

---

## 3. Status Bar Modules & Visual Feedback

Visual state indicators are essential so users know when the microphone is hot. Voxtype provides a streaming JSON status feed via `voxtype status --follow --format json`.

### 3.1. Waybar Integration (`custom/voxtype`)

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
    "on-click": "voxtype record toggle"
}
```

```css
/* ~/.config/waybar/style.css */
#custom-voxtype.recording {
    background-color: #e06c75;
    color: #282c34;
    animation-name: blink;
    animation-duration: 0.8s;
    animation-timing-function: linear;
    animation-iteration-count: infinite;
    animation-direction: alternate;
}

#custom-voxtype.transcribing {
    background-color: #61afef;
    color: #282c34;
}
```

### 3.2. Visual Equalizer Plugins (`dms-voxtype` & Cava)
In modular status bars (such as DMS / Polybar), community plugins pipe audio amplitudes from PipeWire via `cava` into the status block while `voxtype status` indicates the `recording` state, rendering a real-time mini audio waveform.

---

## 4. Post-Processing Pipelines & LLM Cleanup

Voxtype includes an external post-processing hook:
```toml
# ~/.config/voxtype/config.toml
[output.post_process]
command = "your-filter-script"
timeout_ms = 30000
```
This hook passes the raw transcript via `stdin` and types whatever the script outputs to `stdout`.

### 4.1. Local LLM Formatting via Ollama
Power users pipe raw spoken transcripts through small, fast local LLMs (e.g. `llama3.2:1b`, `phi-3-mini`, `qwen2.5:1.5b`) to eliminate filler words ("um", "ah", "like"), fix punctuation, and format code snippets.

**Direct Ollama Command**:
```toml
[output.post_process]
command = "ollama run llama3.2:1b 'You are a dictation cleanup assistant. Correct typos, remove filler words (um, uh), and apply proper punctuation. Output ONLY the cleaned text with no introductory phrases or quotes:'"
timeout_ms = 15000
```

**Advanced Post-Processing Dispatch Script (`~/.local/bin/voxtype-llm-cleaner.sh`)**:
```bash
#!/usr/bin/env bash
set -euo pipefail

RAW_TEXT=$(cat)

if test -z "${RAW_TEXT}"; then
    exit 0
fi

# Detect voice command shortcuts
case "${RAW_TEXT}" in
    *"clear history"*|*"delete history"*)
        harnez tools voice-input history clear >/dev/null 2>&1 || true
        exit 0
        ;;
    *"camel case "*|"camel "* )
        PHRASE="${RAW_TEXT#*camel }"
        echo -n "${PHRASE}" | sed -r 's/(^| )([a-z])/\U\2/g' | sed 's/ //g' | sed -r 's/^([A-Z])/\L\1/'
        exit 0
        ;;
    *"snake case "*|"snake "* )
        PHRASE="${RAW_TEXT#*snake }"
        echo -n "${PHRASE}" | tr '[:upper:]' '[:lower:]' | tr ' ' '_'
        exit 0
        ;;
esac

# Fast local LLM cleanup
PROMPT="Clean this voice transcript. Fix punctuation and grammar. Remove filler words (um, uh, like). Do not answer questions; only transcribe and clean. Output exact text only:

${RAW_TEXT}"

curl -s http://127.0.0.1:11434/api/generate -d @- <<EOF | jq -r '.response'
{
  "model": "llama3.2:1b",
  "prompt": $(echo -n "${PROMPT}" | jq -sR .),
  "stream": false,
  "options": {
    "temperature": 0.1,
    "top_p": 0.9
  }
}
EOF
```

### 4.2. In-Place Pass-Through History Capture (Harnez Pattern)
In Harnez, the post-process hook is wired as a zero-overhead logging filter (`harnez tools voice-input history record`) that records the transcript to an encrypted/restricted `history.jsonl` (mode `0600`) before echoing it to `stdout`, enabling instant desktop UI recall without patching upstream Voxtype.

---

## 5. Editor & Note-Taking Workflows

Because Voxtype injects keystrokes at the OS level, it works universally across all applications. However, specific editor workflows have gained significant traction.

### 5.1. Obsidian PKM (Personal Knowledge Management)
- **Rapid Daily Notes**: Users hold the hotkey while skimming articles or reading research papers; spoken thoughts appear directly in their open Obsidian daily note.
- **Voice Callouts & Markdown Syntax**:
  Using `[text] replacements` in `config.toml`, users map spoken triggers to markdown syntax:
  ```toml
  [text.replacements]
  "callout note" = "> [!NOTE]"
  "callout tip" = "> [!TIP]"
  "callout warning" = "> [!WARNING]"
  "bullet point" = "- "
  "task item" = "- [ ] "
  "new line" = "\n"
  "new paragraph" = "\n\n"
  ```

### 5.2. Neovim & VSCode
- **Modal Editing Awareness**:
  - In Neovim, voice typing operates naturally when in Insert mode (`i`).
  - Power users bind a Neovim key to trigger `voxtype record toggle` from Normal mode, automatically entering Insert mode upon start and returning to Normal mode (`<Esc>`) upon stop.
- **Code Dictation**: Voice macros for camelCase, snake_case, and common keywords ("const", "function", "return") allow hands-free drafting of comments, docstrings, and boilerplate.

---

## 6. Long-Form & Meeting Mode Workflows

Voxtype includes a dedicated continuous transcription engine for meetings, interviews, and lectures (`voxtype meeting`).

### 6.1. Meeting Pipeline Architecture
```text
[Microphone + Desktop Audio] (PipeWire Loopback / Monitor)
            │
            ▼
    [voxtype meeting daemon]
            │ (Continuous Chunked Transcription)
            ▼
   [Diarization Engine] (ML Speaker Clustering / Source Separation)
            │
            ├── Export to Markdown (with speaker labels & timestamps)
            ├── Export to Subtitles (SRT / VTT)
            └── Post-Meeting Summarization (via Ollama)
```

### 6.2. Key Commands & Automation
- **Start Meeting**: `voxtype meeting start --title "Architecture Sync"`
- **Pause / Resume**: `voxtype meeting pause` / `voxtype meeting resume`
- **Export & Summarize**:
  ```bash
  voxtype meeting stop
  voxtype meeting export latest --format markdown --output notes.md
  voxtype meeting summarize latest --model llama3.2:3b
  ```

---

## 7. Speech Recognition Engine Comparison

| Engine | Execution Mode | Accuracy | Latency (First Word) | CPU / GPU Requirements | Notes & Recommended Usage |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Whisper (`base.en` / `small.en`)** | Offline Batch | ★★★★★ | 0.8–1.5s (post-utterance) | Very low CPU (AVX2), no GPU needed | Default gold standard for batch dictation. Zero dropped words. |
| **Whisper (`large-v3-turbo`)** | Offline Batch | ★★★★★+ | 1.5–3.0s | Modern CPU or Vulkan/CUDA GPU | Best multi-language and technical terminology accuracy. |
| **Parakeet ONNX (`unified-en-0.6b`)** | Offline Streaming | ★★★★☆ | 250–480ms | Medium CPU (AVX2), no GPU needed | Incremental streaming partials. Sensitive to conversational pauses. |
| **Moonshine / SenseVoice** | Offline Low-Latency | ★★★★☆ | 400–700ms | Ultra-lightweight CPU / NPU | High speed on resource-constrained devices. |
| **Soniox** | Cloud Streaming | ★★★★★ | 150–250ms | Internet connection + API Key | Commercial cloud backend; lowest latency live captioning. |

---

## 8. Common Pain Points & Community Workarounds

### 8.1. Synthetic Typing Backend Matrix
Wayland compositors intentionally isolate input between clients. Different injection drivers have specific trade-offs:

| Injection Driver | Wayland Support | Layout Awareness | Latency Per Call | Security / Sandboxing Profile |
| :--- | :--- | :--- | :--- | :--- |
| **`dotool` / `dotoold`** | Universal (GNOME, KDE, Sway, Hyprland) | Full XKB awareness (`DOTOOL_XKB_LAYOUT`) | **<10ms** (via persistent pipe) | Direct `/dev/uinput` access via systemd-logind `uaccess`. |
| **`wtype`** | Hyprland, Sway, River | Compositor virtual keyboard protocol | ~20–50ms | Restricted to compositors implementing `virtual-keyboard-v1`. Fails on GNOME. |
| **`ydotool`** | Universal | Poor (often forces US layout) | ~15–30ms | Requires background root or uinput socket daemon. |
| **`eitype` (libei)** | GNOME 45+ | XKB aware | Variable | Routes through XDG RemoteDesktop portal; triggers recurring consent prompts. |

**Best Practice**: Use `dotoold` daemon for sub-10ms latency across GNOME/KDE Wayland with explicit `DOTOOL_XKB_LAYOUT` matching the physical keyboard.

### 8.2. Non-US Keyboard Layout Mapping
*Issue*: Voxtype's default settings can force `layout = "us"` when transcribing English speech, mangling characters (e.g. `z`/`y`, `@`, `/`, `-`) on German QWERTZ or French AZERTY keyboards.  
*Workaround*: In `~/.config/voxtype/config.toml`, set `language_to_layout = {}` and export `DOTOOL_XKB_LAYOUT=de` in the `dotoold` environment.

### 8.3. Conversational Pauses in Streaming Engines
*Issue*: Streaming ONNX models (Parakeet) experience a multi-second backlog recovery penalty and can drop the first word after an unvoiced pause (>1.5s).  
*Workaround*: Use batch Whisper for short commands and drafting, or employ **Continuous Eager Sentence Streaming** (rolling chunked Whisper passes on silence detection, as detailed in [Issue 026](../../issues/026-continuous-eager-sentence-streaming.md)).

### 8.4. Focus Stealing & Retype Race Conditions
*Issue*: Triggering a GUI menu or status popup steals window focus. Injecting text immediately on click types into the menu or void.  
*Workaround*: Capture target `MetaWindow` on menu open, close the popup, reactivate the window, await the compositor's focus settle signal, and only then fire keystroke injection (implemented in Harnez's GNOME extension).

---

## 9. Conclusion & Recommendations for Harnez

1. **Maintain the 4-Tier Decoupled Architecture**: Continue isolating Audio Engine (Voxtype) → Storage/CLI (`harnez tools`) → Desktop Shell (GNOME Extension) → Injection (`dotoold`).
2. **Standardize on `dotoolc` Fast Path**: Sub-10ms injection latency is required for both batch and streaming dictation without compositor lock-in.
3. **Offer Modular Post-Processing Recipes**: Provide pre-configured, optional Ollama cleanup scripts for users seeking automated punctuation and filler-word removal.
4. **Expand Waybar / Statusbar Presets**: Document standard JSON-formatted Waybar module snippets for users on Hyprland/Sway.
