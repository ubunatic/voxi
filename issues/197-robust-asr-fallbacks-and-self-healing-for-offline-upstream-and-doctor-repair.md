# 197 — Robust ASR fallbacks and self-healing for offline upstream and doctor repair

**Status**: Closed
**Priority**: P1 (High)
**Severity**: High
**Category**: Bug
**Related**: lmcoder:issues/196, voxi:issues/181, voxi:issues/189, `spec/models.yaml`, `internal/doctor`

---

## 1. Problem & Motivation
When the upstream LLM/ASR host (x600) is offline or unreachable, `lmcoder-proxy` fails to establish an SSH tunnel and crashes on t14. Consequently, `http://127.0.0.1:8735/v1` (the ASR endpoint for `r2t2-confucius4`) becomes unreachable.
Meanwhile:
1. `voxi doctor` only checked audio hardware/PipeWire services and reported 7 OK, completely oblivious that ASR speech-to-text was dead and voice input could not work.
2. `voxi doctor --fix` had no check or repair mechanism to detect ASR failure, restart proxy, or switch to available local fallbacks (such as `r2t2-confucius4-voxi` on port 18131).
3. Voxi's eager transcription failure logged connection refused and gave up without fallback.

## 2. Technical Specification / Findings
- Added `asr-engine` check to `spec/doctor.yaml` and schema `spec/schemas/doctor.schema.json`.
- Implemented `checkAsrEngine` in `internal/doctor/asr.go`:
  - Validates active ASR engine and probes its HTTP endpoint or CLI binary.
  - When endpoint is unreachable: marks status `FAULT` and suggests concrete repairs (`systemctl --user restart lmcoder-proxy.service`, `systemctl --user start voxi-r2t2.service`, `voxi settings set model r2t2-confucius4-voxi`).
  - When running fallback `r2t2-confucius4-voxi` and primary endpoint 8735 is back online: marks `WARN` and suggests reverting to primary default model and stopping `voxi-r2t2.service`.
- Added `voxi settings set KEY VALUE` subcommand to `internal/settings/command.go` for non-interactive and automated model/settings switching.
- Added transparent fallback retry in `internal/eager/openai_transcribe.go` when proxy port 8735 fails.

## 3. Implementation & Verification
1. Implemented `asr-engine` check in `internal/doctor/asr.go` and wired into `HostDoctor.check`.
2. Added unit tests in `internal/doctor/doctor_test.go` (`TestCheckAsrEngine`).
3. Added non-interactive `voxi settings set` command in `internal/settings/command.go`.
4. Added runtime fallback to `transcribeOpenAIWAV` in `internal/eager/openai_transcribe.go`.
5. Tested live with `voxi doctor` (8 OK, 0 FAULT) and `voxi settings test` (PASS). All package tests pass with `make test-short`.
