# GNOME transcriber UI: Visual typing feedback indicator (keyboard icon during synthesis)

- **Status:** Open — Enhancement to issue 022

## Context

During batch dictation mode, transcription occurs after recording finishes. When Voxtype or `harnez tools voice-input history retype` synthesizes text keystrokes via `dotoolc`/`dotoold`, the top bar panel indicator should display clear visual feedback that synthetic typing is actively in progress.

Displaying a keyboard icon (e.g. `input-keyboard-symbolic`) during the text typing phase provides immediate UX confirmation that the system is injecting text into the target application window, differentiating the typing state from active audio capture (`media-record-symbolic`) and idle standby (`audio-input-microphone-symbolic`).

## Desired Behavior & Flow

1. **State Indicator Mapping**:
   - **Idle**: `audio-input-microphone-symbolic` (white/neutral top bar icon).
   - **Recording / Listening**: `media-record-symbolic` (bright red `#e01b24`).
   - **Typing / Emitting Keystrokes**: `input-keyboard-symbolic` (accent blue or amber pulse).

2. **Integration Touchpoints**:
   - **Voxtype Post-Process / State Poller**: When Voxtype transitions through `transcribing` or actively running `dotoold` typing, update `_icon.icon_name = 'input-keyboard-symbolic'`.
   - **Retype Action (`_onRetypeEntry`)**: Set the icon to `input-keyboard-symbolic` when synthetic typing begins, reverting to `audio-input-microphone-symbolic` when the subprocess completes.
   - **Automatic Reversion**: Return to the idle microphone icon as soon as typing output finishes.

## Acceptance Criteria

- [ ] Top bar panel indicator displays `input-keyboard-symbolic` during active synthetic typing in batch mode.
- [ ] Retyping from history popup displays the keyboard icon while injecting synthetic keystrokes.
- [ ] Icon automatically reverts to `audio-input-microphone-symbolic` upon completion of typing.
