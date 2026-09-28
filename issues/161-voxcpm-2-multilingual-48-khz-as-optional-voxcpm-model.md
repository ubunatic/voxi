# 161 — VoxCPM 2 (multilingual, 48 kHz) as optional voxcpm model

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Feature
**Category**: Feature
**Related**: issue 160 (VoxCPM engine, must be done first), `spec/tts.yaml`

---

/goal Users can select VoxCPM 2 instead of VoxCPM 1/1.5 for the `voxcpm` engine (spec setting plus
`voxi install` option), verified live with the cloned voice — or close with recorded findings if it
misses the bar on this machine; stop and ask when blocked on a user decision or denied permission.

## Motivation
Issue 160 adopts VoxCPM 0.5B (1/1.5): small and fast, but 16 kHz and mainly English/Chinese.
VoxCPM 2 (2B, GGUF Q4 ~1.5 GB, claimed 2.6–3.4 GB peak) is multilingual (e.g. German) at 48 kHz and
fits the 8 GB iGPU VRAM carve-out, but it is ~4x larger and slower. Keep it as a later, opt-in model.

## Plan
1. Canary: run VoxCPM 2 with the runtime chosen in 160 (Vulkan) on the same reference and demo
   texts, plus German texts. Record RTF, peak memory, dropped words, user quality rating vs VoxCPM 1.
2. If acceptable: model choice in `spec/tts.yaml` (default stays the 160 model), pinned GGUF +
   checksum in `voxi install`, docs update.

## Open questions
- Does the 160 runtime (`audio.cpp` / `VoxCPM.cpp`) support VoxCPM 2 GGUF? If not, the runtime choice
  may need revisiting.
