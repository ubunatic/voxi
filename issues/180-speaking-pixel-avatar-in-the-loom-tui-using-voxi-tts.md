# 180 — Speaking pixel avatar in the Loom TUI using Voxi TTS

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Feature
**Related**: [Cloned-voice TTS](155-integrate-pocket-tts-voice-cloning-as-voxi-tts-backend.md), [Local voice-cloning research](../docs/studies/2026-09-27-local-voice-cloning-research.md), `../loom`

---

## 1. Problem & Motivation
Add a small speaking avatar to a Loom-based TUI, using Voxi's existing TTS
playback and the user's own cloned voice. The supplied pixel avatar is at
`assets/avatar.png`.

## 2. Technical Specification / Findings
Use Loom for terminal rendering and Voxi for speech synthesis/playback. Show an
idle state and a visible speaking animation while synthesized audio is playing;
return to idle when playback ends or fails. Reuse the configured Voxi voice
profile rather than introducing a separate voice setup.

## 3. Implementation & Verification Plan
/goal Deliver a Loom TUI experience that speaks with the user's configured Voxi
voice and animates `assets/avatar.png` during playback, with tests for playback
state transitions and a live check where the local voice backend is available;
or stop and report when blocked on user input or denied permission.
