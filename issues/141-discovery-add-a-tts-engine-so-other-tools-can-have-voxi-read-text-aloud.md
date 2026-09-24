# 141 — Discovery: add a TTS engine so other tools can have voxi read text aloud

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Feature / Discovery
**Related**: 139 (`voxi install` owns all units and servers), `@docs/ASREngines.md`, `@docs/InstallationArchitecture.md`

---

## 1. Idea

Voxi turns speech into text. It should also go the other way: turn text into
speech. The first version is a simple synthesizer, with better engines later.
Other tools send voxi some text and voxi reads it aloud. `voxi install` sets up
any server this needs, the same way it now handles the R2T2 ASR unit (139).

## 2. /goal

**This is a discovery ticket.** The agent and the user talk it through and agree
on a written MVP definition, which gets recorded in this ticket. Done means: the
MVP scope, the interface, the first engine and the install story are decided,
and implementation tickets or milestones are filed. No code in this ticket.

## 3. Questions to settle in the chat

- **Callers and interface:** who sends text? Candidates are agents such as
  Claude Code hooks, a CLI (`voxi say "..."`), and other desktop tools. Pick the
  transport: CLI, a Unix socket on the existing `voxi-agent`, or HTTP (for
  example an OpenAI `/v1/audio/speech`-compatible endpoint).
- **First engine:** `espeak-ng` and `spd-say` (speech-dispatcher) are already
  installed on this machine. Is a system synth enough for the MVP, or should it
  start with a neural engine (Piper, Kokoro, ...) behind a local server?
- **Behaviour:** queueing versus interrupting, a stop command, and how it
  interacts with dictation (don't let voxi transcribe its own voice; mute or
  gate while recording).
- **Install:** what `voxi install` provisions (binaries, voices, a user unit),
  whether that is opt-in, and the resource limits. The iGPU memory trap from
  134/139 applies to any GPU model.
- **Monitor:** should `voxi monitor` show TTS backend state like it does for
  ASR (136)?

## 4. Notes

- Before starting, re-check the live code and recent commits. 139 M2 may have
  changed how `voxi install` handles units.

## 5. MVP decisions (user, 2026-09-24)

- **Callers:** mostly other agents, which send longer agent replies to be read
  aloud. The interface must accept multi-paragraph text and split it into
  sentences or paragraphs that can be skipped.
- **First engine:** the best engine that installs with plain `apt`/`dnf` and
  needs little configuration. Findings on Fedora 44:
  - `espeak-ng` is installed and exists on every distro, but sounds robotic.
  - `festival` and `mimic` are packaged in Fedora. Their quality and
    availability on Debian/Ubuntu are still to be checked.
  - Fedora's `piper` package is the gaming-mouse tool, not Piper TTS. Neural
    TTS such as Piper or Kokoro needs a model download, so it belongs to a
    later milestone.
  - Next step: a canary comparing the quality and startup time of the
    packaged candidates, then pick one, with `espeak-ng` as the fallback.
- **Controls:** media keys control playback while voxi is speaking:
  - prev/next jump to the previous or next chunk;
  - play/pause pauses and resumes.

  The likely mechanism is an MPRIS player that voxi registers on the session
  D-Bus, so GNOME sends the media keys to it; a canary should confirm this.
  Starting a recording with Super-x stops playback, and voxi must not
  transcribe its own voice.
- **Install:** `voxi install` sets up TTS by default, including packages and a
  user unit if one is needed. TTS is **opt-out** (the exact flag or setting is
  still to be decided).

### Still open
- The transport for agents: `voxi say` CLI, the voxi-agent socket, or an
  HTTP `/v1/audio/speech` endpoint.
- Whether new text queues behind current playback or replaces it.
- Whether `voxi monitor` shows TTS state.
