# 181 — Coordinate with lmcoder: host VoxCPM model there, voxi hosts models only as fallback

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Feature
**Category**: Coordination
**Related**: issues 158, 178 (cleanup LLM via lmcoder proxy), 126 (ASR via lmcoder), 160, 163 (VoxCPM engine, remote x600), lmcoder repo `../lmcoder`

---

/goal voxi uses lmcoder for every model it can, starting with VoxCPM TTS, and runs its own model
processes only as a fallback when lmcoder is unreachable or lacks the model. Stop and report when
blocked on a user decision, a denied permission, or missing lmcoder-side work.

## 1. Problem & Motivation
voxi still hosts models itself. First target: the VoxCPM TTS model
`~/.local/share/voxi/voxcpm/model/voxcpm-0.5b-q8_0-audiovae-f16.gguf` (847 MB), which voxi runs
through the `audiocpp_cli` subprocess (`spec/tts.yaml` `voxcpm:`, `internal/tts`, `internal/install/voxcpm.go`).
The user wants lmcoder to host it instead.

This is a coordination ticket. The user will set up one agent in `../lmcoder` (hosting side) and one
in voxi (client side). The split of work and the interface between them get agreed here later;
nothing is decided yet.

## 2. Technical Specification / Findings
Open questions for the two agents (not answered yet):
- VoxCPM runs today as a one-shot CLI (`audiocpp_cli`), not a server. How lmcoder hosts it
  (a long-lived audio.cpp server, a wrapper, something else) is lmcoder's decision.
- The request API voxi calls (e.g. an OpenAI-compatible `/v1/audio/speech` on the lmcoder proxy
  8735, selected by model name like 158), and how voice presets / reference WAVs are passed.
- Which side owns download, checksum and the model file location (today: `voxi install --voxcpm`).
- Which other voxi models move next: ASR (126), cleanup LLM (158/178), Piper.
- What fallback means in practice: trigger, timeout, and whether `voxi install` still fetches
  local models by default.

## 3. Implementation & Verification Plan
To be filled by the coordinating agents. Check live state of both repos and recent commits first.
Done when `voxi say` with the VoxCPM backend is served by lmcoder, and still works locally with
lmcoder stopped.
