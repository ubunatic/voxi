# 215 — audio: energy gate thresholds are Go literals, move to spec

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Refactor
**Related**: 210, 213

---

/goal The energy gate values (ThresholdRMS 150, MinMeanRMS 120, MinVoicedFrames 8, MinVoicedRunFrames 7) have one definition in spec, read by internal/audio, eager and debug, with unchanged behaviour; or stop and report when blocked on a user decision.

## Findings (213 developer report)
- `DefaultSegmenterOptions` in `internal/audio/audio.go` holds the literals (docs/Spec.md says spec values must not be copied into Go).
- `eager.go` and `debug.go` build their own `SegmenterOptions`, and `NewAudioSegmenter` fills zero values with the same numbers. This is more than a one-line move, so it was parked from 213.
- Relevant to 210: soft Bluetooth speech after the 2.75× gain landed at mean RMS 125, just above MinMeanRMS 120. A spec home makes tuning visible.

## Jules runs
| kind | session name | Jules ID | PR | state |
|---|---|---|---|---|
| audit | jules-audit-215b | 9348235491313542040 | none | FAILED on Jules (no PR, no branch); trial run, not retried |
