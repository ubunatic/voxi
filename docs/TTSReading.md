# Text-to-Speech Reading

This document describes the `voxi say` narration path, queue behavior, host and
session selection, latency observations, and pause tuning. For installation
and package prerequisites, see [VoiceInput.md](VoiceInput.md). LLM reading is
tracked in [Issue 146](../issues/146-plan-voxi-say-llm-for-fluent-document-reading.md);
Markdown normalization remains tracked separately in [Issue 145](../issues/145-improve-markdown-text-normalization-for-tts-reading.md).

## Playback path

`voxi say` accepts text arguments, standard input, or a Wayland selection. LLM
narration is enabled by default. `--no-llm` sends the original text; `--llm
<host>` overrides the LLM host. Narrated text is sent to the queue owned by the
persistent `voxi-agent.service` daemon (via `/run/user/<uid>/voxi/tts.sock`). If the
daemon is not running, `voxi say` reports an error unless invoked offline with
`-o, --output <file>` or `--no-play`. If lmcoder is unavailable or the first
request fails, Voxi queues the original input. If continuation fails after the
first paragraph has been queued, Voxi appends the remaining original text.

`SplitText` turns paragraphs and sentence runs into separately synthesized
chunks. The queue manager synthesizes the current chunk and prefetches the next
one only when that next chunk is already queued. Playback starts each chunk's
WAV as it becomes available. If an LLM continuation is still generating after
the current queue drains, playback waits until that continuation is queued.

The standard Super+Y shortcut reads the primary selection with
`--interrupt`. That stops current TTS and replaces the pending queue with the
new selection. Shift+Super+Y reads the clipboard. The repository's shortcut
setup leaves host selection to the user config; this machine's GNOME binding
adds `--llm x600` explicitly.

**Audio Arbiter & Recording Mute Gate (Super+X)**:
When the user begins voice dictation via Super+X, the STT recording engine increments
an active recording epoch (`ActionRecordingStart`). The daemon's TTS playback queue mutes
in-flight audio playback immediately and permanently discards all pending queued chunks
to prevent acoustic feedback into the microphone and ensure TTS does not unexpectedly
resume talking over the desktop when dictation concludes (`ActionRecordingEnd`).
The monitor Stop action stops current playback and clears the queue. Clear removes
pending chunks while allowing the current chunk to finish.

## LLM host and session behavior

Host selection is resolved in this order:

1. `--llm <host>` on the `say` command.
2. `tts_llm_host` in `~/.config/voxi/config.yaml`.
3. `llm.default_host` in the embedded `spec/tts.yaml`, which is `localhost`.

This developer machine sets `tts_llm_host: x600`. The standard shortcut does
not need a host override when the config supplies the intended host.

Each `say` invocation creates a random lmcoder session ID and uses it for the
first paragraph and its continuation. This keeps context within one document
while preventing another request from resuming it. This matters because a
plain `lmcoder prompt` can resume the terminal's previous session by default.
Use a fresh explicit session when measuring isolated requests; reuse a session
only when conversation history is part of the behavior being tested.

Voxi sends the first paragraph with a prompt to narrate it while expecting more
text. It queues that response, then submits the remaining document in the same
session. Playback can overlap continuation generation. The narration prompt
preserves facts and detail, renders Markdown for listening, and does not
request summarization or translation. lmcoder output is captured as a complete
response with `--raw`; Voxi does not stream partial LLM tokens into the TTS
queue. A short first paragraph or a Markdown table in one paragraph can
therefore leave a gap if its continuation takes longer to generate than the
queued speech takes to play.

## Speech synthesis engines

Voxi provides a pluggable text-to-speech engine seam with automatic fallback:

1. **Piper (Neural)**: High-quality, local ONNX neural text-to-speech. Installed as a self-contained, pip-free prebuilt binary under `~/.local/lib/voxi/piper/` and symlinked to `~/.local/bin/piper`. Voice models reside in `~/.local/share/voxi/voices/`.
2. **Festival (`text2wave`)**: Packaged standard synthesizer fallback.
3. **`espeak-ng`**: Lightweight, instant synthetic fallback.
4. **`tts-serve` (cloned voice, opt-in)**: HTTP client to a separately running tts-serve/Chatterbox server; see "Cloned-voice reading via tts-serve" below.

