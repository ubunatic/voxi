# 053: Ring Buffer of Last 10 Recorded Audio Chunks and Transcription Metadata

**Status**: Closed
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Feature
**Related**: [042 private dev sample recorder](042-private-dev-sample-recorder.md), [045 editable transcript for sample recorder](045-sample-recorder-editable-transcript-and-keyterms.md), [internal/eager/eager.go](../internal/eager/eager.go), [internal/history/history.go](../internal/history/history.go)

---

## 1. Problem & Motivation

Currently, when `internal/eager` captures and transcribes spoken audio:
1. It writes an audio slice to a temporary file `utt_%03d.wav` in a temporary directory (`/tmp/voxi-eager-...`).
2. It executes `voxtype transcribe` on that file.
3. Immediately upon completion of `cmd.Run()`, it executes `_ = os.Remove(wavPath)` (`internal/eager/eager.go:304`).
4. While the typed text is appended to history (`~/.local/share/voxi/history.txt`) and basic metrics are recorded in `eager-metrics.json` (`internal/eager.recordEagerStat`), the actual recorded audio bytes are permanently deleted.

This creates several usability and diagnostic pain points:
- **Debugging & Misrecognitions**: When the user notices a bad transcription, a garbled word, or a hallucination, the source audio that produced it is already gone. There is no way to inspect what audio Whisper actually heard (e.g. mic clipping, low volume, background noise, or truncated syllables).
- **Corpus & Sample Collection Workflow**: Under [Issue 042](042-private-dev-sample-recorder.md), creating a test sample requires explicitly running `voxi feedback sample record <name>` ahead of time. If a user says something interesting during normal dictation, they cannot say "save that last utterance as a test sample."
- **Retype & Correction UX**: If dictation failed or was interrupted, having the last few audio chunks cached allows re-transcribing with different options (e.g., higher model tier, different temperature, or alternative vocabulary prompts) without asking the user to repeat themselves.

---

## 2. Desired Design

Maintain a persistent ring buffer of the **last 10 recorded audio chunks** along with their complete transcription metadata (e.g., in `$XDG_RUNTIME_DIR/voxi/chunks/` or `~/.cache/voxi/chunks/`):

### 2.1 Storage Scheme

A fixed ring buffer of $N=10$ entries (0 to 9 or timestamped/indexed):
- **Audio file**: `chunk_0.wav` .. `chunk_9.wav` (or `chunk_<index>.wav`), 16kHz mono PCM WAV.
- **Metadata**: JSON sidecar (`chunk_<index>.json`) or a consolidated `manifest.json` recording:
  - `index`: Monotonic utterance sequence ID.
  - `timestamp`: ISO-8601 capture time.
  - `audio_duration_secs`: Length of the audio snippet.
  - `transcribe_duration_secs`: Execution time of the ASR engine.
  - `rtf`: Real-time factor.
  - `raw_transcript`: Exact text output from the model before cleaning.
  - `cleaned_transcript`: Text after stop-word and hallucination stripping.
  - `accepted`: Boolean flag indicating whether it passed silence/artifact filtering and was typed.
  - `rejection_reason`: If not accepted, why (e.g., "silence_artifact", "stop_word", "empty").
  - `wav_file`: Relative or absolute path to the audio file.

When chunk $N+1$ is recorded, the oldest chunk file is pruned or overwritten so disk usage remains strictly bounded (10 chunks of ~3–10s audio is under 3–5 MB total).

### 2.2 CLI Commands

Expose inspecting, playing, and promoting these chunks:

```text
# List the last 10 recorded chunks with timestamps and transcribed text
voxi chunks list

# Play back audio of a specific recent chunk (or the most recent by default)
voxi chunks play [index|last]

# Inspect full diagnostic metadata for a chunk
voxi chunks show [index|last]

# Promote a recent chunk directly into the permanent sample corpus (connecting with Issue 042)
voxi feedback sample save-last [name]
```

---

## 3. Implementation Plan

1. **Ring Buffer Manager (`internal/chunks`)**:
   - Create a clean abstraction for managing the bounded ring buffer on disk.
   - Enforce atomic writes and safe rotation (maximum 10 entries).
   - Ensure clean cleanup on daemon shutdown if stored in `$XDG_RUNTIME_DIR/voxi/chunks/`.
2. **Integration into Eager Pipeline (`internal/eager/eager.go`)**:
   - In `runEagerCaptureSession`, instead of deleting `wavPath` immediately with `os.Remove(wavPath)`, pass the audio buffer or move the file into the chunk ring buffer.
   - Save both accepted and rejected chunks (with `accepted: false` and the rejection reason), since diagnosing false rejections or silence artifacts is one of the highest-value use cases.
3. **CLI Plumbing**:
   - Add `voxi chunks [list|play|show]` subcommands under `cmd/voxi`.
   - Add `voxi feedback sample save-last <name>` to allow instant saving of the last utterance into `~/.config/voxi/samples/corpus.tsv`.
4. **Tests**:
   - Unit tests for the ring buffer rotation: verify that writing 15 items leaves exactly the last 10, pruning oldest files and metadata.

---

## 4. Verification

- Implemented `internal/chunks` ring buffer with rotation, pruning, atomic manifest writing, and 0700/0600 permissions.
- Integrated eager capture pipeline in `internal/eager/eager.go` to store both accepted and rejected chunks (capturing `silence_artifact`, `stop_word`, `empty`, or transcribe error).
- Added `voxi chunks list`, `voxi chunks show [index|last]`, and `voxi chunks play [index|last]`.
- Added `voxi feedback sample save-last <name>` integrating with `internal/devsample.SaveChunkAsSample`.
- Comprehensive unit tests added across `internal/chunks`, `internal/eager`, and `internal/feedback`.
- Rebuilt binaries and restarted daemon with `make restart-service`.
- All tests pass: `go test -count=1 ./...`.
