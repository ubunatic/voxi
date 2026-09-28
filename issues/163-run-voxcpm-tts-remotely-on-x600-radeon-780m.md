# 163 — Run VoxCPM TTS remotely on x600 (Radeon 780M)

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Feature
**Category**: Feature
**Related**: issue 160 (VoxCPM engine, M2 local subprocess), `tts_llm_host: x600` (existing remote LLM use)

---

/goal `voxi say` can synthesize with the VoxCPM engine on x600's Radeon 780M, selectable by config,
if the M1 canary shows x600 is clearly faster than local (target RTF ≤ 2); otherwise stop after M1
with a recorded no-go. Stop and ask when blocked on a user decision or denied permission.

## Motivation
Local VoxCPM on the 5650U iGPU runs at RTF ~3 (issue 160 M1/M1b). x600 has a much faster iGPU
(AMD Phoenix, Radeon 780M, RADV Vulkan, VRAM/GTT 26.8/34.5 GB) and already serves voxi's LLM step.

## Findings (2026-09-28)
- x600: x86_64, Fedora, `cmake` and `glslc` present, Vulkan works. RAM 45 GB but only ~6 GB free
  while the LLM is loaded; VoxCPM needs ~3–4 GB — tight, measure it.
- Local `build-hotfix-vulkan/bin/audiocpp_cli` (281 MB) links only system libs and embeds its Vulkan
  shaders; built with `GGML_NATIVE=ON` for Zen 3, which Zen 4 runs. So copy the binary instead of
  building; build on x600 only if copying fails or is slow.

## M1 — canary on x600 (no voxi code changes)
Copy only the runtime pieces to x600 — never the audio.cpp repo, build tree or voxi repo — into an
XDG location: `~/.local/share/voxi/voxcpm/{bin/audiocpp_cli, model/<gguf>, voices/<preset wav>}`
(user addendum 2026-09-28). The same layout should become the local install target in 160 M3.
**Pre-work (host, after 160 M3):** `voxi install --voxcpm` now exists and installs exactly this
layout locally (source-built 64 MB CLI; the Ubuntu prebuilt SIGILLs on the laptop's Zen 3). On x600
first try the pinned prebuilt from `spec/tts.yaml` (Zen 4 may run it); if it fails, copy the local
`~/.local/share/voxi/voxcpm/` tree. Do not deploy the voxi binary to x600 in M1.
Then render demo 1–3 with the issue 160 M2 settings (hotfix, 10 steps, CFG 2.0, Vulkan).
Record RTF, peak RSS + GTT delta, dropped words, and ssh round-trip overhead per sentence
(copy text in, WAV back). Go bar: end-to-end RTF ≤ 2 including transfer, x600 stays within free RAM.

### M1 results (2026-09-28)

- Installed only `audiocpp_cli`, the pinned GGUF, and `voices/full.wav` under
  `~/.local/share/voxi/voxcpm/` on x600. The pinned Ubuntu x64 Vulkan archive SHA-256 matched
  `spec/tts.yaml`; the CLI reports audio.cpp `v0.8.2-audio8-perf-hotfix`, commit `ac16661`, and
  detected the AMD Radeon 780M (`RADV PHOENIX`). The remote GGUF SHA-256 also matched the spec.
- Rendered the three exact texts from `scripts/canary_voiceclone/run.sh` with the hotfix runtime,
  Vulkan, 12 threads, seed 42, 10 inference steps, CFG 2.0, and the spec's 320,000-sample
  AudioVAE capacity override. Used the spec's full reference WAV and transcript.
- `/usr/bin/time` measured peak process RSS. A 250 ms sampler measured system-wide GTT; because this
  is an integrated GPU, RSS plus GTT delta is an approximate combined-memory upper bound. Before
  demo 1, x600 had 7.07 GiB `MemAvailable` and 26.50 GiB GTT in use. GTT returned to baseline after
  each render; `MemAvailable` after demo 1 was 9.12 GiB.
- SSH time is measured from the local host through the remote render and return of the CLI result;
  WAV return is the separate local `scp`. End-to-end RTF includes both. Local
  `voxtype --model small.en --threads 6 -q transcribe` matched every input exactly (0 dropped words).
- WAVs copied to `~/.local/share/voxi/voice-demo/voxcpm/x600/demo-{1,2,3}.wav` for listening.

| Demo | Audio | CLI RTF | Peak RSS | GTT delta | RSS + GTT delta | SSH round trip | WAV return | End-to-end RTF | ASR / dropped words |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| 1 | 5.28 s | 1.551 | 680 MiB | 3.41 GiB | 4.07 GiB | 11.343 s | 0.195 s | 2.185 | exact / 0 |
| 2 | 4.40 s | 1.224 | 684 MiB | 3.37 GiB | 4.04 GiB | 7.149 s | 0.193 s | 1.669 | exact / 0 |
| 3 | 5.44 s | 1.163 | 683 MiB | 3.39 GiB | 4.06 GiB | 8.944 s | 0.189 s | 1.679 | exact / 0 |

Weighted end-to-end RTF is 1.852 across all three demos. **Decision: NO-GO for M2.** Demo 1's
transfer-inclusive RTF is 2.185, above the ≤2 bar, even though demos 2 and 3 meet it. The measured
memory estimate fits within the available RAM, and all ASR checks passed. The x600 runtime install
is left in place; temporary remote WAVs were removed after copying them locally. Stop at M1.

## M2 — remote engine option
Config selects local vs remote host for the `voxcpm` engine (reuse the `x600` host naming of
`tts_llm_host`); remote runs the same CLI over ssh and fetches the WAV. Clear error and no silent
fallback when x600 is unreachable. Unit tests for command construction; live `voxi say --no-llm`.
