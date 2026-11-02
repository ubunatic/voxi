# 214 — monitor: mic meter shows the level voxi hears (apply Bluetooth gain)

**Status**: Closed — monitor meter now applies the dictation gain to its samples before level computation (shared probe + audio.ApplyGain, 92030f8); BT default shows x2.75 label live in voxi monitor -w; test asserts +8.78 dB at 2.75, non-BT/override 1 unchanged, clipping capped; pw-dump per tick measured 9 ms; make test-q1 green (38 ok).
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Feature
**Related**: 210, 213

---

/goal The `voxi monitor -w` mic meter shows what voxi would hear: the same samples after the same gain that dictation applies (Bluetooth gain from `bluetooth_mic_gain` / spec, 1.0 for non-Bluetooth sources). Verify with a test and a live look; or stop and report when blocked on a user decision.

## Findings
- The meter captures raw audio via parec/pw-record (`internal/monitor/monitor.go:55-66`) into `audiolevel.StartManager` (`monitor.go:332`), with no gain.
- Dictation applies `audio.ApplyGain` with the effective gain (spec `gain_factor` 2.75, overridable by `voxi settings set bluetooth_mic_gain`), only for a Bluetooth default source, and clips at int16 (210 M2/M4/M5).
- Result: with a BT headset the meter reads about 8.8 dB below what voxi hears.

## Plan
- Reuse the dictation code path that resolves the effective gain (Bluetooth predicate + config override) and `audio.ApplyGain` on the meter's chunks, before level computation. Don't copy the logic.
- Show the active gain in the meter label when it isn't 1 (e.g. `MIC ×2.75`), so the boost is visible.
- If the default source changes while the monitor runs, the gain follows it, at the same cadence the dictation probe uses.
- Tests: BT source at 2.75 raises the computed level by the expected dB; a non-BT source is unchanged; override 1 means unchanged; clipping is capped at full scale.
- Live: `voxi monitor -w` with the BT headset, label shows the gain. The monitor runs on demand, so no service restart is needed.
