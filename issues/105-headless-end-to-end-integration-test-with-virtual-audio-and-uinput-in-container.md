# 105 — Headless end-to-end integration test with virtual audio and uinput in container

**Status**: Closed — implemented + verified: `make test-e2e` completes in ~8s with virtual PulseAudio and isolated dotool sink
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

The test harness runs in an isolated container (Podman) and virtualizes both the input and output boundaries:

1. **Virtual Audio Source (Microphone Simulation)**:
   - Provide pre-recorded standard audio samples (`test/fixtures/short-one-two.wav`, `test/fixtures/short-abc.wav`).
   - Feed audio into `voxi eager` via PulseAudio virtual sink (`auto_null.monitor`).
   - Assert that the Voxi voice activity detector (VAD) segments the audio correctly and CrispASR transcribes the expected sentence.

2. **Headless Isolated Typing Simulation**:
   - Run in an isolated container without host uinput device mapping to ensure zero host window typing leakage.
   - Attach a persistent FIFO typing sink on `/tmp/dotool-pipe` to verify dotool keystroke generation.
   - Assert deterministic transcription in ring buffer chunks and logs.

## 3. Acceptance Criteria & Verification

- [x] A new script `scripts/test-e2e-headless.sh` (and `make test-e2e`) runs the complete audio-to-keystroke pipeline in a container.
- [x] Test uses fixed reference fixtures (`.wav` files) and verifies deterministic output text.
- [x] Verified keystroke injection into an isolated virtual sink via `dotoolc`.
- [x] Runs cleanly without requiring real physical microphone hardware or an interactive desktop session (~8s execution time).
- [x] Exits with non-zero status and detailed diagnostics if audio transcription or keystroke injection fails or drifts.