### Configuration & Precedence

TTS backend and voice selection resolve in this order:

1. **Process Environment Overrides**:
   - `VOXI_TTS_BACKEND`: `auto`, `piper`, `festival`, `espeak-ng`, or `tts-serve`.
   - `VOXI_PIPER_MODEL`: Absolute path to a `.onnx` voice model file.
   - `VOXI_PIPER_CONFIG`: Optional path to a `.onnx.json` voice config file.
   - `VOXI_TTS_SERVE_REFERENCE_WAV`: Absolute path to the cloned-voice reference WAV.
2. **Environment File (`~/.config/voxi/env`)**:
   - `VOXI_TTS_BACKEND`, `VOXI_PIPER_MODEL`, `VOXI_PIPER_CONFIG`, `VOXI_TTS_SERVE_REFERENCE_WAV`.
3. **User Configuration (`~/.config/voxi/config.yaml`)**:
   - `tts_backend`: Default `"auto"` (prefers Piper if model and binary are present).
   - `tts_piper_model`: E.g. `/home/uwe/.local/share/voxi/voices/en_US-lessac-medium.onnx`.
   - `tts_piper_config`: Optional custom model JSON.
   - `tts_serve_reference_wav`: Set by `voxi voice clone`; the cloned-voice reference WAV path.
4. **Embedded Spec Defaults (`spec/tts.yaml`)**:
   - `backend.default_backend: auto`
   - `piper.model: ~/.local/share/voxi/voices/en_US-lessac-medium.onnx`
   - `tts_serve.reference_wav: ""` (always empty; see below)

### Multi-Voice & Dialect Library

Voice models in `~/.local/share/voxi/voices/` include:
- `en_US-lessac-medium.onnx`: Clear American English female narrator (default).
- `en_GB-alan-medium.onnx`: British English male scholar/butler dialect.
- `en_GB-southern_english_female-low.onnx`: British English female accent.
- `en_US-bryce-medium.onnx`: Deep, narrative American English male voice.

### Prosody & Pacing Control

- **Fast Listening Mode**: Lowering length scale (e.g. `--length_scale 0.85`) enables rapid document and telemetry consumption while preserving phoneme clarity.
- **Character & Mentoring Cadence**: Slower pacing (e.g. `--length_scale 1.35`) combined with LLM punctuation and pause prompt conditioning delivers expressive character rhythm (e.g. Yoda-style phrasing) without requiring dedicated voice cloning.

## Latency and benchmark findings

Local CPU canary measurements on AMD Ryzen 5 PRO 5650U (Cezanne APU, 12 threads, 23.3 GiB RAM) using `scripts/canary_tts/benchmark.py` on a standard test sentence:

| Engine / Model | Latency | Audio Duration | Real-Time Factor (RTF) | Peak RSS | Character / Dialect |
|---|---|---|---|---|---|
| **Piper (`en_US-lessac-medium`)** | 0.600 s | 3.74 s | **0.160** | 144.6 MiB | American female (natural) |
| **Piper (`en_US-bryce-medium`)** | 0.964 s | 13.15 s | **0.073** | 144.6 MiB | Deep American male |
| **Piper (`en_GB-alan-medium`)** | 0.586 s | 8.35 s | **0.070** | 144.6 MiB | British English male |
| **Piper (`en_GB-southern_female`)**| 0.234 s | 4.80 s | **0.048** | 144.6 MiB | British English female |
| **Festival (`text2wave`)** | 2.219 s | 3.91 s | **0.568** | 373.9 MiB | Package default |
| **`espeak-ng`** | 0.018 s | 3.59 s | **0.005** | 8.2 MiB | Synthetic fallback |

Piper synthesizes ~4x faster than Festival with less than half the memory footprint, achieving sub-second first-chunk audio playback.

