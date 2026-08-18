# Continuous Eager Sentence Streaming Dictation

- **Status:** Implemented (Go Harnez Pipeline)
- **Related Issues:** 020 (Voice Input), 021 (Fluent Streaming), 022 (Transcriber UI)

## Context & Problem Statement

In issue 021 and empirical canary testing (`harnez tools voice-input canary`), we established two major findings regarding voice dictation modes on Linux (Fedora 44 / Wayland):

1. **Parakeet Streaming Limitations with Pauses**:
   - Continuous speech produces deltas at steady ~0.25–0.48s intervals.
   - However, **natural pauses (1–5 seconds) break the stream**: Voxtype's cache-aware Parakeet ONNX pipeline pays a 4–11 second backlog resynchronization penalty after each pause and consistently drops the initial word/consonant (e.g. "One" in *"One, two, three... [pause]... One, two, three"*).
2. **Pure Batch Mode (`whisper base.en`) Limitations**:
   - Whisper has 100% transcription accuracy across pauses of any duration, with zero dropped words.
   - However, output is entirely deferred until the user manually triggers stop/toggle.

**Goal:** Provide a fluent, real-time typing experience where sentences stream incrementally into the focused application as you finish speaking them, without dropping words after pauses and without requiring repeated manual start/stop toggles.

---

## Architecture: Continuous Eager Sentence Streaming

Instead of relying on fragile frame-level streaming or all-at-once batching, **Continuous Eager Sentence Streaming** operates on rolling chunked batch inference over a continuous audio stream:

```
 Microphone (Continuous PipeWire capture)
   │
   ▼
[Audio Ring Buffer & Streamer] (keeps rolling audio + timestamps)
   │
   ├── (Every 2–3s OR on 500–700ms silence)
   ▼
[Whisper Batch Worker] (warm, in-memory ggml-base.en)
   │
   ▼
[Sentence Boundary & Prefix Resolver]
   ├── Detect completed sentence / punctuation boundary
   ├── Commit & Type finalized prefix via dotoolc -> Focused App
   └── Retain trailing tail as acoustic prompt for next chunk
```

### Key Principles

1. **Continuous Audio Ingestion**: The microphone capture never stops mid-turn. There is no start/stop latency and no risk of losing starting phonemes.
2. **Sentence-Boundary Stability**: Whisper produces timestamped segments. When a phrase concludes with punctuation (period, comma, question mark) or is followed by >500ms acoustic silence, that prefix is marked **final** and typed immediately.
3. **Context Preservation (Overlap Rolling)**: The unfinalized tail (last 1–2 words or overlap audio) is retained and passed as the `initial_prompt` or acoustic overlap to the next Whisper pass, preventing word repetition or boundary clipping.
4. **Zero Lost Words on Pauses**: Because Whisper processes full audio utterances with complete acoustic context, pauses do not cause cache resync bugs or swallowed numbers/words.

---

### Strategy B: Native Harnez Go Orchestrator (Implemented & Shipped)

Implemented native Go streaming orchestration in `internal/tools/voice_eager.go` under `harnez tools voice-input eager --daemon`:
- Audio pipeline with `pw-record` streaming into rolling `AudioSegmenter` with `500ms` circular pre-roll and `350ms` post-roll padding.
- Official release `voxtype-0.7.5-linux-x86_64-vulkan` leveraging local AMD Radeon Cezanne iGPU via Mesa RADV (`/dev/dri/renderD128`).
- Lowers transcription latency to $<250\text{ms}$ GPU compute ($10.5\times$ faster than realtime speech) with zero pause drops.
- Active context cancellation watcher eliminates zombie recording subprocesses.
- Integrated with `harnez tools voice-input resources --watch` for real-time Btop-style TUI monitoring.

---

## Verification & Benchmarks

1. **Pause Test**: Dictated *"1 2 3 4 5 6 7 [pause] abcdefg [pause] The small brown fox jumps over the yellow cat"* $\to$ 100% cleanly transcribed across pauses with zero missing words.
2. **Speed & Latency**:
   - Audio duration: $1\text{m } 12\text{s}$
   - Total GPU compute time: $6.8\text{s}$
   - Real-time factor: $0.09\text{x}$ ($10.8\times$ faster than human speech)
   - Visual lag after voice stops: $<250\text{ms}$
3. **Hardware Utilization**:
   - VRAM footprint: $487\text{MB}$
   - Average CPU utilization: $1.2\%$ (down 97% from multi-threaded CPU compute).


### Strategy B: Harnez Eager Sentence Pipe (Go / Subprocess Pipeline)

Harnez implements a lightweight orchestrator daemon:
- Captures 16kHz mono audio via `pw-record`.
- Spawns background worker(s) performing fast local Whisper inference on completed phrase windows.
- Resolves finalized text deltas and types them via Harnez's existing `TypeText` (`dotoolc`) primitive.
- Controlled via `harnez tools voice-input record [start|stop|toggle]`.

---

## Acceptance Criteria

- [x] Audio capture remains continuously active without stopping between conversational pauses.
- [x] Spoken sentences are transcribed and typed into the focused application within 0.5–1.5s of completing each phrase/sentence.
- [x] Zero dropped words after natural conversational pauses (e.g. *"The quick brown fox... [pause]... jumps over the yellow cat"* captures all words).
- [x] No duplicate text or typing stutter across chunk boundaries.
- [x] Validated with canary benchmarks measuring inter-sentence latency and token completeness.

