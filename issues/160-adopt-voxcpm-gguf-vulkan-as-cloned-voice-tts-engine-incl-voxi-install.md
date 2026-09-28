# 160 — adopt VoxCPM (GGUF, Vulkan) as cloned-voice TTS engine incl. voxi install

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Feature
**Category**: Feature
**Related**: issue 155 (tts-serve backend), issue 159 (`voxi install --tts-serve`), `spec/tts.yaml`,
`internal/tts/`, `docs/TTSReading.md`, source note `~/Downloads/Low-RAM AMD Voice Cloning.md`

---

/goal A `voxcpm` TTS engine speaks in the user's cloned voice via a native C++ runtime (no Python/ROCm),
installed and started by `voxi install`, faster and lighter than Chatterbox — or stop after M1 with a
recorded no-go if the canary misses the bar, and stop to ask when blocked on a user decision or denied
permission.

## Motivation
Chatterbox via tts-serve works (155, 159) but runs ~RTF 10 on CPU and needs ~8 GB free RAM. VoxCPM
(0.5B, GGUF) on `audio.cpp` or `VoxCPM.cpp` claims ~1.2–1.8 GB and RTF ~0.4–1.2 with the Vulkan backend.
Those numbers come from faster hardware (Strix Halo, Zen 5, dGPUs); verify on this machine first.

Machine (2026-09-28): Ryzen 5 PRO 5650U (Cezanne, AVX2, no AVX-512), RADV Vulkan on the iGPU with
8 GB VRAM carve-out, `cmake` and `glslc` present. ROCm is not an option on gfx90c.

## Open questions
- German output: deferred. This ticket uses VoxCPM 1/1.5 (mainly EN/ZH, 16 kHz); VoxCPM 2
  (multilingual, 48 kHz) is a later option in issue 161.
- `audio.cpp` vs `VoxCPM.cpp`: pick in M1 by build success, Vulkan support and CLI/server fit.

## M1 — canary (no voxi code changes)
Build the runtime with Vulkan in a scratch dir, fetch the GGUF, clone from the same 10 s reference
used for Chatterbox and render the same 7 demo texts. Record per model/backend (Vulkan vs CPU):
RTF, peak RSS/VRAM, dropped words (voxtype check), and the user's quality rating vs Chatterbox.
Go bar: RTF ≤ 2 on Vulkan, peak memory ≤ 3 GB, no dropped words, user rates quality acceptable.
Write results into this ticket; no-go closes the ticket with findings.

### M1 results (2026-09-28) — NO-GO

- **Runtime:** `audio.cpp` release `v0.8.2`, commit
  `4d88768fbcae4e6eb3352c6ab1422dabb7d90b58`; source and separate CPU/Vulkan
  builds are in `~/.cache/voxi/voxcpm-canary/`. Both CLI targets built successfully.
- **Model:** `audio-cpp/audio.cpp-gguf`, revision
  `5f57aad57dc0acea2e6a571ec99c71e4035f812d`, file
  `VoxCPM1-GGUF/voxcpm-0.5b-q8_0-audiovae-f16.gguf`; downloaded size
  847,888,032 bytes (808.6 MiB), SHA-256
  `01210319c5ce617613c9d1c38e34649f7479e98a60d51b719f7df82970658241`.
