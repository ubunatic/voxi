# 182 — Remove unused model files to free disk space

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Enhancement
**Related**: issue 181 (R2T2 moves to lmcoder), 160 (`voxi install --voxcpm`), `spec/models.yaml`, `spec/tts.yaml`

---

/goal voxi can list every model file it downloaded, mark which ones the current config uses, and
delete the unused ones on request, so unused models stop taking disk space. Stop and report when
blocked on a user decision or denied permission.

## 1. Problem & Motivation
voxi downloads models for engines the user no longer runs and never removes them. The user had to
delete voxtype models by hand. R2T2 is the default and best ASR model, so the others are rarely
used. They stay supported (Cohere may improve again), but their files should not sit on disk.

## 2. Findings (2026-10-06, t14)
- `~/.local/share/voxi/voxcpm/` 871 MB: binary + `voxcpm-0.5b-q8_0-audiovae-f16.gguf`. Unused
  locally: config has `tts_voxcpm_host: x600`, so VoxCPM runs on x600.
- `~/.local/share/voxtype/models/` 608 MB: `ggml-base.en.bin` 142 MB, `ggml-small.en.bin` 466 MB
  (Whisper; `small.en` is the CPU fallback of `large-v3-turbo`). Unused with `asr_model: r2t2-confucius4`.
- `~/.local/share/voxi/voices/` 242 MB: four Piper voices + `cloned.wav`. Only `cloned.wav`
  (VoxCPM reference) and the configured Piper voice are in use.
- Cohere model location not checked yet.
- `~/.cache/voxi/models/` (R2T2) is already gone, see 181.

## 3. Plan
- A command (e.g. `voxi models` with `list` and `prune`) that knows every model path from the
  specs, shows size and "in use by current config / unused", and deletes unused files after
  confirmation (`--yes` for scripts). Never touch `samples/`, `cloned.wav` or other user data.
- `voxi install` re-downloads a removed model when the user switches back to its engine.
- Verify: on t14, prune frees the unused files above, and dictation and `voxi say` still work.
