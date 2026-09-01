# 031: Claude Code and agent CLI voice pipeline research

**Status**: Proposed / Research  
**Category**: Voice Transcription & Agent CLI Research  

---

## 1. Objective

Research reliable voice capture / voice command pipelines for CLI coding
agents, especially Claude Code, and document what `voxi` can learn from them.

Treat transcript variants like "boxy", "waxy", "Voxy", or "VoxType" as `voxi`
unless context clearly says otherwise.

## 2. Research Questions

- What does Claude Code use for voice capture and transcription, if that is
  discoverable from local installation artifacts, public source/code bundles,
  remote copies, reverse-engineering notes, or official documentation?
- What is the likely end-to-end Claude Code voice pipeline: trigger/hotkey,
  audio capture, local vs remote processing, transcription provider/model,
  correction or prompt-injection path into the CLI, and privacy/network
  implications?
- Which facts are confirmed by source/docs and which parts remain informed
  inference?
- Are there other open-source agent CLI tools with voice command workflows that
  users actually appear to use and find helpful?
- What can `voxi` adopt or test to reduce current ASR friction, especially
  hallucinated proper nouns, odd YouTube/person names, and command-mode
  misrecognitions?

## 3. Research Scope

- Inspect local Claude Code installation artifacts available on this machine.
- Search the web for Claude Code voice capture, transcription internals, and
  community reverse-engineering notes.
- Prefer primary sources: official docs, public repositories, installed bundles,
  package metadata, and source files.
- Use secondary/community sources only to guide investigation or to identify
  artifacts worth verifying.
- Look for other open-source CLI/dev-agent voice systems with concrete usage
  evidence, not generic "voice assistant" lists.

## 4. Deliverable

Update this ticket with:

- confirmed Claude Code pipeline findings;
- source links and local file references;
- explicit uncertainty/inference notes;
- comparison to other plausible CLI/agent voice workflows;
- concrete `voxi` follow-up recommendations;
- any high-value experiments to improve transcription quality and command
  usefulness.

## 5. Non-goals

- Do not implement code in this research task.
- Do not change `voxi` transcription behavior yet.
- Do not claim Claude Code internals are known unless backed by source,
  installed artifacts, or official documentation.

---

## Findings (2026-09-01)

### Claude Code: confirmed local capture, cloud transcription

**Evidence:** the installed native Claude Code executable
`/home/uwe/.local/share/claude/versions/2.1.257` contains the bundled voice
implementation. This is reverse-engineering evidence from a shipped binary,
not official protocol documentation.

- It opens the microphone locally through a native audio-capture module; on
  Linux it falls back to ALSA (`arecord`) and then SoX (`rec`). The fallback
  command specifies signed 16-bit, mono, **16 kHz raw PCM**.
- It starts capture while the connection is being made, buffers the early
  chunks, then sends buffered and live audio chunks over an authenticated
  **WebSocket** (`connectVoiceStream`). The client receives both interim and
  final transcript events. The binary explicitly reports that voice requires a
  Claude.ai account and advises the user to check their network on a connection
  failure.
- Therefore no ASR model is bundled or inferred to run locally. Audio capture,
  level calculation, silence/end handling, transcript UI, and injection into
  the CLI are local; recognition is a cloud service.
- Claude Code derives up to 50 contextual **keyterms** from fixed developer
  terms (for example `MCP`, `TypeScript`, `OAuth`, `gRPC`) and names from the
  current project, paths, and files. It sends them with the voice-stream
  connection. This is the strongest practical lesson for technical dictation:
  bias the ASR decoder with bounded, automatically generated project terms,
  rather than asking an LLM to rewrite an already-decoded command.
- The implementation has a one-time retry for an early stream failure, a
  circuit breaker for repeated early failure, connectivity probing, an audio
  level buffer for the UI, and salvage of accumulated final text on mid-stream
  failure.

**Uncertain:** the server endpoint, wire framing, ASR model/provider, decoder
configuration, and retention policy cannot be established from the client
strings inspected. Do not call the exact model "Claude" or assume Whisper.

### Gemini web / Gemini Live: cloud processing

Google's official Gemini Apps privacy notice explicitly says that Gemini Apps
collects what the user says and stores/handles transcripts and recordings from
Gemini Live, including shared audio, video, and screens. That rules out a
fully-local ASR interpretation for the Gemini web/Live experience. The web UI
uses the browser's microphone permission; however, Google does not publish the
web client implementation or identify the ASR model/transport in the material
reviewed.

Google does document a separate, non-comparable product: Pixel advanced Gboard
voice typing keeps dictated text on-device, except for "Fix it" and detailed
editing. That is Android keyboard dictation, not Gemini web/Live.

Sources:

- [Gemini Apps Privacy Notice](https://support.google.com/gemini/answer/13594961?hl=en-CA)
- [Google Assistant: Advanced voice typing](https://support.google.com/assistant/answer/11197787?hl=en)

### Concrete Voxi follow-ups

1. Add an opt-in per-project speech-context provider: repository name, basename
   of recently changed files, language/tool names, and an explicit user
   glossary. Cap, deduplicate, and validate the list (Claude Code caps it at
   50); never derive secrets or arbitrary file contents.
2. Determine whether the selected local backend accepts prompts/hotwords. For
   Whisper this is an initial-prompt test; for any Parakeet replacement use its
   native vocabulary-biasing mechanism if available. Measure proper-noun and
   command WER against the current unprompted baseline.
3. Keep Voxi's audio/ASR path local by default. A cloud streaming path could
   improve quality but is a materially different privacy product and must be a
   separately configured opt-in.
4. Adopt the transport-independent reliability ideas: capture pre-roll while
   inference connects, show interim/final distinction, retain an audio-level
   ring, retry only a failed pre-transcript connection, and preserve completed
   text on a later failure.
