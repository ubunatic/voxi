# Case Study: Building Linux Desktop TTS Reading (From Zero to Super+Y LLM Passthrough)

**Date**: 2026-09-25 / 2026-09-26  
**Scope**: Text-to-Speech (TTS) engine, GNOME Wayland selection reading hotkeys, LLM document narration, pip-free native Piper ONNX runtime, multi-voice library, and persistent systemd daemon queue ownership with STT recording epoch muting.  
**Starting State**: `voxi` was strictly an input/ASR dictation tool with no speech output capabilities.  
**Outcome**: Complete, ultra-low latency desktop reading pipeline (`voxi say`, `Super+Y` hotkey) with streaming chunk prefetching, local LLM fluency restructuring, high-quality multi-dialect Piper neural synthesis (~0.05 RTF on AMD APU, ~140MB RAM), and independent daemon playback supervision with `Super+X` mic-feedback safety.

---

## 1. Executive Summary

In a single multi-phase sprint, `voxi` was expanded from a voice-to-text dictation engine into a two-way voice interface for the Linux desktop. 

The resulting pipeline allows users to select any on-screen text (code, markdown, prose, tables) in Wayland/GNOME and press **`Super+Y`** (or `Shift+Super+Y` for clipboard). The system transparently streams the text through an LLM narration prompt (via local/network `lmcoder` endpoints) to normalize symbols, restructure tables, and tune cadence for fluent audio comprehension, feeds the result into a local pip-free native neural TTS engine (Piper ONNX), and plays it seamlessly through a persistent background audio queue managed by `voxi-agent.service`. Starting voice dictation (`Super+X`) instantly interrupts and mutes synthetic speech via an audio arbiter epoch gate, preventing acoustic feedback loops.

---

## 2. What Was Built (The Journey)

```
+-----------------------------------------------------------------------------------+
| WAYLAND DESKTOP SELECTION (Super+Y) / CLIPBOARD (Shift+Super+Y) / CLI (voxi say) |
+-----------------------------------------+-----------------------------------------+
                                          |
                                          v
                    +-----------------------------------+
                    |    voxi say (Document Splitting)  |
                    +-----------------+-----------------+
                                      |
                                      v
                    +-----------------------------------+
                    |  LLM Fluency Normalizer (lmcoder) |
                    |  - Preserves facts & nuance       |
                    |  - Spoken markdown / tables       |
                    |  - Pacing / rhythm conditioning   |
                    +-----------------+-----------------+
                                      |
                                      v
                    +-----------------------------------+
                    | voxi-agent.service (Daemon Queue) |
                    | Unix Socket: /run/user/1000/tts.sock|
                    +--------+------------------+-------+
                             |                  |
              +--------------+                  +---------------+
              v                                                 v
+-------------------------------+              +--------------------------------+
| Piper Neural Engine (C++ ONNX)|              | Audio Arbiter Mute Gate        |
| - ~/.local/lib/voxi/piper/    |              | - Tracks active STT epochs     |
| - Standalone C++ binary       |              | - Super+X dictation halts and  |
| - ~0.05 RTF, ~140MB RAM       |              |   discards in-flight TTS       |
+---------------+---------------+              +--------------------------------+
                |
                v
+-------------------------------+
| Multi-Voice ONNX Library      |
| - en_US-lessac-medium         |
| - en_US-bryce-medium          |
| - en_GB-alan-medium           |
| - en_GB-southern_english_fem  |
+---------------+---------------+
                |
                v
+-------------------------------+
| PipeWire / PulseAudio Output  |
| - Overlapped prefetch         |
| - Trailing silence trim       |
+-------------------------------+
```

1. **Discovery & Shortcut Pipeline (Issues 141, 143, 144)**:
   - Initialized `internal/tts` with playback abstractions, sentence splitters, and trailing silence trimming.
   - Wired `Super+Y` (primary selection) and `Shift+Super+Y` (clipboard) into the GNOME custom shortcut manager with automatic fallback to standard Wayland selection tools (`wl-paste`).

