# 162 — remove Chatterbox/tts-serve, keep a cloned-voice engine placeholder

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Refactor
**Category**: Refactor
**Related**: issue 155 (tts-serve backend), issue 159 (`voxi install --tts-serve`), issue 160 (VoxCPM),
`spec/tts.yaml`, `internal/tts/`, `internal/install/`, `docs/TTSReading.md`

---

/goal voxi has no Chatterbox/tts-serve code, install path, systemd unit or config, while the interfaces a
cloned-voice engine plugs into (engine selection in spec/config, voice clone profile, the TTS engine
interface/dispatch) survive behind a clearly marked placeholder engine that 160 (VoxCPM) will fill;
`make test` green, Piper default unchanged.

## Motivation
User decision (2026-09-28): Chatterbox is too slow (RTF ~10 on CPU) and too big (~8 GB RAM), and voxi
should avoid PyTorch. Remove it now instead of waiting for 160's canary, but do not lose the integration
seams.

## Scope
Files referencing chatterbox/tts-serve (preflight grep, 349 hits): `spec/tts.yaml`, `spec/tts.go`,
`spec/tts_test.go`, `spec/schemas/tts.schema.json`, `internal/tts/{ttsserve,process,text}.go` + tests,
`internal/tts/clone/profile.go` + test, `internal/config/config.go` + test,
`internal/install/install.go` + test, `systemd/voxi-tts-serve.service`, `docs/TTSReading.md`,
`docs/Roadmap.md`. Leave `issues/` and `docs/studies/` history untouched.

## M1 — remove and placeholder
- Delete the tts-serve HTTP client, `voxi install --tts-serve`, the systemd unit, Chatterbox spec/config keys.
- Keep: engine interface/dispatch, clone profile (reference WAV + transcript), engine enum in spec/schema.
- Add a placeholder cloned-voice engine (e.g. `clone`/`voxcpm` stub) selectable by config that returns a
  clear "not yet implemented, see issue 160" error; unit test for that error and for spec/config parsing.
- Existing configs naming `tts-serve` must fail with a clear message (or map to the placeholder), not panic.
- Docs: `docs/TTSReading.md` and `docs/Roadmap.md` drop Chatterbox usage, mention placeholder + 160.
- Update 160's M2/M3 wording if it references tts-serve as the fallback shape.
- `make test` green; `make restart-service`.
