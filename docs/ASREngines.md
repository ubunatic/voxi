---
title: ASR Engines
weight: 30
---

# ASR Engines

How voxi selects and drives a speech-to-text backend, what each engine costs to
operate, and the traps found while adding the third one. Companion to
`@docs/VoiceInputArchitecture.md` (which covers the audio/typing tiers) and
`@docs/BenchBaseline.md` (RTF numbers).

## The three engine shapes

`spec/models.yaml` gives every model an `engine:`, and `internal/eager` switches
on it. The shapes differ in *who owns the process*, which is the thing that
actually bites.

| Engine | Binary / endpoint | Process ownership | Weights |
|---|---|---|---|
| `whisper` | `voxtype` | voxi execs per utterance | voxtype's own |
| `cohere-transcribe` | `crispasr` | voxi execs per utterance | auto-downloaded to `~/.cache/voxi/models/` |
| `openai-transcribe` | HTTP `POST <base_url>/audio/transcriptions` | **nobody — the user starts it** | the server's problem |

The first two are self-contained: select the model and it works. The third is
not, and that asymmetry is the source of most of this doc.

## Spec fields that drive the HTTP engine

Go must not duplicate spec values (`@docs/Spec.md`), so per-backend behaviour
lives in `spec/models.yaml`:

- `api_model` — the `model` field sent in the request.
- `base_url` — **per-model** endpoint. Resolution order is model `base_url`, then
  the global `openai_asr_base_url` setting, then the built-in default
  (`ResolveOpenAIASRBaseURL`). Without this, two `openai-transcribe` models
  cannot coexist, because the global setting can only point at one of them.
- `response_format` — `text` (default, whisper-server/agy contract: the body *is*
  the transcript) or `json` (llama-server: `{"type":"transcript.text.done","text":…}`).
- `strip_before_marker` — cut everything up to and including the last occurrence
  of a literal marker. Used for R2T2's `language English<asr_text>` scaffold.

`spec/models_schema_test.go` cross-checks models.yaml, the JSON schema and the Go
struct, so a new field must be added in all three or the suite fails.

### Trap: never strip a language-specific prefix literally

R2T2 prefixes output with `language <Name><asr_text>`. Stripping the literal
`language English<asr_text>` works until the user dictates German — and this user
does. Cut at the `<asr_text>` marker instead; a test with a non-English prefix is
what keeps that honest.

## R2T2 (Confucius4-R2T2) via llama-server

Evaluated in issue 131, integrated in 134. Qwen3-ASR based, Apache-2.0 code with
NetEase's own model licence.

Run it with:

```
llama-server -m ~/.cache/voxi/models/Confucius4-R2T2-Q4_K_M.gguf \
  --mmproj ~/.cache/voxi/models/mmproj-Confucius4-R2T2-Q8_0.gguf \
  -ngl 99 -c 4096 --no-warmup --port 18131
```

`voxi install` writes and manages `voxi-r2t2.service`. It resolves
`llama_server_path` from `~/.config/voxi/config.yaml`, then falls back to
`llama-server` on `PATH`; an active R2T2 selection fails with a clear error if
neither resolves. The unit port is generated from this model's `base_url` in
`spec/models.yaml`. Installation enables it only when the selected
`openai-transcribe` model has the same `127.0.0.1` host and port, and only when
both the model and mmproj files exist under `~/.cache/voxi/models/`. Other
selections stop and disable the service.

### Trap: `-c 4096` is not optional on an iGPU

On AMD Cezanne, VRAM *is* system RAM. `llama-server` fits its context to the free
VRAM it sees (~27 GiB here), so an uncapped `-ngl 99` start inflated to **26.5 GiB
VRAM and 99% RAM** and made the machine unusable. With the cap it sits at ~2.7 GiB.
Check `free -m` and `/sys/class/drm/card*/device/mem_info_vram_used` *after*
startup, not only before. Related: keep builds at `-j 6` on this laptop; two
parallel llama.cpp builds pushed the desktop into swap.