- **Reference:** reused `~/.local/share/voxi/voices/cloned.wav` unchanged
  (18.64 s, mono 16 kHz PCM). Voxi's `voxtype --model small.en --threads 6 -q
  transcribe` output, retained verbatim as the model prompt:
  > Good morning. Today I want to read a short passage at a relaxed pace. I speak clearly, take a breath between sentences and let each word finish before the next one starts. This is how I would like my voice to sound.
- The model's default 240,000-sample AudioVAE reference capacity rejected this
  18.64 s clip. All successful renders used the runtime session option
  `voxcpm1.audiovae_encoder_sample_capacity=320000`; the WAV itself was not
  changed.
- **Exact demo-text source:** `scripts/canary_voiceclone/run.sh` contains only
  demo 1–3. Repo search and tracked history contain no source for demo 4–7, so
  those four inputs were not guessed and were not rendered. M1 is limited to
  the three repository-defined texts.
- **Measurement:** CLI `--metrics` wall time / output duration gives RTF;
  `/usr/bin/time -v` gives process peak RSS; `radeontop` sampled the AMD GPU
  once per second. This iGPU shares system memory: the table's GTT is the
  system-wide observed peak (idle was about 852 MiB), not a process-attributed
  VRAM counter. The reported VRAM heap is the reserved 8 GB carve-out and is
  not useful as a per-render allocation. CPU runs use no model GPU backend.
  `voxtype --model small.en --threads 6 -q transcribe` checked each WAV against
  its exact input text; all six transcriptions matched with zero dropped words.
- Outputs are in `~/.local/share/voxi/voice-demo/voxcpm/{vulkan,cpu}/demo-{1,2,3}.wav`.

| Demo | Backend | Audio duration | RTF | Peak RSS | Peak GPU memory observation | Dropped words |
|---:|---|---:|---:|---:|---|---:|
| 1 | Vulkan | 5.68 s | 3.784 | 665 MiB | GTT 4,344 MiB system-wide | 0 |
| 1 | CPU | 6.32 s | 5.334 | 3,396 MiB | none (CPU backend) | 0 |
| 2 | Vulkan | 4.24 s | 3.785 | 664 MiB | GTT 4,348 MiB system-wide | 0 |
| 2 | CPU | 4.24 s | 6.693 | 3,358 MiB | none (CPU backend) | 0 |
| 3 | Vulkan | 5.04 s | 2.811 | 665 MiB | GTT 3,115 MiB system-wide | 0 |
| 3 | CPU | 5.44 s | 5.873 | 3,377 MiB | none (CPU backend) | 0 |

**Decision:** NO-GO. All three Vulkan RTF measurements exceed 2. All three CPU
peak RSS measurements exceed 3 GB. The missing demo 4–7 inputs also prevent
the planned full-set comparison. **Voice quality rating vs Chatterbox: user rates
Vulkan demo 1–3 "awesome" (2026-09-28).** No code or install integration is
warranted until the M1b tuning round meets the speed and memory bar.

### M1b — tuning round (host, 2026-09-28)
Quality accepted by the user; misses are speed (Vulkan RTF 2.8–3.8 vs ≤ 2) and memory (Vulkan GTT
rise ~3.5 GB over idle, process RSS 665 MiB; CPU RSS ~3.4 GB). One tuning pass on Vulkan before a
final no-go.

Pre-Work / Required Refinements:
- Memory metric: report Vulkan total as process RSS + GTT delta over idle (not system-wide GTT).
- Try `v0.8.2-audio8-perf-hotfix` (`ac16661`) vs `v0.8.2`.
- Shorter reference: cut cloned.wav to a clean ~8 s sentence-aligned clip (default AudioVAE capacity,
  no override); transcript via voxtype; keep the original untouched.
- Fewer inference timesteps and other documented speed knobs (CFG, threads); note quality risk.
- Measure a persistent/server mode if audio.cpp has one, since per-sentence model load is overhead.
- Same demo 1–3 texts; outputs to `voice-demo/voxcpm/tuned/<variant>/`; table per variant; flag the
  best variant and whether it meets RTF ≤ 2 and ≤ 3 GB. The user re-listens to the best variant.

M1b results (2026-09-28):

- Pre-Work: built hotfix `v0.8.2-audio8-perf-hotfix` at
  `ac16661d144f00f84ea0483f3574c374c9868e2d` in scratch. Stable baseline is
  `v0.8.2` at `4d88768fbcae4e6eb3352c6ab1422dabb7d90b58`. Reused the already-cached
  847,888,032-byte GGUF; no model download in M1b.
- Reference: original `cloned.wav` was not modified. Cropped a clean 8.669063 s
  sentence to `~/.cache/voxi/voxcpm-canary/prework/reference-8s.wav` with ffmpeg.
  Voxi `voxtype transcribe` transcript, verbatim: “I speak clearly, take a breath
  between sentences and let each word finish before the next one starts.”
- Rendered the three exact texts from `scripts/canary_voiceclone/run.sh` with
  seed 42. Stable and hotfix defaults use 6 threads, 10 inference steps, CFG 2.0.
  Tuned hotfix uses 12 threads, 4 steps, CFG 1.0 and
  `voxcpm1.mem_saver=true`; 2-step hotfix uses 12 threads, 2 steps, CFG 1.0.
  Lower steps/CFG risk voice and text fidelity. Outputs are under
  `~/.local/share/voxi/voice-demo/voxcpm/tuned/{stable-default,hotfix-default,hotfix-tuned,hotfix-2step}/`.
- RTF is audio.cpp CLI `--metrics` wall time / output duration. `/usr/bin/time -v`
  measured peak RSS. `radeontop` sampled GTT once per second; idle GTT mean was
  889.9 MiB. Combined memory per render is peak process RSS + (render peak GTT -
  idle mean). GTT remains system-wide on this integrated GPU, so this is an
  approximate upper bound. `voxtype transcribe` checked every WAV;
  dropped count is the expected word count minus the longest ordered sequence
  retained in the transcript (substitutions are counted as missing words).
- Persistent mode was attempted with the hotfix `audiocpp_server`, 12 threads,
  4 steps and CFG 1.0. It started, then aborted on the first Vulkan request with
  `GGML_ASSERT(tensor) failed`; no server renders or metrics were produced.

| Variant | Demo | Output | RTF | Peak RSS | GTT delta over idle | RSS + GTT delta | ASR transcript / dropped words |
|---|---:|---:|---:|---:|---:|---:|---|
| stable-default | 1 | 5.28 s | 3.112 | 665 MiB | 1,922 MiB | 2,586 MiB | exact / 0 |
| stable-default | 2 | 2.96 s | 3.546 | 665 MiB | 2,149 MiB | 2,813 MiB | exact / 0 |
| stable-default | 3 | 4.48 s | 2.929 | 665 MiB | 2,162 MiB | 2,826 MiB | exact / 0 |
| hotfix-default | 1 | 5.28 s | 3.025 | 664 MiB | 1,888 MiB | 2,551 MiB | exact / 0 |
| hotfix-default | 2 | 2.96 s | 3.587 | 664 MiB | 1,888 MiB | 2,551 MiB | exact / 0 |
| hotfix-default | 3 | 4.48 s | 3.177 | 664 MiB | 2,221 MiB | 2,885 MiB | exact / 0 |
| hotfix-tuned | 1 | 7.28 s | 3.035 | 664 MiB | 1,859 MiB | 2,523 MiB | “Good morning.” / 12 |
| hotfix-tuned | 2 | 5.28 s | 2.035 | 663 MiB | 1,861 MiB | 2,524 MiB | “Please send me the latest notes when you have more.” / 2 |
| hotfix-tuned | 3 | 3.76 s | 2.355 | 664 MiB | 1,891 MiB | 2,555 MiB | “I will be back shortly, so I will be back shortly.” / 6 |
| hotfix-2step | 1 | 8.48 s | 2.347 | 663 MiB | 1,889 MiB | 2,552 MiB | “I thought you guy!” / 13 |
| hotfix-2step | 2 | 6.56 s | 2.436 | 664 MiB | 2,151 MiB | 2,815 MiB | “Oh no!” / 11 |
| hotfix-2step | 3 | 7.52 s | 2.338 | 663 MiB | 2,164 MiB | 2,827 MiB | “Oh?” / 12 |
| hotfix-server | 1–3 | — | — | — | — | — | server aborted before rendering |

**Best candidate for user re-listen:** `hotfix-tuned` is the practical speed/fidelity
compromise; 2-step has lower mean RTF but lost nearly all words in ASR. Reusable
demo 1 command (replace text and output for demos 2–3):

```sh
/home/uwe/.cache/voxi/voxcpm-canary/build-hotfix-vulkan/bin/audiocpp_cli \
  --task tts --family voxcpm1 \
  --model /home/uwe/.cache/voxi/voxcpm-canary/model/voxcpm-0.5b-q8_0-audiovae-f16.gguf \
  --backend vulkan --threads 12 --seed 42 \
  --voice-ref /home/uwe/.cache/voxi/voxcpm-canary/prework/reference-8s.wav \
  --reference-text 'I speak clearly, take a breath between sentences and let each word finish before the next one starts.' \
  --num-inference-steps 4 --guidance-scale 1.0 \
  --session-option voxcpm1.mem_saver=true \
  --text 'Good morning. I hope your day is off to a calm and pleasant start.' \
  --out /home/uwe/.local/share/voxi/voice-demo/voxcpm/tuned/hotfix-tuned/demo-1.wav --metrics
