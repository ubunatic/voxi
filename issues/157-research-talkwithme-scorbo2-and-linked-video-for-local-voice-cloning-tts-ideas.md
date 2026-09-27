# 157 — research TalkWithMe (scorbo2) and linked video for local voice cloning / TTS ideas

**Status**: Closed — Done; see docs/Roadmap.md close/park section
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Research
**Related**: issue 154 (Pocket TTS canary), issue 155, `docs/studies/2026-09-27-local-voice-cloning-research.md`

---

## 1. Problem & Motivation
The user pointed to two sources to review:
- https://github.com/scorbo2/TalkWithMe (Python, MIT, "Configurable AI chat with TTS/STT", pushed 2026-09-26)
- https://www.youtube.com/watch?v=P0N91YLX04A (content not yet reviewed)

Context: issue 154 showed Pocket TTS via sherpa-onnx clones the user's voice but drops words on longer
texts (int8 and fp32). We look for local, non-cloud tooling where a few WAVs are enough, ideally without
managing PyTorch ourselves.

## 2. Technical Specification / Findings
### TalkWithMe (README and current source)

- TalkWithMe is a local single-user Python 3.10+ web chat app. Its required LLM is an OpenAI-compatible
  endpoint, with the README recommending a local `llama.cpp` server. TTS and STT are optional,
  separately configured HTTP services; the app itself does not bundle an STT or TTS engine.
- TTS uses the `tts-serve` API. The README names OmniVoice, Qwen3-TTS, and dots.tts as supported
  examples; current `tts-serve` also supports LuxTTS, Chatterbox, Index-TTS, VoxCPM, and others.
  Persona voice conditioning uses a `ref.wav` plus a nonblank `ref.txt` transcript, and optional
  `language.txt`; both audio and transcript are needed by TalkWithMe's TTS client.
- STT is any OpenAI-compatible service exposing `/v1/audio/transcriptions`. The README recommends
  `whisper-fastapi`; it does not ship STT itself. Its suggested low-VRAM setup runs that server on CPU.
- This is zero-shot/reference-audio voice cloning, not training a persistent model inside TalkWithMe.
  You can supply a reference recording per persona, but a transcript is required by TalkWithMe.
- TalkWithMe's own `requirements.txt` contains FastAPI, Uvicorn, HTTPX, Jinja, Pydantic, PyYAML,
  multipart and file helpers; no PyTorch, TTS, or STT package. This is because those engines run as
  separate services. LuxTTS is the most relevant current `tts-serve` option: its server supports one
  reference WAV without a supplied transcript (it transcribes the clip with Whisper internally),
  claims under 1 GB VRAM, and has a CPU path. However, its install is not PyTorch-free: the LuxTTS
  server imports PyTorch, and setup requires a separate Python environment, a Git checkout, pip
  dependencies (including Git-only `linacodec`), plus the `zipvoice` package. LuxTTS model weights
  download from Hugging Face on first run by default. CPU mode uses an ONNX-based model path, but
  that does not remove the server's PyTorch dependency.
- TalkWithMe's repository license is MIT (copyright 2026 Steve Corbett). Its `tts-serve` wrapper
  repo is also separate from the app; check individual engine/model licenses before adoption.

