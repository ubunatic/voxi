# Continuous Listening, Wake-Word Activation & Verbal Turn-Taking

- **Status:** Proposed / Research & Planning
- **Related Issues:** 020 (Voice Input), 021 (Fluent Streaming), 026 (Continuous Eager Sentence Streaming)

## Context & Vision

During hands-free voice typing and agent interaction experiments, physical hotkeys (`Super+Ctrl+X` or `F9`) and GUI click triggers still create friction when the user's hands are away from the keyboard or focused on other tasks.

The user tested verbal turn delimiters:
- **Start Triggers**: `"Robot start"`, `"Robot listen"`, `"Hey Harnez"`, `"Listen computer"`
- **End / Submit Triggers**: `"Human over"`, `"Robot stop"`, `"Stop recording"`

**Goal:** Enable an always-on, hands-free voice interaction loop that wakes on a verbal trigger (e.g. `"Robot start"`), streams transcribed dictation, and automatically finalizes/submits upon hearing the verbal stop delimiter (e.g. `"Human over"`), with near-zero idle resource cost and local privacy.

---

## 1. What Does Continuous Listening Cost? (Resource & Power Analysis)

Running full speech-to-text models continuously 24/7 on raw microphone audio has very different costs depending on architecture:

### Cost Comparison Matrix

| Approach | Idle CPU Usage | Memory Footprint | Battery / Thermal Impact | Feasibility for 24/7 Background |
| :--- | :--- | :--- | :--- | :--- |
| **Continuous Whisper (`base.en`)** | **$15\% - 35\%$ CPU** | $\sim 200 - 400\text{ MB}$ | High (fans spin, drains laptop battery) | ❌ Impractical for always-on idle |
| **VAD-gated Whisper** | $\sim 1\% - 3\%$ (silent room)<br>$15\% - 30\%$ (ambient noise) | $\sim 200 - 400\text{ MB}$ | Medium (any background noise/music triggers full Whisper) | ⚠️ Unreliable; frequent false activations |
| **Two-Stage Wake Word (openWakeWord / ONNX)** | **$< 0.5\% - 1.0\%$ CPU** | **$\sim 15 - 30\text{ MB}$** | **Near-zero (silent, low battery consumption)** | **✅ Standard industry pattern** |

---

## 2. Two-Stage Architecture: Wake Word + Active Whisper

To achieve zero idle overhead and reliable activation, the system separates idle listening from active transcription:

```
[Microphone (PipeWire Stream)]
         │
         ▼
[Circular Audio Ring Buffer (500ms)]
         │
         ▼
┌────────────────────────────────────────────────────────┐
│ Stage 1: Ultra-Lightweight Wake-Word Engine            │
│ (openWakeWord / MicroWakeWord — <1% CPU, in-memory)    │
└────────────────────────────────────────────────────────┘
         │
         ├── Listens continuously for "Robot start" / "Hey Harnez"
         │
         ▼ (Trigger Detected!)
┌────────────────────────────────────────────────────────┐
│ Stage 2: Active Dictation Engine (Harnez Eager Stream) │
│ - Play subtle audio wake chime                         │
│ - Prepend ring buffer and stream audio to Whisper      │
│ - Types / buffers speech in real time                  │
│ - Listens for Verbal Stop ("Human over") or Silence    │
└────────────────────────────────────────────────────────┘
         │
         ▼ (Heard "Human over" or prolonged silence)
[Finalize & Submit]
  ├── Strip "Robot start" / "Human over" delimiters from transcript
  ├── Play subtle turn-complete chime
  └── Return to Stage 1 Low-Power Idle
```

---

## 3. Key Technical Challenges & Research Questions

### 3.1. Wake-Word Engine Selection (Linux / Go Integration)
1. **openWakeWord (Open-Source, Apache 2.0)**:
   - Uses small ONNX models ($<5\text{MB}$) executed via `onnxruntime`.
   - Supports training custom wake words (e.g. `"Robot start"`, `"Hey Harnez"`, `"Human over"`) using synthetic speech datasets (Piper/VITS).
   - Runs efficiently on CPU without GPU requirements.
2. **MicroWakeWord / Porcupine**:
   - Picovoice Porcupine (very accurate, but proprietary license / key required).
   - MicroWakeWord (TensorFlow Lite micro, designed for embedded/low power).

### 3.2. Delimiter Stripping & Cleaning
- The transcribed text must not leak the verbal control tokens into the target document or agent prompt.
- The pipeline must reliably strip leading start triggers (`"Robot start, ..."`, `"Hey Harnez, ..."`) and trailing stop triggers (`"... human over"`, `"... robot stop"`).

### 3.3. Dual Endpointing (Verbal Delimiter + Adaptive Silence)
- If the user says `"Human over"`, dictation finalizes **immediately** (zero wait time).
- If the user simply stops speaking without saying the delimiter, the system falls back to a standard $1.5\text{s} - 2.0\text{s}$ conversational silence timeout.

---

## 4. Proposed Milestone Plan

1. **Phase 1: Research & Benchmark (Spike)**
   - Benchmark `openWakeWord` ONNX runtime on this workstation (AMD Ryzen 5650U).
   - Measure actual CPU/RAM metrics during 10 minutes of continuous idle listening.
2. **Phase 2: Custom Wake Word & Delimiter Models**
   - Generate custom ONNX models for `"robot_start"` and `"human_over"`.
   - Test false-positive rejection against typical room audio / conversation.
3. **Phase 3: Harnez Integration**
   - Integrate Stage 1 listener into `harnez tools voice-input listen`.
   - Connect trigger transitions to Harnez's continuous eager sentence streamer.
4. **Phase 4: Agent & Desktop UI Feedback**
   - Visual indicator state in GNOME Shell top-bar (Idle $\to$ Listening $\to$ Transcribing).
   - Audio feedback chimes for wake and submit.
