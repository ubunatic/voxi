# Voice Input: Real-time audio input volume / VU meter animation in recording indicator

- **Status:** Open — Enhancement to issues 022 & 024

## Context

When the user initiates dictation (either via the `Ctrl+Super+X` global shortcut or the top-bar indicator/menu button), they currently receive binary feedback: recording or idle. There is no visual confirmation that the physical microphone is picking up sound or whether the user is speaking loud enough for the speech recognition model (Whisper/Parakeet).

Adding a real-time, lightweight audio volume VU meter / level animation inside the recording button and/or top-bar indicator gives the user instant confidence that audio capture is active, responsive, and properly configured.

## Desired Behavior & Flow

1. **Visual Presentation**:
   - **In-Menu Recording Button**: A dynamic multi-bar equalizer / volume level meter or pulsing sound wave (3–5 vertical bars with variable heights) positioned next to the `⏹ Stop Recording` label.
   - **Top-Bar Panel Icon (Optional / Compact)**: An animated wave or volume-scaled icon (e.g. stepping between 1 to 3 audio signal waves or dynamic scale transform) during active speech.

2. **Audio Signal Source**:
   - **Option A (Voxtype socket / state stream)**: Voxtype exposes a live RMS volume meter stream over its IPC socket (`/run/user/1000/voxtype/audio.sock` or `voxtype status --follow`).
   - **Option B (PipeWire / PulseAudio Peak Meter via GJS / Harnez)**: Query PipeWire / PulseAudio source monitor stream directly (via `libpulse` / `Gvc.MixerControl` already built into GNOME Shell) to read peak input volume with near-zero overhead.

3. **Performance & Lifecycle Requirements**:
   - **Zero Idle Overhead**: Audio level monitoring / polling must be strictly deactivated when idle (`isRecording == false`).
   - **Smooth 30-60 FPS UI Rendering**: UI widget animations must use Clutter/St transitions without blocking the main event loop.

## Acceptance Criteria

- [ ] Real-time audio volume level animation renders inside the `Stop Recording` button when dictation is active.
- [ ] Visual animation dynamically responds to spoken volume intensity (silent speech vs loud speech).
- [ ] Monitoring activates only while recording and stops completely when idle.
- [ ] Can be verified and tested via `scripts/canary_ext`.
