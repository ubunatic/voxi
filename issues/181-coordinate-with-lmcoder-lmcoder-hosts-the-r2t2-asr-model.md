# 181 — Coordinate with lmcoder: lmcoder hosts the R2T2 ASR model

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Feature
**Category**: Coordination
**Related**: issue 134 (R2T2 engine), 139 (voxi-r2t2 systemd unit), 158, 178 (cleanup LLM via lmcoder proxy), 182 (model cleanup), lmcoder issue 091, repo `../lmcoder`

---

/goal lmcoder serves R2T2 Confucius4 and voxi's default ASR reaches it through lmcoder; voxi's own
`voxi-r2t2.service` is removed or kept only as a documented fallback. Stop and report when blocked
on a user decision, a denied permission, or missing lmcoder-side work.

## 1. Problem & Motivation
R2T2 Confucius4 is voxi's default and best ASR model (`asr_model: r2t2-confucius4`). voxi runs it
in its own user service `voxi-r2t2.service`: lmcoder's vendored llama-server, port 18131, ~1 GB.
It is a plain llama-server GGUF model, which is exactly what lmcoder hosts, and lmcoder already
manages the GPU memory budget of the shared t14 host. A second llama-server outside lmcoder's view
works against that.

This is a coordination ticket: one agent in `../lmcoder` (hosting side), one in voxi (client side).

## 2. Decisions (user, 2026-10-06)
- lmcoder hosts R2T2.
- All other voxi models stay in voxi: Whisper and Cohere (ASR), VoxCPM and Piper (TTS). They are
  audio engines run as voxi subprocesses with voxi-specific settings, and lmcoder has no audio stack.
  The first draft of this ticket (VoxCPM on lmcoder) is dropped.
- The cleanup LLM already goes through lmcoder (158/178); nothing changes there.

## 3. Findings (2026-10-06)
- The R2T2 model files (`~/.cache/voxi/models/Confucius4-R2T2-Q4_K_M.gguf` and its mmproj) are
  no longer on disk; `~/.cache/voxi/` is gone. The running service still answers from memory and
  will fail on its next restart. Whoever starts here: check the files first; lmcoder's model
  registry should own the download from now on.
- R2T2 is called through `/v1/chat/completions`-style transcription (`engine: openai-transcribe`,
  `strip_before_marker: "<asr_text>"`, `spec/models.yaml`), so lmcoder needs the model plus its
  mmproj, and must not apply chat sampling profiles that change the transcript.

## 4. Open points for the two agents
- lmcoder: add R2T2 (GGUF + mmproj, checksums) to its model spec and serve it as a resident model,
  reachable through the proxy 8735 by model name, like voxi's cleanup model (lmcoder 155).
- voxi: point `r2t2-confucius4` at the proxy and model name instead of 18131; then retire
  `voxi-r2t2.service` and its install path, or keep it as a fallback (decide, record here).
- Verify: dictation with R2T2 works through lmcoder after a reboot, with `voxi-r2t2.service` stopped.
