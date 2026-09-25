# Text-to-Speech Reading

This document describes the `voxi say` narration path, queue behavior, host and
session selection, latency observations, and pause tuning. For installation
and package prerequisites, see [VoiceInput.md](VoiceInput.md). LLM reading is
tracked in [Issue 146](../issues/146-plan-voxi-say-llm-for-fluent-document-reading.md);
Markdown normalization remains tracked separately in [Issue 145](../issues/145-improve-markdown-text-normalization-for-tts-reading.md).

## Playback path

`voxi say` accepts text arguments, standard input, or a Wayland selection. LLM
narration is enabled by default. `--no-llm` sends the original text; `--llm
<host>` overrides the LLM host. Narrated text is sent to the queue owned by an
active `voxi monitor -w` process. Without that monitor socket, the command
reports an error. If lmcoder is unavailable or the first request fails, Voxi
queues the original input. If continuation fails after the first paragraph has
been queued, Voxi appends the remaining original text.

`SplitText` turns paragraphs and sentence runs into separately synthesized
chunks. The queue manager synthesizes the current chunk and prefetches the next
one only when that next chunk is already queued. Playback starts each chunk's
WAV as it becomes available. If an LLM continuation is still generating after
the current queue drains, playback waits until that continuation is queued.

The standard Super+Y shortcut reads the primary selection with
`--interrupt`. That stops current TTS and replaces the pending queue with the
new selection. Shift+Super+Y reads the clipboard. The repository's shortcut
setup leaves host selection to the user config; this machine's GNOME binding
adds `--llm x600` explicitly. Super+X toggles dictation; it does not control
TTS. The monitor Stop action stops current playback and clears the queue.
Clear removes pending chunks while allowing the current chunk to finish.

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

## Latency canary

On 2026-09-25, the same Markdown comparison table was sent to the T14's
`qwen3-4b-instruct-2507-q4` localhost server and x600's
`qwen3.8-27b-instruct-q5` server. The benchmark used Voxi's first-paragraph
prompt (`firstPrompt`) and system instruction (`narrationSystemPrompt`) in
`internal/tts/command.go`, `--raw --format plain --no-preamble
--max-tokens 4096`, and a unique explicit session for every request. The two
host requests in each trial ran in parallel. Wall time measures complete
lmcoder command time, not normalized token throughput or audible first-sample
time.

The input table was submitted as one paragraph:

| Dimension | Document/CLI Prompt | MCP Server Tool |
| --- | --- | --- |
| Token Efficiency | Higher prompt bloat; needs examples and usage rules | Compact JSON Schema; only present in tool definitions |
| Reliability | Prone to shell quoting, escaping, and argument errors | Near 100% parameter formatting reliability |
| Multi-line Payloads | High friction: bash quotes, EOF markers, subshell escapes | Clean JSON string encoding |
| Observability and Control | Unstructured stdout capture in bash | Structured responses, status codes, and clean error handling |
| Setup Cost | Zero: just text in AGENTS.md | Requires packaging an MCP server binary/subcommand |

| Host and loaded model | Run 1 | Run 2 | Mean |
| --- | ---: | ---: | ---: |
| T14 localhost, qwen3-4b-instruct-2507-q4 | 36.42 s | 37.44 s | 36.93 s |
| x600, qwen3.8-27b-instruct-q5 | 33.26 s | 29.70 s | 31.48 s |

x600 completed sooner in these two samples and produced more fluent, more
verbose narration. The models ran on different hardware, output lengths
differed, and the sample is small; the result does not establish normalized
model throughput. The calls produced text only and did not measure playback
quality or acoustic pauses. Both servers remained on their original models.

## Pause trimming and deployment

`spec/tts.yaml` sets `playback.trailing_silence_trim_ms` to 250 and
`playback.trailing_silence_threshold_db` to -42. Synthesis removes at most 250
ms of trailing PCM16 audio whose samples stay below that threshold. This trims
quiet audio recorded at the end of a WAV; it is not a separate inter-sentence
sleep and does not remove player startup overhead between chunks.

The TTS spec is embedded in the binary. Rebuild and install after changing it.
The monitor owns the synthesis engine and keeps its loaded binary in memory, so
close and reopen `voxi monitor -w` to activate engine or spec changes.
`make install` does not hot-reload the running monitor. If gaps remain after a
fresh monitor starts, inspect the WAV tail and player transition separately;
increasing the trim limit cannot remove startup delay.
