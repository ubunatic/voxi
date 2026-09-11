# 105 — Headless end-to-end integration test with virtual audio and uinput in container

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Infrastructure
**Related**: [Installation Architecture](../docs/InstallationArchitecture.md), [Issue 104](104-add-voxi-install-command-for-complete-user-install-with-optional-privileged-setup.md), [Issue 081](081-install-and-run-a-persistent-dotoold-systemd-user-service.md), [scripts/test-install-podman.sh](../scripts/test-install-podman.sh)

---

## 1. Problem & Motivation

The current Podman installation test (`scripts/test-install-podman.sh` / `make test-install`) verifies the package download, checksum verification, CrispASR binary installation, Dotool build, and systemd unit placement. However, it does not exercise the live runtime pipeline:
- capturing audio from an input stream,
- transcribing eager phrases via CrispASR/Cohere, and
- injecting synthetic keystrokes via `dotool`/`dotoold`.

Running full integration tests on developer workstations or CI environments often lacks physical microphones, physical monitors, or focused desktop windows. To test real dictation behavior deterministically and safely without touching host sound cards or active desktop windows, we need a headless end-to-end container test environment.

## 2. Headless Design & Architecture

The test harness will run in a container (Podman) and virtualize both the input and output boundaries:

1. **Virtual Audio Source (Microphone Simulation)**:
   - Provide pre-recorded standard audio samples (e.g. 16kHz mono `.wav` saying a reference sentence).
   - Feed audio into `voxi eager` or `voxi agent` via virtual ALSA (`snd-dummy`), PulseAudio/PipeWire virtual source, or audio FIFO/stream redirection.
   - Assert that the Voxi voice activity detector (VAD) segments the audio correctly and CrispASR transcribes the expected sentence.

2. **Headless Wayland & Virtual Keyboard (Typing Simulation)**:
   - Run a lightweight headless Wayland compositor (such as `weston --headless` or `cage`) inside the container.
   - Attach `/dev/uinput` to allow `dotoold` to create a virtual input device.
   - Run a headless test client (or key logger listener) within the Wayland compositor to verify that the exact transcribed characters are received in order without dropped keystrokes or modifier corruption.

## 3. Acceptance Criteria

- A new script `scripts/test-e2e-headless.sh` (and `make test-e2e`) runs the complete audio-to-keystroke pipeline in a container.
- Test uses fixed reference fixtures (`.wav` files) and verifies deterministic output text.
- Verified keystroke injection into a headless Wayland compositor via `dotoold`.
- Runs cleanly without requiring real physical microphone hardware or an interactive desktop session.
- Exits with non-zero status and detailed diagnostics if audio transcription or keystroke injection fails or drifts.