```

**Host review (2026-09-28):** `hotfix-tuned` is not a usable candidate — ASR drops 2–12 words per
demo. The real best is `hotfix-default` / `stable-default` with the 8 s reference: 0 dropped words,
memory ~2.5–2.9 GiB (memory bar now met), RTF 2.9–3.6 (speed bar missed). Speed looks capped by
this iGPU; the persistent server (which would drop per-call model load) crashed on Vulkan. Open user
decision: accept RTF ~3 and proceed to M2 with `hotfix-default`, or keep the no-go.

**Decision: NO-GO for M2.** All tested variants exceed RTF ≤ 2 on at least one
demo; none meets the speed bar across all three, while each successful variant
stays under 3 GiB by the refined combined-memory estimate. Hotfix-tuned also has
ASR omissions, and 2-step is substantially worse. User re-listen to the
hotfix-tuned renders remains **pending**; no quality rating for M1b is assigned.

## M2 — engine onboarding
- Integration shape chosen from M1: subprocess (Piper pattern) if the CLI loads fast enough per
  sentence, otherwise a dedicated VoxCPM local server/client. Issue 162 removes the tts-serve
  client; build on the preserved TTS synthesis interface and cloned-voice WAV profile.
- `spec/tts.yaml`: engine `voxcpm` (binary/URL, model path, reference WAV + transcript, backend,
  timesteps, chunk cap); Go reads from spec, no duplicated values. Piper stays default.
- Reference transcript: VoxCPM needs the exact text of the reference clip; store it with the voice
  profile (voxi voice prepare), transcribe via voxi's own ASR if missing.
- Unit tests for command/request construction; live `voxi say --no-llm` check; update `docs/TTSReading.md`.

Pre-Work / Required Refinements (host, 2026-09-28, after M1b):
- User decision: connect despite RTF ~3 (speed bar waived); quality is the priority.
- Runtime: audio.cpp `v0.8.2-audio8-perf-hotfix` (`ac16661`) Vulkan CLI, default steps (10) and CFG
  (2.0) — never the 2/4-step variants (they drop words). Subprocess per sentence (server crashed).
- Two selectable voice presets, switchable by config, default `full`:
  - `full`: original 18.6 s `cloned.wav` + its transcript, session option
    `voxcpm1.audiovae_encoder_sample_capacity=320000` (M1; user prefers its end-of-phrase tone;
    ~0.5–1 GB more memory, same speed).
  - `short`: 8.7 s `reference-8s.wav` + its transcript (M1b; ~2.5–2.9 GB total).
  Both transcripts are verbatim in the M1/M1b results above.
- Paths may point into `~/.cache/voxi/voxcpm-canary/` for now; M3 moves install to a pinned location.
- Live check: `voxi say --no-llm` with each preset after `make restart-service`; the user restarts
  `voxi monitor` (it reads config once at start).

### M2 delivered (2026-09-28)
- Added a spec-driven `voxcpm` CLI engine with Vulkan, 12 threads, seed 42, 10 steps, CFG 2.0,
  and a 1,000-character chunk cap. Piper remains the default backend.
- Added `tts_voxcpm_preset` (`full` or `short`; empty selects spec default `full`). Both presets carry
  their exact WAV and transcript in the spec; `full` adds the 320,000-sample AudioVAE option.
- `Synthesize` invokes the hotfix CLI once per queue chunk, validates the selected assets, applies
  the configured text cap, and returns the generated WAV through the existing playback path.
- Added spec/schema, config, and command-construction tests; updated `docs/TTSReading.md`.
- `make test` passed; captured output at `/tmp/voxi-issue160-m2-make-test.log`, with no `--- FAIL` lines.
- `make restart-service` succeeded. Live `voxi say --no-llm` checks queued both presets; the service
  journal recorded two completed Vulkan WAV renders per request with the expected reference/transcript
  and full-only capacity option, and no synthesis errors. Monitor was restarted for each config and
  returned to the original config afterward. No user listening rating recorded.
- Runtime/model remain in the canary cache pending M3 installation work.

## M3 — `voxi install`
`voxi install --voxcpm`: fetch/build the pinned runtime release
with Vulkan, download the pinned GGUF with checksum, install a systemd --user unit if a server is used,
idempotent re-run, clear errors when Vulkan/`glslc` are missing. Live-verify from a clean state.
User decision (2026-09-28): Chatterbox is too slow and too big, and voxi should avoid PyTorch.
Issue 162 retires Chatterbox/tts-serve (155, 159) and leaves a `voxcpm` placeholder. If M1 passes,
fill that placeholder; any replacement engine must be PyTorch-free.

Pre-Work / Required Refinements (host, 2026-09-28, after M2):
- Install layout (user addendum, shared with issue 163): only runtime pieces, never repos/build trees,
  into `~/.local/share/voxi/voxcpm/{bin/audiocpp_cli, model/<gguf>, voices/<preset wav>}`. Point
  `spec/tts.yaml` voxcpm paths there instead of `~/.cache/voxi/voxcpm-canary/`.
- Runtime source: prefer the pinned upstream Vulkan prebuilt of `v0.8.2-audio8-perf-hotfix` if one
  exists and runs here (checksum-pinned); otherwise build from the pinned commit in a temp dir, copy
  only the binary, and delete the build tree. No systemd unit (subprocess per sentence; server crashed).
- The 8 s `short` reference was cut from `cloned.wav`; generate it at install from `cloned.wav`
  (same cut points as `~/.cache/voxi/voxcpm-canary/prework/reference-8s.wav`) or copy it; decide in plan.
- Pin GGUF by HF revision + SHA-256 from M1 results. Idempotent re-run skips verified files.
- Live-verify: move the canary cache aside, `voxi install --voxcpm`, `voxi say --no-llm` with both
  presets; then the canary cache may be deleted (ask the user first).