## Cloned-voice reading via tts-serve (issue 155)

A fourth backend, `tts-serve`, speaks in a cloned voice built from the user's
own recorded speech. It is opt-in only: `backend.default_backend: auto` never
selects it, even when a tts-serve server is reachable. Select it explicitly
with `tts_backend: tts-serve` in `~/.config/voxi/config.yaml` or
`VOXI_TTS_BACKEND=tts-serve`.

**What it is.** [tts-serve](https://github.com/scorbo2/tts-serve) is a
separately installed local Python/FastAPI server that wraps one voice-cloning
TTS engine (Chatterbox, MIT license) behind a small HTTP API
(`GET /capabilities`, `POST /synthesize`). Voxi is only an HTTP client
(Go `net/http`, no new dependencies); Voxi does not install, launch, or manage
that server. It must already be running at `tts_serve.url` in the embedded
`spec/tts.yaml` (default `http://127.0.0.1:8000`).

**Consent: only clone your own voice.** The voice profile is a private,
local, unencrypted WAV file. Do not clone anyone else's voice without their
permission.

**Setup**

1. Record and transcribe a calm ~10 s sample the normal way
   (`voxi feedback sample record`), then allowlist it for training in
   `~/.config/voxi/samples/voice-training.txt` (issue 153).
2. `voxi voice clone --sample <id>` copies that sample's WAV to
   `~/.local/share/voxi/voices/<name>.wav` (default name `cloned`) and records
   its path as `tts_serve_reference_wav` in `~/.config/voxi/config.yaml`. If
   more than one sample is allowlisted, `--sample` is required; `--name`
   installs multiple named profiles side by side.
3. Set `tts_backend: tts-serve` (or `VOXI_TTS_BACKEND=tts-serve`) and run
   `make restart-service` so the running `voxi-agent.service` picks up the new
   backend and voice profile — `make install` alone does not hot-reload it.
4. `voxi say --no-llm "text"` now speaks with the cloned voice.

`tts_serve_reference_wav` (and its `VOXI_TTS_SERVE_REFERENCE_WAV` environment
override) exist because `spec/tts.yaml`'s `tts_serve.reference_wav` is
embedded in the binary and stays empty by default: the wrapped engine's
request schema requires a non-empty reference clip, so a real path must come
from runtime configuration, not a rebuild.

**Latency.** Chatterbox on CPU costs roughly 10 s of compute per 1 s of
synthesized audio and needs about 8 GB of free RAM. `SplitText` already
breaks queued text into sentence-sized chunks for every backend (see
"Playback path" above); for `tts-serve` this keeps each HTTP request's
synthesis time far under `tts_serve.timeout_ms` (60 s covers only ~6 s of
audio) and lets playback start on the first sentence while later sentences
are still requested and prefetched.

**Request shape.** Each tts-serve server instance wraps exactly one engine
and exposes that engine's own flat request schema (no envelope, no `engine`
selector field, unknown fields rejected). `spec/tts_serve.settings` is merged
as flat top-level request fields (e.g. `seed`, `exaggeration`, `cfg_weight`
for Chatterbox); see the source comment in `internal/tts/ttsserve.go` for the
schema reference.

## Pause trimming and deployment

`spec/tts.yaml` sets `playback.trailing_silence_trim_ms` to 250 and
`playback.trailing_silence_threshold_db` to -42. Synthesis removes at most 250
ms of trailing PCM16 audio whose samples stay below that threshold. This trims
quiet audio recorded at the end of a WAV; it is not a separate inter-sentence
sleep and does not remove player startup overhead between chunks.

The TTS spec is embedded in the binary. Rebuild and install after changing it.
The persistent `voxi-agent.service` owns the synthesis engine and playback queue.
Run `make restart-service` (or `systemctl --user restart voxi-agent.service`) to
activate engine or spec changes in the running daemon. `make install` alone does
not hot-reload the running background service. If gaps remain after a fresh
daemon starts, inspect the WAV tail and player transition separately; increasing
the trim limit cannot remove startup delay.

