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
| `gemini-transcribe` (issue 201) | HTTPS `POST .../models/...:generateContent` | Hosted Google Cloud REST API | Cloud hosted (no local weights) |

The first two are self-contained: select the model and it works. The third is
not, and that asymmetry is the source of most of this doc. The fourth is fully
remote, requiring an API key (`GEMINI_API_KEY`) and no local weights.

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
- `wake_url` — POSTed `{"model": <api_model>}` when a recording starts, fire and
  forget, so a stopped server loads while the user speaks (lmcoder `/lmcoder/wake`).
- `transcribe_timeout_ms` — per-model request timeout (default 30 s). R2T2 via
  lmcoder uses 90 s: the proxy holds a request up to 60 s while the local server
  cold-starts. `drain.delivery_deadline_ms` in `spec/eager.yaml` must stay above it.

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

`r2t2-confucius4` (the one to use) is served by lmcoder: its `asr` service
(`lmcoder-asr.service`, part of lmcoder's default setup, lmcoder issue 171) runs
the model, and voxi sends requests to lmcoder's proxy at
`http://127.0.0.1:8735/v1` with `model=r2t2`. On t14 the proxy prefers x600's R2T2
to take load off the laptop, starts the local server when x600 is not serving and
stops it after x600 has been healthy for a while (lmcoder issue 173); voxi does not
know which host answers. lmcoder downloads the model files.
If lmcoder is not installed, select `r2t2-confucius4-voxi`, the fallback in voxi's
own `voxi-r2t2.service` (issue 181). That service runs:

```
llama-server -m ~/.cache/voxi/models/Confucius4-R2T2-Q4_K_M.gguf \
  --mmproj ~/.cache/voxi/models/mmproj-Confucius4-R2T2-Q8_0.gguf \
  -ngl 99 -c 4096 --no-warmup --port 18131
```

`voxi install` writes and manages `voxi-r2t2.service` for the fallback entry. It resolves
`llama_server_path` from `~/.config/voxi/config.yaml`, then falls back to
`llama-server` on `PATH`; an active R2T2 selection fails with a clear error if
neither resolves. The unit port is generated from this model's `base_url` in
`spec/models.yaml`. Installation enables it only when the selected
`openai-transcribe` model has the same `127.0.0.1` host and port, and only when
both the model and mmproj files exist under `~/.cache/voxi/models/`. Other
selections stop and disable the service.

### Trap: voxi starts its own R2T2 server by endpoint match

voxi decides whether to run `voxi-r2t2.service` by comparing the selected model's
`base_url` with the fallback entry's (`IsR2T2Active`). Pointing two entries at the same
port makes voxi start its server there; that is why the lmcoder-served default and the
voxi-hosted fallback are separate entries on different ports (issue 181).

### Trap: a running server hides missing model files

llama-server keeps the model in memory, so a health check passes after the files are
deleted and the server fails on its next restart (seen 2026-10-06, issue 181). Check the
files, not only the endpoint.

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

Run over 24 labelled dictation samples: short clips 0.5–1.0 s, sentences 1.4–2.0 s.
Startup is ~25 s, so a resident server is mandatory; per-utterance CLI invocation
throws that away.

### Streaming is not reachable

R2T2's headline feature is 80 ms–2 s chunk streaming. Mainline llama.cpp accepts
only a whole `input_audio` blob per request (`tools/server/server-common.cpp`),
and has no incremental audio path. So R2T2 in voxi is a **whole-utterance**
engine, like crispasr. Native streaming would need NetEase's own runtime
(vLLM/ROCm).

## Accuracy and vocabulary biasing

Measured on the private dictation samples (then `~/.config/voxi/samples/corpus.tsv`, now
the sample store, see `docs/SampleStore.md`). R2T2 is fluent but mangles project jargon: Voxi→"Foxy",
voxtype→"box type", PipeWire→"pipe wire", harnez→"harness", uman→"human".

**The fix exists and is verified**: `/v1/audio/transcriptions` accepts a `prompt`
field which llama.cpp uses as the literal ASR instruction
(`tools/server/server-chat.cpp:653`). Passing the samples' `keyterms` as the
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

Corollary for sample work: check clip duration against the expected text before
blaming the model. `bug-d-etc.wav` is 0.64 s long while its transcript claims a
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

**Placement and actionable advice (issue 198)**: when the backend is OFFLINE, the
monitor explicitly identifies which component provides the endpoint (e.g.
`lmcoder-proxy.service` vs `voxi-r2t2.service`) and advises running `voxi doctor --fix`
or inspecting the corresponding systemd unit.

## Robust Fallbacks, Client Failover, and Self-Healing (issues 196, 197)

Multi-tier resilience protects against offline upstream hosts (e.g. `x600` down) and
backend crashes:
1. **Lmcoder Proxy Degraded Mode (lmcoder issue 196)**: `lmcoder proxy` tolerates initial
   SSH tunnel failures at startup when `upstream` is configured, immediately opening port
   `8735`, serving local models on `GET /v1/models`, and routing ASR requests to the local
   `lmcoder-asr.service` (port 8749) via `upstream_first: [asr]`.
2. **Eager Transcription Failover (voxi issue 197)**: If port 8735 fails during an eager
   dictation session, `transcribeOpenAIWAV` automatically attempts local fallback endpoints
   (`http://127.0.0.1:18131/v1` and `http://127.0.0.1:8749/v1`) before giving up, ensuring
   spoken utterances are not dropped.
3. **Doctor Health & Automated Repair**: `voxi doctor` probes the active ASR engine (`asr-engine`
   check) and provides concrete repair actions: restarting `lmcoder-proxy.service`, starting
   `voxi-r2t2.service`, or switching the active model to `r2t2-confucius4-voxi`.
4. **Automatic Reversion Watchdog**: `voxi-agent.service` runs a background loop that probes
   primary port 8735 when a fallback model (`r2t2-confucius4-voxi`) is active. As soon as the
   primary endpoint recovers, it automatically reverts settings to `r2t2-confucius4` and stops
   `voxi-r2t2.service` to reclaim GPU VRAM and system memory.
5. **Scriptable Configuration**: `voxi settings set model <name>` provides non-interactive,
   programmatic model switching without requiring interactive TUI interaction.

## Model Onboarding & Runtime Architecture Principles

Adding new ASR or TTS models (see `@docs/TestedModels.md` for historical evaluations)
follows strict runtime discipline:
1. **Desktop Runtime Hygiene**: No heavy PyTorch or persistent Python environments
   in standard desktop installations. High memory overhead, dependency bloat, and
   breakages during distro Python upgrades conflict with Voxi's lightweight desktop goals.
2. **Containerized Spikes First**: Exploratory evaluation of novel architectures or
   unsupported weights runs in isolated Podman/Docker containers.
3. **Lean Runtime Extraction**: Before production onboarding, runtime dependencies must
   be extracted into self-contained formats: GGUF (`llama-server`), ONNX (`sherpa-onnx`,
   `piper-phonemize`), or compiled standalone binaries (`crispasr`, `voxtype`).
4. **Disk Quota Awareness**: Models should not fill the root partition. Pre-flight checks
   (`df -h`) enforce >= 10 GiB free headroom before downloading large weights, redirecting
   scratch and heavy build artifacts to `/mnt/kdev/scratch` or specced cache directories.

## Open gaps

- `voxi bench` supports multi-engine dispatch (`whisper` and `cohere-transcribe` via
  issue 133); expanding coverage to `openai-transcribe` and `gemini-transcribe` remains open.
- Speech-context bench fixtures need migration to the sample store layout (`@docs/SampleStore.md`,
  issue 176).
- Keyterms are not wired into the `openai-transcribe` request prompt yet (issue 137).
