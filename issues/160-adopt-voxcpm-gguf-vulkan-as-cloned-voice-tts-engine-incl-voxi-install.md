# 160 — adopt VoxCPM (GGUF, Vulkan) as cloned-voice TTS engine incl. voxi install

**Status**: Closed — No-go: M1 latency and memory miss the acceptance bar
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
the planned full-set comparison. **Voice quality rating vs Chatterbox: pending
user listening/rating.** No code or install integration is warranted from
this canary result.

## M2 — engine onboarding
- Integration shape chosen from M1: subprocess (Piper pattern) if the CLI loads fast enough per
  sentence, otherwise a dedicated VoxCPM local server/client. Issue 162 removes the tts-serve
  client; build on the preserved TTS synthesis interface and cloned-voice WAV profile.
- `spec/tts.yaml`: engine `voxcpm` (binary/URL, model path, reference WAV + transcript, backend,
  timesteps, chunk cap); Go reads from spec, no duplicated values. Piper stays default.
- Reference transcript: VoxCPM needs the exact text of the reference clip; store it with the voice
  profile (voxi voice prepare), transcribe via voxi's own ASR if missing.
- Unit tests for command/request construction; live `voxi say --no-llm` check; update `docs/TTSReading.md`.

## M3 — `voxi install`
`voxi install --voxcpm`: fetch/build the pinned runtime release
with Vulkan, download the pinned GGUF with checksum, install a systemd --user unit if a server is used,
idempotent re-run, clear errors when Vulkan/`glslc` are missing. Live-verify from a clean state.
User decision (2026-09-28): Chatterbox is too slow and too big, and voxi should avoid PyTorch.
Issue 162 retires Chatterbox/tts-serve (155, 159) and leaves a `voxcpm` placeholder. If M1 passes,
fill that placeholder; any replacement engine must be PyTorch-free.
