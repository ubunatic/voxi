# 156 — voice train: Piper fine-tuning fails (espeak path length, epoch offset, torch weights_only)

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Major
**Category**: Bug
**Related**: issue 151, issue 153, issue 154, `internal/tts/clone/train.go`, `scripts/voice-training/`

---

## 1. Problem & Motivation
`voxi voice train` does not work on a fresh machine. Found live on 2026-09-27. Low priority because
issue 154 and 155 aim to replace Piper fine-tuning with PyTorch-free Pocket TTS cloning; decide after 154
whether to fix or remove `voxi voice train`.

## 2. Technical Specification / Findings
1. **espeak-ng path length:** `uv sync` builds piper-tts from git in `~/.cache/uv/git-v0/...`; espeak-ng's
   phoneme compiler truncates paths at ~179 chars, so the build fails ("Bad vowel file"). The PyPI wheel lacks
   `piper.train.vits`. Working manual route: clone v1.3.0 to a short path, editable install, run
   `build_monotonic_align.sh`, copy `espeakbridge.so` from `_skbuild/.../cmake-install/src/piper/`.
2. **Epoch offset:** the base checkpoint is at epoch 2164, so `--epochs 100` (max_epochs) ends at once. Must pass
   checkpoint epoch + N.
3. **torch.load weights_only:** PyTorch ≥ 2.6 refuses the checkpoint (`pathlib.PosixPath` global). Allowlist that
   one class via `torch.serialization.add_safe_globals` rather than disabling weights-only loading.
4. Issue 151 follow-up: `runner.py` must keep the prior ONNX until training succeeds.

## 3. Implementation & Verification Plan
Decide after issue 154: fix all four, or remove `voxi voice train`.
