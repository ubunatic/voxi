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
4. **VoxCPM (cloned voice)**: local audio.cpp Vulkan CLI using a GGUF model and the `full` or `short` reference profile.

### Configuration & Precedence

TTS backend and voice selection resolve in this order:

1. **Process Environment Overrides**:
   - `VOXI_TTS_BACKEND`: `auto`, `piper`, `festival`, `espeak-ng`, or `voxcpm`.
   - `VOXI_PIPER_MODEL`: Absolute path to a `.onnx` voice model file.
   - `VOXI_PIPER_CONFIG`: Optional path to a `.onnx.json` voice config file.
   - `VOXI_TTS_VOICE_REFERENCE_WAV`: Absolute path to the cloned-voice reference WAV.
2. **Environment File (`~/.config/voxi/env`)**:
   - `VOXI_TTS_BACKEND`, `VOXI_PIPER_MODEL`, `VOXI_PIPER_CONFIG`, `VOXI_TTS_VOICE_REFERENCE_WAV`.
3. **User Configuration (`~/.config/voxi/config.yaml`)**:
   - `tts_backend`: Default `"auto"` (prefers Piper if model and binary are present).
   - `tts_piper_model`: E.g. `/home/uwe/.local/share/voxi/voices/en_US-lessac-medium.onnx`.
   - `tts_piper_config`: Optional custom model JSON.
   - `tts_voice_reference_wav`: Set by `voxi voice clone`; the cloned-voice reference WAV path.
   - `tts_voxcpm_preset`: `full` or `short`; defaults to the embedded spec's `full` preset.
4. **Embedded Spec Defaults (`spec/tts.yaml`)**:
   - `backend.default_backend: auto`
   - `piper.model: ~/.local/share/voxi/voices/en_US-lessac-medium.onnx`
   - Piper and playback tuning only.

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

## VoxCPM cloned-voice engine

Chatterbox and the `tts-serve` integration were removed because the CPU path
was too slow and resource hungry. `voxi voice clone` installs an allowlisted
reference WAV. VoxCPM uses `full` and `short` WAV/transcript profiles embedded in
`spec/tts.yaml`; `full` is the default. Select it with `tts_backend: voxcpm` and
choose the profile with `tts_voxcpm_preset: full` or `short`. Synthesis invokes
the local CLI once per chunk. The full reference uses an AudioVAE capacity
override for its 18.6 s clip. Runtime and model paths remain spec values pending
the M3 installer. Missing runtime assets and unknown presets return synthesis
errors. Unknown backend names fail during `voxi say` configuration loading.
Synthesis and playback failures appear in the monitor TTS panel and are logged
to stderr (the agent service journal when running under systemd).

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
