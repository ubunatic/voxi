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
- German output needed? VoxCPM 1/1.5 is mainly EN/ZH at 16 kHz; VoxCPM 2 (2B, Q4 ~1.5 GB) is
  multilingual at 48 kHz and fits the VRAM. Ask the user before M1 if not answered.
- `audio.cpp` vs `VoxCPM.cpp`: pick in M1 by build success, Vulkan support and CLI/server fit.

## M1 — canary (no voxi code changes)
Build the runtime with Vulkan in a scratch dir, fetch the GGUF, clone from the same 10 s reference
used for Chatterbox and render the same 7 demo texts. Record per model/backend (Vulkan vs CPU):
RTF, peak RSS/VRAM, dropped words (voxtype check), and the user's quality rating vs Chatterbox.
Go bar: RTF ≤ 2 on Vulkan, peak memory ≤ 3 GB, no dropped words, user rates quality acceptable.
Write results into this ticket; no-go closes the ticket with findings.

## M2 — engine onboarding
- Integration shape chosen from M1: subprocess (Piper pattern) if the CLI loads fast enough per
  sentence, otherwise a long-lived local server behind the existing tts-serve HTTP client/API.
- `spec/tts.yaml`: engine `voxcpm` (binary/URL, model path, reference WAV + transcript, backend,
  timesteps, chunk cap); Go reads from spec, no duplicated values. Piper stays default.
- Reference transcript: VoxCPM needs the exact text of the reference clip; store it with the voice
  profile (voxi voice prepare), transcribe via voxi's own ASR if missing.
- Unit tests for command/request construction; live `voxi say --no-llm` check; update `docs/TTSReading.md`.

## M3 — `voxi install`
`voxi install --voxcpm` (mirroring `--tts-serve` from 159): fetch/build the pinned runtime release
with Vulkan, download the pinned GGUF with checksum, install a systemd --user unit if a server is used,
idempotent re-run, clear errors when Vulkan/`glslc` are missing. Live-verify from a clean state.
Decide with the user whether Chatterbox/tts-serve stays as an option or is retired.
