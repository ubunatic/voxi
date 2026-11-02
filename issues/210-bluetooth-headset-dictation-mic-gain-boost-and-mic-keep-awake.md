# 210 — Bluetooth headset dictation: mic gain boost and mic keep-awake

**Status**: Closed — M5: 'voxi settings set bluetooth_mic_gain <x>' (default=spec 2.75, 1 off, (0,16]) shown in summary/dump/json/menu, read per capture session so no restart; live round-trip set 3 -> default left config.yaml unchanged; make test-q1 green (38 ok) (3c02fcd). Spec-literal leftovers parked in 213.
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

## Decision
Lean sprint 2026-11-02 (dev d210 haiku:med, reviewer opus:med); host decisions on d210's plan:
1. Gain: digital gain in voxi's capture path, only for Bluetooth sources (`bluez_input.*`), factor from spec. No change to system volume.
2. Keep-awake drop-in and autoswitch-off: an explicit opt-in `voxi mic headset` subcommand with an undo (`--off`). Doctor shows read-only status. The "no config files" rule in docs/AudioDoctor.md is amended for this one opt-in drop-in.
3. No M0 capture window: start from the 2026-11-02 host measurement (source volume 1.4 raised soft-speech chunk RMS from 83–109 to 517–1139). Pick the digital factor as the sample-domain equivalent of that boost, clipped to int16. The owner verifies at the end by dictating.
4. Lean milestones: M1 BT predicate + spec block (gate literals stay put unless needed); M2 digital gain + tests; M3 `voxi mic headset` (drop-in, autoswitch, undo) + doctor status line + docs. The live check is the owner's dictation after the host's manual wpctl volume is reset to 1.0 (host asks first).

## Progress
- M1 (BT predicate + spec block) 3715e41, M2 (digital gain) efdc914, M3 (`voxi mic headset` + doctor status) 44b5b7d.

## M4 — pre-work / required refinements (Opus review r210 + host)
1. `gain_factor` 2.75 instead of 6. wpctl software volume is cubic: 1.4 → 1.4³ ≈ 2.74× amplitude. The RMS spread in the measurement came from different utterances, not from the gain. Correct the derivation in the docs.
2. A test that noise-only audio (e.g. constant RMS 13–20 and breath-like bursts) is still rejected by the energy gate with gain applied, and that silence still ends a chunk.
3. Clipping: keep the clamp, and at 2.75 check that a mean-RMS-1000 speech fixture clips less than 1% of samples (test).
4. `--off` restores autoswitch only if voxi turned it off: record the previous value at enable (e.g. a line in the drop-in or a voxi state file) and restore that value. With no voxi record, leave autoswitch untouched (test that no wpctl call is made).
5. Ownership marker: a unique token `# managed-by: voxi mic headset`, matched exactly on line 1. The host's manual drop-in counts as foreign, so enable refuses to overwrite it and prints how to adopt it (or `--adopt`).
6. When enabling, also pin the BT card to the headset (HFP) profile if it is on A2DP (spec profile name), or document why not. Fix the contradictory `Long` help text about playback quality.
7. Spec duplication: use `AutoswitchSetting` from spec (headset_cmd.go:63,82). Make one "Bluetooth node" definition (state.go:218 `bluez_` vs spec node_matches). Drop the Go max that duplicates the schema maximum.
8. `captureGain`: don't block the read loop on `pw-dump`. Resolve the BT state asynchronously, or reuse micwatch's dump, with a short timeout; start reading immediately. Make `TestHeadsetStatusLine` match the exact status word.

## M5 — gain in `voxi settings` (reopened 2026-11-02, owner request)
- Add the Bluetooth mic gain as a user setting: config.yaml key (e.g. `bluetooth_mic_gain`) that overrides the spec `gain_factor` default; 1.0 disables. Validate the range from the spec/schema bounds (no Go copy).
- Show it in the `voxi settings` summary, `--dump`/`--json`, the interactive menu (editable value), and `voxi settings set`; include it in `voxi settings test` only if cheap.
- The daemon reads it at session start (no restart needed if config is read per session; otherwise document `make restart-service`/restart).
- Leftovers from M4: Bluetooth node prefix (state.go:218) and the HFP profile literal (`HeadsetProfile()`) come from spec.
- Tests: default = spec value, override, out-of-range rejected, 1.0 disables, settings dump/json contain it.