### Performance (T14 Gen2 AMD, Vulkan, jfk-reference.wav)

| | RTF |
|---|---|
| whisper small.en | 0.14 |
| R2T2 warm (resident server) | ~0.18–0.25 |
| R2T2 cold (CLI, reloads model) | 0.55 |
| whisper large-v3-turbo | 0.45 |

Corpus run over 24 labelled clips: short clips 0.5–1.0 s, sentences 1.4–2.0 s.
Startup is ~25 s, so a resident server is mandatory; per-utterance CLI invocation
throws that away.

### Streaming is not reachable

R2T2's headline feature is 80 ms–2 s chunk streaming. Mainline llama.cpp accepts
only a whole `input_audio` blob per request (`tools/server/server-common.cpp`),
and has no incremental audio path. So R2T2 in voxi is a **whole-utterance**
engine, like crispasr. Native streaming would need NetEase's own runtime
(vLLM/ROCm).

## Accuracy and vocabulary biasing

Measured on the private corpus (`~/.config/voxi/samples/corpus.tsv`, see
`AGENTS.md`). R2T2 is fluent but mangles project jargon: Voxi→"Foxy",
voxtype→"box type", PipeWire→"pipe wire", harnez→"harness", uman→"human".

**The fix exists and is verified**: `/v1/audio/transcriptions` accepts a `prompt`
field which llama.cpp uses as the literal ASR instruction
(`tools/server/server-chat.cpp:653`). Passing the corpus `keyterms` column as the
prompt corrected Voxi, voxtype and PipeWire in one shot; "dotool"→"two tool"
survived, so biasing is partial and the prompt *shape* matters (bare
pipe-separated terms may be weaker than a wrapping instruction).

This closes a long-standing asymmetry: whisper has `initial_prompt` via voxtype,
crispasr's `--prompt` is a no-op for Cohere (issue 074 §5), and until now no
non-whisper engine had a working vocabulary hook. `openai-transcribe` does.

### No-speech is a sentinel, not an empty string

For audio it judges to contain no lexical speech, R2T2 returns
`language None<asr_text>` with nothing after it — exactly 4 output tokens. After
marker stripping this becomes `""`, indistinguishable from a failed transcription.
It is not a duration effect: `short-yes` (0.92 s) transcribes, `short-uh` (1.06 s)
does not.

Corollary for corpus work: check clip duration against the expected text before
blaming the model. `bug-d-etc.wav` is 0.64 s long while its corpus row claims a
7-word sentence — a mislabelled sample masquerading as a model bug.

## Monitor must not stay silent about the backend

Because nobody owns the HTTP server process, a stopped server means dictation
fails with nothing typed and no visible cause — only per-chunk
`transcribe_error: … connection refused` in `voxi chunks list`.

`voxi monitor` therefore resolves the active model the same way the daemon does
(user settings first — issue 135) and always states the backend's condition
(issue 136): online with endpoint, OFFLINE with endpoint, or "no server" for
voxtype/crispasr. Probing is a 300 ms TCP dial rather than `GET /health`, because
`/health` is a llama.cpp route, not part of the OpenAI-compatible surface; the
result is cached 5 s so the TUI never stalls.

**Placement matters as much as wording**: the first implementation appended the
line to an already-overflowing daemons line, so it was truncated away and never
seen. A status line that can be truncated out of view is not a status line.

## Open gaps

- No supervision for `llama-server`: it does not survive a reboot. Deliberate —
  an auto-start path is a second place the `-c` cap can rot — but a user systemd
  unit carrying the capped command is the intended fix.
- `voxi bench` and `scripts/speech_context_bench` are whisper-only, so RTF and WER
  cannot be compared across engines in one table (issues 133, and 131 option 2).
- Keyterms are not wired into the request prompt yet.