Sources inspected: [TalkWithMe README and source](https://github.com/scorbo2/TalkWithMe), its
[requirements](https://github.com/scorbo2/TalkWithMe/blob/master/requirements.txt) and
[MIT license](https://github.com/scorbo2/TalkWithMe/blob/master/LICENSE).

### tts-serve (README, common API, implementations, and license)

- `tts-serve` is a wrapper framework, not a TTS model: it launches a local FastAPI server around one
  selected engine and presents a common REST interface. Routes include `GET /health`,
  `GET /capabilities` (engine-specific request schema and supported settings), `GET /docs`, and
  `POST /synthesize`. The POST takes JSON including `text` and, for cloning engines,
  `audio_base64` with the reference WAV; `reference_text`, `language`, `seed`, and engine-specific
  parameters vary by engine. It returns the synthesized PCM16 WAV as `audio_base64`, with sample rate
  and timing metadata. Calls are request/response (not a streaming audio protocol); clients can
  submit sentence-sized requests when incremental playback is desired.
- The current README lists Chatterbox, OmniVoice, Qwen3-TTS, Qwen3-TTS (MLX), Faster Qwen3-TTS,
  dots.tts, Index-TTS, LuxTTS, and VoxCPM. Many support reference-audio cloning from one sample;
  transcript requirements differ: e.g. Chatterbox and IndexTTS condition on audio alone, LuxTTS
  transcribes it internally, and Qwen3-TTS can use speaker-embedding-only mode when transcript is
  omitted. Consult each server's live `/capabilities` and engine notes rather than assuming a
  uniform cloning contract.
- Runtime is local and each server loads its model at startup. Model weights normally download on
  first use from Hugging Face, with engine-specific options for offline/local paths. The wrapper's
  shared `tts-engine-common` package uses FastAPI/Pydantic and has no torch dependency, but the
  engines must each have isolated Python environments because their ML dependency trees conflict.
  Most listed engine scripts import PyTorch. Qwen3-TTS (MLX) is the Apple Silicon MLX variant; LuxTTS
  has an ONNX-based CPU path but its server still imports PyTorch. So this reduces client coupling,
  but does not eliminate PyTorch setup for the likely Linux/GPU or LuxTTS service. LuxTTS setup in
  particular involves a Git checkout, a Git-only `linacodec` dependency and `zipvoice`; different
  engines have their own setup burden.
- The `tts-serve` repository is MIT licensed (copyright 2026 Steve Corbett). The license of each
  wrapped engine and model is separate and should be checked independently.
- A Go voxi client can call it directly using `net/http` and JSON: GET `/capabilities`, POST text and
  base64 reference audio to `/synthesize`, then decode the base64 WAV and send it through voxi's
  audio playback path. There is no Python dependency in the Go client. A configurable localhost
  endpoint is sufficient; voxi would still need an installed/running Python engine service.

Sources: [tts-serve README/API/engine list](https://github.com/scorbo2/tts-serve),
[common API and capabilities](https://github.com/scorbo2/tts-serve/tree/master/tts-engine-common),
[engine implementation notes](https://github.com/scorbo2/tts-serve/tree/master/impl), and
[MIT license](https://github.com/scorbo2/tts-serve/blob/master/LICENSE).

### Linked video

- Title: “Running AI group chat in very low VRAM with LuxTTS”. The description says it examines
  whether TalkWithMe can run with chat, TTS and STT in 4 GB VRAM, describes LuxTTS as a voice-cloning
  TTS claiming to use 1 GB VRAM, and has chapters on STT optimization, LuxTTS, LLM optimization and
  minimal/modest setups. It notes that AI-generated voices are marked on screen.
- The title and description were accessible via `yt-dlp` metadata. English automatic captions were
  listed, but fetching them failed with YouTube HTTP 429; transcript content was not accessible.

### Recommendation

**Keep tts-serve as a strong integration candidate; skip TalkWithMe as a direct voxi dependency and
do not treat either project as a demonstrated fix for issue 154.** The common local HTTP API is easy
for Go to call, lets voxi swap speech engines without binding itself to their Python APIs, and
supports reference-WAV voice cloning across several engines. A separately installed service
isolates its Python/PyTorch conflicts from voxi itself. However, the server setup still requires
Python and usually PyTorch, which misses the preference to avoid managing PyTorch; LuxTTS's ONNX CPU
path does not remove its server's torch dependency. If a local Python service is acceptable, keep
tts-serve and canary LuxTTS or another reference-audio engine against the long-text corpus, including
word retention, before choosing it. The docs specify request/response rather than streaming, so
voxi would need sentence chunking/queueing for eager playback. If Python/PyTorch-free deployment is
a hard requirement, skip tts-serve for production and continue investigating ONNX-native embedding
or packaging a service behind a similar local protocol.

## 3. Implementation & Verification Plan
Research completed: TalkWithMe and tts-serve findings, local runtime and dependencies, licensing,
Go integration fit, and video metadata are summarized in section 2. The transcript fetch was
rate-limited (HTTP 429), so only the video title and description were reviewed. No implementation
or code verification is part of this research ticket.
