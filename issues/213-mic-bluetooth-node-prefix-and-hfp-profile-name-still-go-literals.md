# 213 — mic: Bluetooth node prefix and HFP profile name still Go literals

**Status**: Closed — Bluetooth identity (device_api bluez5, node_prefix bluez_) and HFP profile now from spec bluetooth_mic, one accessor used by mic and doctor (5e39469); existing pw-dump assertions unchanged; make test-q1 green (38 ok); live voxi mic unchanged. Gate literals parked in 215.
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Refactor
**Related**: 210

---

/goal "Bluetooth node" and the HFP profile name have one definition, in spec, used by both detection and the keep-awake drop-in; or stop and report when blocked on a user decision.

## Findings (from 210 reviews)
- `internal/mic/state.go:241` detects Bluetooth by `device.api == "bluez5"` or the `bluez_` prefix, while the keep-awake drop-in uses the spec's `node_matches`. That makes two definitions.
- `internal/mic/state.go:159-167` hardcodes `"headset-head-unit"` (HeadsetProfile), while M4's HFP pin uses a spec profile name.
- The energy gate literals in `DefaultSegmenterOptions` (internal/audio) are still Go literals (docs/Spec.md).

## Plan
Move the values into the existing `bluetooth_mic` spec block, read them through accessors, and keep the tests on the saved pw-dump fixtures.
