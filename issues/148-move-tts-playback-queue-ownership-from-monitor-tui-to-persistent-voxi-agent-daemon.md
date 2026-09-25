# 148 — Move TTS playback queue ownership from monitor TUI to persistent voxi-agent daemon

**Status**: Closed — Implemented persistent daemon TTS queue ownership, audio arbiter recording epoch mute gate, and offline CLI synthesis
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

### 2.1 Always-Running Daemon Architecture & Independent Component Lifecycle
The user-scoped background daemon (`voxi-agent.service` managed by `systemd --user`) acts as the central, lightweight supervisor managing three decoupled modular components:
1. **Voice / Playback Component**: Always-on IPC socket listener (`/run/user/$UID/voxi/tts.sock`) managing the TTS queue, synthesis worker (Piper/Festival), MPRIS DBus player interface, and `pw-play` audio playback.
   - **Independent Lifecycle**: Starts and binds its IPC socket immediately and runs concurrently. A crash, timeout, or startup delay in the ASR/eager pipeline MUST NOT bring down or delay the TTS playback listener.
2. **Transcribe / ASR Component**: Handles dictation triggering (`Super+X` / modifier gating), connects to the active ASR model, and injects text via `dotool`.
3. **LLM Cleaner Component**: Post-processes dictation when enabled.

The `voxi monitor` TUI attaches to the daemon's socket purely as an observability consumer (streaming log and playback telemetry events), rather than hosting the queue.

### 2.2 STT / Recording Interruption & Audio Arbiter
- **Audio Arbiter at Capture Transition**: When any recording path starts (e.g. `Super+X`, modifier hold, `voxi eager` daemon, signal, or CLI record), it invokes a local audio-arbiter callback to immediately interrupt live playback.
- **Recording Epoch / Mute Gate**: To eliminate race conditions where in-flight `voxi say` commands, background LLM continuations, or prefetched synthesis chunks might enqueue and play audio *after* an initial Stop signal during an active recording session, the queue manager maintains an active recording epoch gate. All live playback remains muted/paused until the recording epoch closes.
- **Non-Blocking Immediate Interruption**: `ActionStop` must kill the active `pw-play` playback process immediately and cancel background synthesis jobs without blocking the recording start path.

### 2.3 Safe / Isolated Offline TTS Rendering
- Callers requesting "safe" / unmanaged TTS (e.g. generating an audio file for testing, scripts, or non-desktop sinks) should use `voxi say -o <file.wav>` (or `--no-play`), rendering the audio directly to a file via `Engine.Synthesize` without dialing the daemon IPC socket, starting audio players, or altering live desktop playback.

### 2.4 IPC Compatibility & Socket Ownership
- Retain the `/run/user/$UID/voxi/tts.sock` path and line-oriented protocol for seamless backward compatibility.
- Ensure safe socket takeover with active-process verification before unlinking existing socket files.

## 3. Implementation & Verification Plan

**/goal**: Move the TTS socket, MPRIS interface, and playback queue lifecycle into `voxi-agent.service` with independent startup resilience, wire recording-start to an audio-arbiter with an active recording epoch gate, provide `voxi say -o <file>` for unmanaged offline synthesis, and decouple `voxi monitor` into a telemetry client.

1. **Independent TTS Daemon Service**:
   - Integrate `tts.Manager` and MPRIS into `internal/agent` (`voxi agent --daemon`) so it starts independently of eager/ASR readiness.
   - Verify that if ASR is misconfigured or fails, the TTS socket continues to accept and play `voxi say` requests.
2. **Audio Arbiter & Recording Epoch Gate**:
   - Implement an audio arbiter in `internal/agent` and `internal/tts` with recording epoch tracking to guarantee no queued or in-flight chunks play during active recording.
   - Ensure `ActionStop` executes non-blocking immediate playback termination.
3. **CLI Direct Offline Synthesis**:
   - Add `-o, --output <file.wav>` and `--no-play` to `voxi say`, bypassing daemon socket communication and rendering directly via `tts.Engine`.
4. **Monitor Telemetry Client**:
   - Update `voxi monitor` to connect to `tts.sock` as a telemetry subscriber rather than binding the server socket.
5. **Verification**:
   - Unit tests for recording epoch gate under concurrent enqueue/prefetch races.
   - Daemon resilience test: TTS playback working when eager/ASR engine is halted/errored.
   - Mutual exclusion test: STT recording start immediately silencing ongoing TTS playback.
   - Direct offline synthesis test: `voxi say -o sample.wav "test"` creating valid WAV with no daemon and no audio output.
   - Pass `make test-q1` and verify with `make install`.


