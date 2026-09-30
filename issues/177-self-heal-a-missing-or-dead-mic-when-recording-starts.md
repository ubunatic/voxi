# 177 — Self-heal a missing or dead mic when recording starts

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Major
**Category**: Feature
**Related**: 026 (eager streaming, `internal/eager/eager.go` pw-record launch), 029 (agent mode)

---

/goal When Super+X starts a recording and PipeWire has no usable default input, voxi repairs the
audio defaults, switches a connected Bluetooth headphone to headset mode, and shows a desktop
notification if it changed anything. Verified live by reproducing the broken state. Stop and ask
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

On recording start (Super+X):

1. Detect "no usable input": no default source, the default names a node that does not exist, or
   the default is a Bluetooth source whose card has no HFP/HSP profile active.
2. Repair it:
   - Clear the stale saved default (`wpctl clear-default` or the equivalent).
   - Switch each connected Bluetooth headphone card that offers an HFP/HSP profile to that profile
     (headphones only, not speakers).
   - Re-check. If still nothing usable, fall back to the built-in mic as default.
3. If any setting changed, send one desktop notification listing what changed (e.g. "Mic
   repaired: Der Kopfhörer → headset mode"). No notification if nothing changed.
4. Then start the recording as usual. The repair must not noticeably delay a healthy start: the
   check runs first and is cheap.

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