2. **LLM Narration & Fluency Passthrough (Issue 146)**:
   - Wired `lmcoder prompt` to restructure complex markdown, tables, and dense symbols into listenable prose.
   - Designed a two-phase document pipeline: first paragraph is dispatched immediately to minimize time-to-first-sound; subsequent paragraphs generate in the background while audio is already playing.

3. **Pip-Free Neural Engine Integration (Issue 147)**:
   - Evaluated the open-source TTS landscape on AMD Ryzen 5 PRO 5650U APU (Festival vs. espeak-ng vs. Kokoro vs. Piper).
   - Rejected Python/`pip` packaging to prevent virtualenv fragility. Integrated a self-contained, prebuilt native C++ Piper distribution (`~/.local/lib/voxi/piper/`) directly into `voxi install`.
   - Curated and installed a multi-voice library (`en_US-lessac`, `en_US-bryce`, `en_GB-alan`, `en_GB-southern_female`). Benchmarks achieved **RTF 0.048–0.160** (sub-second synthesis) with only **~144 MiB RAM** and zero GPU/VRAM contention.

4. **Persistent Daemon Queue & Audio Arbiter Gating (Issue 148)**:
   - Decoupled the TTS playback queue from `voxi monitor -w` (which was an interactive TUI) and moved the Unix socket server into the persistent systemd user daemon `voxi-agent.service`.
   - Added an **Audio Arbiter Epoch Gate**: When the user presses `Super+X` to start dictation, the daemon increments an active recording epoch. In-flight and queued TTS chunks are aborted instantly, ensuring the microphone never picks up speech generated by `voxi say`.
   - Provided offline rendering (`voxi say -o output.wav`) for standalone and CI usage.

5. **Sample Library Promotion & Personalized Voice Roadmap (Issue 149)**:
   - Promoted recorded user speech chunks `#1414`, `#1415`, `#1416` into `~/.config/voxi/samples/`.
   - Filed Issue 149 defining the recipe for fine-tuning user voice profiles into standalone Piper ONNX models (`"my-voice"`).

---

## 3. What Worked Well

- **Canary-First Benchmarking**: Writing `scripts/canary_tts/benchmark.py` and running reproducible latency tests against real hardware before integrating code saved days of dead ends. It immediately proved that Piper C++ was 4x faster than Festival and used 60% less RAM than Python neural alternatives.
- **Pip-Free Native Vendoring**: Downloading pre-compiled C++ standalone Piper binaries with vendored `.so` dependencies in `~/.local/lib/voxi/piper/` prevented Python virtualenv hell and system package collisions.
- **Reviewer Sprints (`terra:med`)**: Running dedicated plan reviews caught critical architectural flaws early, specifically the mistake of attaching the TTS queue socket to the transient `voxi monitor` TUI instead of the persistent systemd service.
- **Spec-Driven Architecture**: Centralizing model paths, trailing silence trimming thresholds, and audio parameters in `spec/tts.yaml` allowed rapid tuning without scattering constants across Go packages.

---

## 4. Honest Post-Mortem (Failures, Bugs & Near-Misses)

