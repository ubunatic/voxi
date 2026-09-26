# 157 — research TalkWithMe (scorbo2) and linked video for local voice cloning / TTS ideas

**Status**: Open
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
[MIT license](https://github.com/scorbo2/TalkWithMe/blob/master/LICENSE), plus
[tts-serve LuxTTS setup/runtime notes](https://github.com/scorbo2/tts-serve/blob/master/impl/server_luxTTS.md).

### Linked video

- Title: “Running AI group chat in very low VRAM with LuxTTS”. The description says it examines
  whether TalkWithMe can run with chat, TTS and STT in 4 GB VRAM, describes LuxTTS as a voice-cloning
  TTS claiming to use 1 GB VRAM, and has chapters on STT optimization, LuxTTS, LLM optimization and
  minimal/modest setups. It notes that AI-generated voices are marked on screen.
- The title and description were accessible via `yt-dlp` metadata. English automatic captions were
  listed, but fetching them failed with YouTube HTTP 429; transcript content was not accessible.

### Recommendation

**Keep as an architectural reference; skip TalkWithMe as a direct voxi dependency or as a fix for
issue 154.** Its useful ideas are a thin client talking to independently replaceable local speech
services, reference-audio persona configuration, and CPU/VRAM-aware operation. LuxTTS is worth a
separate canary if a PyTorch-backed engine is acceptable: it can clone from one WAV and its CPU
path is ONNX-based. It does not meet the preference to avoid managing PyTorch, and no evidence in
these sources establishes that it avoids the longer-text word drops seen with Pocket TTS. Keep
investigating an ONNX-native runtime / packaged service for voxi; do not port the chat application.

## 3. Implementation & Verification Plan
Research completed: TalkWithMe, relevant engines and local runtime, PyTorch burden, licensing, and
video metadata are summarized in section 2. The transcript fetch was rate-limited (HTTP 429), so
only the video title and description were reviewed. No implementation or code verification is part
of this research ticket.
