# 148 — Move TTS playback queue ownership from monitor TUI to persistent voxi-agent daemon

**Status**: Open
**Priority**: P1 (High)
**Severity**: Moderate
**Category**: Architecture
**Related**: [029](029-single-voxi-agent-mode-orchestration.md), [125](125-pause-media-playback-during-recording-resume-if-voxi-paused-it.md), [141](141-discovery-add-a-tts-engine-so-other-tools-can-have-voxi-read-text-aloud.md), [142](142-monitor-merge-tts-into-a-unified-voxi-log-feed-tts-box-on-and-hardware-box-off-by-default.md), [147](147-research-oss-tts-engines-and-open-weight-models-for-modest-amd-hardware.md)

---

## 1. Problem & Motivation

Currently, the TTS synthesis and playback queue manager is owned exclusively by an active `voxi monitor -w` TUI process listening on `/run/user/$UID/voxi/tts.sock`. If the monitor TUI window is not running, any invocation of `voxi say` or `Super+Y` fails with a missing socket error.

Voice output must be **always available and instantaneously responsive**, completely independent of:
- Whether a `voxi monitor` terminal is open.
- Which speech transcription model (Cohere, Whisper, R2T2) is active or whether its backend server is running.
- Which LLM cleanup server is running.

## 2. Technical Specification / Findings

### 2.1 Always-Running Daemon Architecture
The user-scoped background daemon (`voxi-agent.service` managed by `systemd --user`) should serve as the central, lightweight supervisor managing three decoupled modular components:
1. **Voice / Playback Component**: Always-on IPC socket listener (`/run/user/$UID/voxi/tts.sock` or unified `voxi.sock`) managing the TTS queue, synthesis worker (Piper/Festival), and `pw-play` audio playback.
2. **Transcribe / ASR Component**: Handles dictation triggering (`Super+X` / modifier gating), connects to the active ASR model, and injects text via `dotool`.
3. **LLM Cleaner Component**: Post-processes dictation when enabled.

The `voxi monitor` TUI attaches to this daemon purely as an observability consumer (streaming log/telemetry events), rather than owning the playback engine or state.

### 2.2 STT / Recording Interruption Rule
- **Mutual Exclusion on Audio**: Starting any voice transcription / recording session (e.g. `Super+X`, modifier hold, or `voxi eager`) MUST immediately stop active live TTS speech playback and clear pending live playback queue chunks. This ensures the microphone does not capture Voxi's own speech output.

### 2.3 Safe / Isolated Offline TTS Rendering
- Callers requesting "safe" / unmanaged TTS (e.g. generating an audio file for testing, scripts, or non-desktop sinks) should use `voxi say -o <file.wav>` (or `--no-play`), rendering the audio directly to a file without submitting it to the live daemon playback queue or disturbing active desktop audio.

## 3. Implementation & Verification Plan

**/goal**: Move the TTS socket and playback queue lifecycle into `voxi-agent.service`, wire recording-start to automatically stop active live TTS playback, provide `voxi say -o <file>` for unmanaged offline synthesis, and decouple `voxi monitor` into a read-only telemetry client.

1. **Daemon Integration**: Migrate `tts.Manager` into `internal/agent` (`voxi agent --daemon`) so the systemd user service owns the IPC socket and playback lifecycle.
2. **STT Stop Trigger**: When ASR recording begins in `internal/eager` or `internal/record`, send an immediate interrupt/stop signal to the TTS queue manager.
3. **CLI Safe Output**: Add `-o, --output <file.wav>` and `--no-play` flags to `voxi say` to synthesize directly to disk without requiring the daemon or altering live playback.
4. **Monitor Decoupling**: Update `voxi monitor` to read state and subscribe to playback events from the daemon's socket rather than hosting the queue.
5. **Verification**:
   - Verify `voxi say "text"` works when `voxi monitor` is NOT running.
   - Verify `voxi say` playback immediately stops when `Super+X` dictation starts.
   - Verify `voxi say -o out.wav "test"` writes `out.wav` without playing live audio or needing the daemon.
   - Run unit tests with `make test-q1` and verify with `make install`.

