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
Then render demo 1–3 with the issue 160 M2 settings (hotfix, 10 steps, CFG 2.0, Vulkan).
Record RTF, peak RSS + GTT delta, dropped words, and ssh round-trip overhead per sentence
(copy text in, WAV back). Go bar: end-to-end RTF ≤ 2 including transfer, x600 stays within free RAM.

## M2 — remote engine option
Config selects local vs remote host for the `voxcpm` engine (reuse the `x600` host naming of
`tts_llm_host`); remote runs the same CLI over ssh and fetches the WAV. Clear error and no silent
fallback when x600 is unreachable. Unit tests for command construction; live `voxi say --no-llm`.
