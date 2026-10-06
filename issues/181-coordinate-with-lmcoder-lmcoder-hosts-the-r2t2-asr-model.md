# 181 — Coordinate with lmcoder: lmcoder hosts the R2T2 ASR model

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Feature
**Category**: Coordination
**Related**: issue 134 (R2T2 engine), 139 (voxi-r2t2 systemd unit), 158, 178 (cleanup LLM via lmcoder proxy), 182 (model cleanup), lmcoder issue 171 (hosting side, filed 2026-10-06), lmcoder issue 091, repo `../lmcoder`

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
  Re-downloaded by hand on 2026-10-06 from Hugging Face `netease-youdao/Confucius4-R2T2-GGUF`
  (Q4_K_M sha256 fa3cb46c…6466, mmproj Q8_0 sha256 8dc2c67e…cb65). `voxi install` cannot
  download them: it only checks the files exist and fails otherwise (`internal/install/install.go:196`).
- R2T2 is called through `POST /v1/audio/transcriptions` (multipart, not chat completions) (`engine: openai-transcribe`,
  `strip_before_marker: "<asr_text>"`, `spec/models.yaml`), so lmcoder needs the model plus its
  mmproj, and must not apply chat sampling profiles that change the transcript.

## 4. Open points for the two agents
- lmcoder: add R2T2 (GGUF + mmproj, checksums) to its model spec and serve it as a resident model,
  reachable through the proxy 8735 by model name, like voxi's cleanup model (lmcoder 155).
- voxi: point `r2t2-confucius4` at the proxy and model name instead of 18131; then retire
  `voxi-r2t2.service` and its install path, or keep it as a fallback (decide, record here).
- Verify: dictation with R2T2 works through lmcoder after a reboot, with `voxi-r2t2.service` stopped.

## 5. Addendum (user, 2026-10-06): voxi bootstraps lmcoder
Idea: lmcoder becomes voxi's model server, partly managed by voxi. When voxi finds no lmcoder
(binary, config or running service), it installs lmcoder, writes a minimal lmcoder config that
enables only the proxy and R2T2, and installs the lmcoder service so R2T2 always runs.
An existing lmcoder config is never overwritten; voxi only checks that R2T2 is enabled and says so.

Findings:
- Fits lmcoder's pattern: services are spec entries (`spec/ports.yaml`: unit, port, model, like
  `embed` and `rerank`), models come from the embedded catalog (`spec/models.yaml`, mmproj supported),
  and a config lists the services to run (`services: [proxy, embed]` on t14). R2T2 would be a new
  `asr` service, and voxi's minimal config would be `services: [proxy, asr]`.
- Decided (user, 2026-10-06): module path `ubunatic.com/lmcoder`. The primary repo
  `codeberg.org/ubunatic/lmcoder` is public (the GitHub mirror is private). Done 2026-10-06:
  module renamed (lmcoder 6475cd0, pushed), vanity tags and install line live on
  ubunatic.com/lmcoder (ubunatic.com 43a9b02, deployed). lmcoder v0.1.2 released; `go install ubunatic.com/lmcoder/cmd/lmcoder@latest` verified from a clean GOBIN.
- Open: on t14 the proxy forwards to `upstream: x600`; check that a local `asr` service is still
  reached through the proxy by model name, not sent to x600.
  Finding: today it is not. The proxy reads `model` only from JSON bodies, and voxi sends multipart,
  so the request goes to x600. Tracked in lmcoder 171.

## 6. Status (2026-10-06): voxi switched to lmcoder
- lmcoder side done (lmcoder 171, 97a39c5): `lmcoder-asr.service` on 8749, in lmcoder's default
  stack, enabled at boot; t14 config `services: [proxy, embed, asr]`. The proxy routes multipart
  requests by `model`, so local `asr` wins over `upstream: x600`.
- voxi side done (62bae3c): `r2t2-confucius4` → `http://127.0.0.1:8735/v1`, model `r2t2`.
  Decision: voxi's own server stays as fallback entry `r2t2-confucius4-voxi` (port 18131,
  `voxi-r2t2.service`); only that entry starts it. On t14 `make restart-service` stopped and disabled
  `voxi-r2t2.service`; its model files in `~/.cache/voxi/models` stay (fallback, see 182).
- Verified: `voxi settings test` passes via 8735; live corpus (`VOXI_LIVE_ASR=1`) through lmcoder
  gives the same transcripts as 18131. Two clips (`bug-d-etc`, `short-uh`) return empty
  (`language None`) on both servers, so that test failure is model behaviour, not lmcoder.
- Dictation confirmed by the user (2026-10-06). Open: a check after the next reboot; §5 (voxi installs lmcoder when
  it is missing) is not started.
- Deleted voxi's copy of the R2T2 files (`~/.cache/voxi/models`, 1.4 GB) on the user's request
  (2026-10-06); lmcoder serves from `~/.cache/llama-canary/models`. To use the fallback
  `r2t2-confucius4-voxi`, copy both files from there into `~/.cache/voxi/models` first.

## 7. Next (user, 2026-10-06): x600 first, local R2T2 only as fallback
Goal: take load off the laptop. On t14, R2T2 requests go to x600 when it serves R2T2, else to the
local asr service. The local server may stop while x600 is up, and must start at once when the user
dictates and no server is up. No latency measuring: x600 is assumed faster and the network fast
enough. Split: lmcoder owns routing and the local server's start and stop; voxi keeps calling 8735
and may send a wake signal when recording starts, so a stopped local server loads while the user
speaks. Sent to the lmcoder session; waiting for its contract (wake signal, cold-start behaviour).
- lmcoder answered (its issue 173): x600 already serves R2T2 and needs no config change; the proxy
  will prefer x600, hold a request up to 60 s while it starts local asr, and stop local asr after
  5 min of x600 health and no local use. Wake contract: `POST 8735/lmcoder/wake {"model":"r2t2"}` → 202.
- voxi side done (6a9f007): spec fields `wake_url` and `transcribe_timeout_ms` (R2T2: wake at
  recording start, 90 s timeout), drain deadline 150 s. Open: verify after lmcoder 173 is deployed
  (wake 202, dictation via x600, fallback with x600 unreachable and local asr stopped).
- lmcoder 173 deployed on t14 (lmcoder 8f80539, config `upstream_first: [asr]`). Verified: wake
  answers 202; a transcription through 8735 is served by x600; voxi settings test passes. The lmcoder
  session ran the fallback (both asr servers stopped → local asr started in 2.5 s, request held and
  answered). Open: a user dictation with x600 serving, the local asr stop after 5 min, the reboot check, §5.