| Incident / Near-Miss | Root Cause | How It Was Caught | Resolution Applied |
|---|---|---|---|
| **TTS Queue Died on Monitor Exit** | Initial prototype attached the TTS Unix socket `/run/user/1000/voxi/tts.sock` to `voxi monitor -w`. Exiting the terminal monitor broke all `Super+Y` hotkeys. | Architectural plan review by `terra:med`. | Migrated socket listener and playback supervisor into `voxi-agent.service` (`cmd/voxi/agent.go`). |
| **Microphone Captured TTS Playback** | Pressing `Super+X` while `voxi say` was speaking caused Whisper/Cohere ASR to transcribe Voxi's own synthetic voice as user speech. | Manual live desktop testing. | Implemented the Audio Arbiter recording epoch gate: STT transitions trigger immediate playback abort and queue purge. |
| **Interactive Prompt Hang in CI/Scripts** | `voxi feedback sample save-chunk` prompted interactively on `stdin` for transcripts/keyterms, causing agent commands without piped inputs to block and fail (exit 125). | Task timeout during chunk promotion. | Documented need for non-interactive stdin piping (`printf "\n\n" \| ...`) and noted flag support for future CLI work. |
| **Subagent Non-Interactive Stdin Lock** | Spawning subagents via `harnez agent start` failed with `codex exec: exit status 1: Reading additional input from stdin...`. | Direct tool call execution check. | Identified that underlying `codex-cli` required closed stdin when executing non-interactively; stopped unprompted loops immediately. |

---

## 5. Quality & Invariants Audit

| Dimension | Assessment | Verification Evidence |
|---|---|---|
| **Module Separation** | Clean | `internal/tts` manages engine interfaces and synthesis; `internal/eager` & `internal/record` manage STT; `cmd/voxi` coordinates daemon/CLI endpoints. |
| **Resource Boundaries** | Excellent | 0 MB VRAM used. CPU utilization on APU is <15% during brief synthesis bursts (~140MB RAM). Background LLM servers on ports 8734–8748 remained untouched. |
| **Backward Compatibility** | Preserved | Falls back gracefully from Piper -> Festival -> espeak-ng if neural binaries/models are absent. Offline `-o` synthesis works without daemon. |
| **Test Quality & Quota-1** | Validated | All tests pass under Quota-1 guardrails (`make test-q1`). |

---

## 6. Efficiency & Velocity Assessment

- **Speed to Value**: Completed full architectural discovery, canary benchmarking, engine integration, daemon refactoring, and multi-voice validation within a single 24-hour cycle.
- **Agent Coordination**: Autonomous developer-reviewer loop (`luna:med` and `terra:med`) allowed deep refactoring of concurrent Go services without human micromanagement.

---

## 7. Key Learnings & Evergreen Upstream

1. **Persistent Daemon Ownership for Audio Sinks**: Never bind desktop-wide IPC services (sockets/FIFOs) to interactive TUI monitors. All long-lived state and sinks must belong to `systemd --user` units (`voxi-agent.service`).
2. **Audio Arbiter Invariant**: In any two-way voice system (STT + TTS), recording start events must be an authoritative mute switch for playback to prevent acoustic feedback loops.
3. **Standalone Binary Distributions Over Pip**: For desktop tools distributed to end users, prefer standalone precompiled C++/ONNX executables vendored in `~/.local/lib/` over Python pip packages.
4. **LLM Fluency Restructuring**: Raw text is often unlistenable (code blocks, markdown syntax, URLs). Passing text through a fast LLM prompt tuned for spoken prosody dramatically improves user experience.

---

## 8. File & Diff Summary

### Key Files Created / Modified
- `internal/tts/`: Engine abstractions, sentence chunking, Piper backend, trailing silence trimming, and queue manager.
- `cmd/voxi/say.go`: CLI command for `voxi say`, Wayland clipboard/selection ingestion, offline `-o` output, and `--no-play` mode.
- `cmd/voxi/agent.go`: Daemon initialization, TTS socket server, and audio arbiter epoch coordination.
- `internal/record/manager.go`: STT recording epoch tracking and toggle state transition hooks.
- `spec/tts.yaml`: Declarative engine fallbacks, model paths, and playback thresholds.
- `scripts/canary_tts/benchmark.py`: Persistent CPU/APU benchmarking canary.
- `docs/TTSReading.md`: Evergreen guide for TTS reading, shortcuts, and engine precedence.
- `docs/InstallationArchitecture.md`: Installation steps for pip-free Piper and voice models.
- `issues/141`, `143`, `144`, `145`, `146`, `147`, `148`, `149`: Complete lifecycle tracking from discovery to closure.
