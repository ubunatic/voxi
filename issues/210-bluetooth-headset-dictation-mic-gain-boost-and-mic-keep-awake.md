# 210 — Bluetooth headset dictation: mic gain boost and mic keep-awake

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Major
**Category**: Feature
**Related**: 177, 189, 208, 209

---

/goal Make Bluetooth headset dictation work out of the box: voxi applies a mic gain boost and keeps the headset mic link awake (opt-in or auto-detected), verified live on a BT headset, or stop and report when blocked on a user decision or denied permission. On a loop or hard PipeWire/WirePlumber blocker, clear the goal and write a failure report here.

## 1. Problem & Motivation
With a Bluetooth headset (Philips TAH4205, mSBC) dictation was poor until three manual host fixes on 2026-11-02:
- Soft speech was dropped by the energy gate (mean RMS 83–109 vs `MinMeanRMS` 120 / `ThresholdRMS` 150 in `internal/audio/audio.go`), although r2t2 transcribed it word-perfect.
- Every Super+X switched A2DP→HFP: GNOME volume OSD popped up and ~2.4 s of speech was lost.
- With HFP pinned, ~0.9 s was still lost after an idle pause: WirePlumber suspends the idle BT source (default suspend timeout), and SCO wake-up drops the first words ("Test one" missing).

These fixes live only on one machine; the owner's other system needs the same.

## 2. Technical Specification / Findings
Manual fixes that worked (verified live):
1. **Gain boost**: `wpctl set-volume @DEFAULT_AUDIO_SOURCE@ 1.4` (hardware gain was already 100%). Chunk mean RMS rose from 83–109 to 517–1139; all transcribed.
2. **No profile switching**: `wpctl settings --save bluetooth.autoswitch-to-headset-profile false` + `pactl set-card-profile <card> headset-head-unit`. Profile stays HFP after captures close; OSD gone. Cost: headset playback at call quality.
3. **Keep-awake**: `~/.config/wireplumber/wireplumber.conf.d/51-voxi-bt-mic-no-suspend.conf` sets `session.suspend-timeout-seconds = 0` for `~bluez_input.*` / `~bluez_capture_internal.*` (`monitor.bluez.rules`). Node stays `idle` instead of `suspended`; first words kept; no tray icon flash; GNOME mic-in-use indicator still accurate (no stream held).

Rejected: holding a parallel idle capture stream keeps the mic-in-use indicator on permanently, which hides voxi's recording state (GNOME extension is deprecated, not an alternative).

Open questions:
- Per-device opt-in setting vs auto-detect when the default source is `bluez_input.*`.
- Install/remove the WirePlumber drop-in from voxi (e.g. `voxi mic` / `voxi doctor --fix`) vs document only.
- Gain: set the PipeWire source volume, or apply digital gain inside voxi's capture path (leaves system volume untouched). Relative/noise-floor gate (option discussed against 208/209) may make a boost unnecessary.
- Security note for docs: keep-awake streams headset mic audio to the PC continuously, but nothing records it unless an app opens a stream; battery cost on the headset.

## 3. Implementation & Verification Plan
- Re-check live state and recent commits first; the host already carries the manual fixes.
- Implement as voxi features with unit tests; document in `docs/AudioDoctor.md` / `docs/MicSelfCheck.md` with undo steps.
- Verify per `docs/AudioDoctor.md`: on a clean host (fixes reverted), enable the feature, then dictate after a ≥30 s pause starting immediately; first words kept, no OSD, soft speech accepted (chunk RMS via `voxi chunks list`).
