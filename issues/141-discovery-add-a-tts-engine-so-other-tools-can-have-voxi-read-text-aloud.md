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

## 6. Addendum (user, 2026-09-24): gate the MVP behind `voxi monitor -w`

- TTS only runs while `voxi monitor -w` is open. With no monitor running,
  `voxi say` refuses with a clear message (it does not queue silently).
- The monitor has TTS controls from the start: a queue/now-playing panel,
  play/pause, prev/next, stop and clear. Media keys and Super-x come on top of
  these controls; they don't replace them.
- **Quitting the monitor stops everything:** playback, the queue and any engine
  process. After you quit, nothing is left speaking or running.
- Open design point: either the monitor process owns playback, or voxi-agent
  owns it and the monitor holds a lease that ends playback when the monitor
  exits (including on crash or kill). Choose in M1; the crash case needs a test.

### Proposed (awaiting user confirmation)
- Transport: `voxi say` (text from arguments or stdin), delivered to whichever
  process owns playback.
- New text queues behind current playback; Super-x or stop clears the queue.

### Milestones (draft)
- **M1 (canaries + ownership):** compare the packaged engines, probe MPRIS
  media-key routing on GNOME, and decide who owns playback, including the
  monitor-exit and crash semantics.
- **M2 (monitor-gated MVP):** `voxi say` sends to a queue that plays only
  while `voxi monitor -w` is open. Monitor panel and key controls.
  Quitting the monitor stops everything.
- **M3 (media keys + Super-x):** MPRIS prev/next/play/pause; Super-x stops
  playback; voxi does not transcribe its own playback.
- **M4 (install, opt-out):** `voxi install` sets up the engine by default,
  with an opt-out setting.

### M1 results (2026-09-24)

- **Engine canary:** standalone probes in `scripts/canary_tts/` generated
  equivalent prose WAVs for espeak-ng, Festival (`text2wave`), and Mimic. The
  user can listen to `~/.cache/voxi/tts-canary/{espeak-ng,festival,mimic}.wav`;
  these files are local cache artifacts and are not committed. No audio was
  played during the probe. All files are valid mono PCM WAVs: espeak-ng at
  22,050 Hz / 12.31 s, Festival at 16,000 Hz / 13.57 s, Mimic at 44,100 Hz /
  13.49 s. First and repeat process wall times were respectively 0.01 / 0.01 s,
  6.47 / 6.56 s, and 0.30 / 0.31 s. These are synthesis startup timings, not
  listening-quality judgments; engine quality selection remains for the user
  after listening.
- **Package availability:** this Fedora 44 host has Fedora packages available
  for all three (`espeak-ng` 1.52.0-3.fc44, `festival` 2.5.0-29.fc44,
  `mimic` 1.3.0.1-16.fc43). Debian/Ubuntu package names and availability were
  not verified because this host has no `apt` tooling; that portability check
  remains open.
- **MPRIS canary:** `scripts/canary_tts/mpris.py` registered
  `org.mpris.MediaPlayer2.VoxiCanary`; a direct D-Bus `PlayPause` call appeared
  in the canary method log. With GNOME running and dotoold active, sending
  `XF86AudioPlay`, `XF86AudioNext`, and `XF86AudioPrev` through `dotoolc`
  produced no canary method calls. Firefox also owned an MPRIS name in this
  session, so injected-key routing to this canary is not confirmed. The user
  still needs to check physical media keys and routing with other players
  stopped.
- **Ownership/crash canary:** `scripts/canary_tts/ownership.py` started a
  dummy direct playback child configured with Linux `PR_SET_PDEATHSIG`, then
  `SIGKILL`ed its owner. The child exited and the canary passed. This validates
  direct-child termination only; engine-spawned grandchildren were not tested.
- **Recommendation:** let `voxi monitor -w` own the TTS queue, MPRIS name, and
  engine child. Normal monitor exit should stop and wait for the engine and
  discard the in-memory queue; the monitor socket and MPRIS name then disappear,
  so `voxi say` refuses until a monitor starts again. Configure the engine
  child with parent-death signaling for monitor crash/`SIGKILL`, and still stop
  and reap it explicitly on normal exit. The kill canary supports this choice
  for a direct child; descendants need separate containment if any selected
  engine spawns them.
