# 177 — Self-heal a missing or dead mic when recording starts

**Status**: Closed — implemented and verified live 2026-09-30
**Priority**: P2 (Medium)
**Severity**: Major
**Category**: Feature
**Related**: 026 (eager streaming, `internal/eager/eager.go` pw-record launch), 029 (agent mode)

---

/goal When Super+X starts a recording and PipeWire has no usable default input, voxi stops that recording, repairs the
audio defaults, switches a connected Bluetooth headphone to headset mode, and notifies the user to
record again. Healthy starts stay as fast as today. Verified live by reproducing the broken state. Stop and ask
the user if a repair step would change settings they did not agree to (see §4).

## 1. Problem & Motivation

The user often switches between Bluetooth headphones and speakers ("Der Kopfhörer", "Die Soundbox",
"Mifa A10", "WH-CH520") and moves the laptop between desk and garden. After this, GNOME Settings
often shows no input device and dictation gets no audio.

Investigation on 2026-09-30 found that the laptop mics were still present. The actual problems:

- WirePlumber's saved default source was a Depstech webcam that was no longer connected.
- The fallback default became `bluez_input.…` for "Der Kopfhörer", but that card was in the
  A2DP profile (playback only), so the source delivers no audio.
- voxi's eager capture runs `pw-record` without a target (`internal/eager/eager.go:397`, `:903`),
  so it follows whatever the broken default is.

## 2. Wanted behaviour

The healthy path must stay fast, so the check runs in parallel with the recording:

1. Super+X starts recording immediately on the current default source (fast path, unchanged).
2. At the same time, a mic check runs. It detects "no usable input": no default source, the
   default names a node that does not exist, or the default is a Bluetooth source whose card has
   no HFP/HSP profile active.
3. If the check finds a problem:
   - Stop the running recording (it is likely capturing silence) and type nothing from it.
   - Repair: clear the stale saved default (`wpctl clear-default` or the equivalent); switch each
     connected Bluetooth headphone card that offers an HFP/HSP profile to that profile
     (headphones only, not speakers); re-check, and if still nothing usable, make the built-in
     mic the default.
   - Send one desktop notification saying what changed and that the user must press Super+X again
     (e.g. "Mic repaired: Der Kopfhörer → headset mode. Please record again."). The first chunk
     is lost, so voxi does not try to restart or salvage it.
4. If the check finds no problem, it does nothing and the recording continues.

## 3. Verification

- Unit tests for the detection logic, run against captured `wpctl`/`pactl` output (testdata).
- Live test: put the headphones in A2DP with a stale default, press Super+X, and confirm the
  notification appears, dictation works and `pactl info` shows the repaired default.
- `make restart-service` (the agent runs this path).

## 4. Open questions

- The user asked to "reset", but `wpctl reset`-style state wipes would forget all saved volumes,
  profiles and routes. Prefer clearing only the default source. Confirm with the user before using
  a full reset.
- Headset mode lowers playback quality while it is active. Should voxi switch back to A2DP when
  recording stops? WirePlumber's `bluetooth.autoswitch-to-headset-profile` may already do this.
  Check it before building our own.
- Several headphones connected at once: which one gets headset mode? Probably the one that is the
  current default sink.

## 5. Resolution

Architecture (`internal/mic`, pure steps, each tested on captured `pw-dump` data):

```
pw-dump --ParseDump--> State --Diagnose--> []Problem --Plan--> []Action --Doctor.apply--> wpctl / pw-metadata
                                          ^ DeadSignal from SignalMonitor (audio itself)
```

- `internal/eager/micwatch.go` runs alongside every recording: one state check at start, then the
  audio is judged every 250ms for the whole session. On a problem: drop the session
  (`sessionDrain.abandon`, `eagerSessionManager.StopSession`), wait until the stream is reaped,
  repair, notify via `notify-send`.
- `voxi mic` shows the setup; `voxi mic --fix` runs the same repair by hand.
- Tuning in `spec/eager.yaml` `mic_check`.
- Agent status now re-checks the daemon when cached as recording, since a session can end itself.

Findings that changed the design (§4 answered):

- WirePlumber 0.5 keeps a Bluetooth loopback input (`bluez5.loopback`) present in A2DP and switches
  to headset mode by itself when recording, restoring A2DP 2s after the stream closes. So
  "headphones in A2DP" is normal, not a fault, and a manual headset switch does not stick. voxi
  does not switch profiles for the loopback; the headset-mode repair remains for plain
  (non-loopback) Bluetooth inputs and for "no default mic at all".
- The real failure seen live: the Bluetooth transport fails and the stream stalls (0.7s of audio
  in 3s). Detected as a stall; repaired by making the built-in mic with an available port the
  default (WirePlumber ignores a default whose port is unavailable, e.g. Mic2).
- Bluetooth headsets emit exact zeros for 1-3s while switching, so all-zero audio only counts as
  dead for built-in mics.
- No full `wpctl` state reset; only the saved default source is cleared.

Verified live: broken Bluetooth default → recording stopped within ~3s, default moved to the
Digital Microphone, notification shown; a healthy built-in recording ran 7s untouched.

Known limit: if 10 utterances queue up behind transcription, capture reads block and could look
like a stall. Not seen; revisit if a false "Mic problem" appears.
