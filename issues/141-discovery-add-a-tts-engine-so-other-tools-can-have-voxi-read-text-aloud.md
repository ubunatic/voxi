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
  listening-quality judgments. The user selected Festival as MVP and
  espeak-ng as fallback in M1 user verification below.
- **Package availability:** this Fedora 44 host has Fedora packages available
  for all three (`espeak-ng` 1.52.0-3.fc44, `festival` 2.5.0-29.fc44,
  `mimic` 1.3.0.1-16.fc43). Debian/Ubuntu package names and availability were
  not verified because this host has no `apt` tooling; the user has since
  marked Debian/Ubuntu portability out of scope.
- **MPRIS canary:** `scripts/canary_tts/mpris.py` registered
  `org.mpris.MediaPlayer2.VoxiCanary`; a direct D-Bus `PlayPause` call appeared
  in the canary method log. With GNOME running and dotoold active, sending
  `XF86AudioPlay`, `XF86AudioNext`, and `XF86AudioPrev` through `dotoolc`
  produced no canary method calls. Firefox also owned an MPRIS name in this
  session, so dotool-injected routing was not confirmed. The subsequent M1
  physical-key check below confirmed keyboard and Bluetooth-speaker media keys
  reach the canary.
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

### M1 user verification (2026-09-24)
- Physical media keys on the keyboard and on a Bluetooth speaker reach the MPRIS
  canary. MPRIS is confirmed for M3.
- All three engines work; **Festival sounds best** and is the MVP engine.
  `espeak-ng` stays as the fallback.
- Debian/Ubuntu portability is out of scope for now.

### M2 Pre-Work / Required Refinements
- **Festival latency:** the whole text took about 6.5 s. Synthesize per chunk
  (sentence or paragraph) and synthesize chunk N+1 while chunk N plays, so the
  first audio starts quickly. Measure the time to first audio on a
  multi-paragraph agent reply and record it. If it is still slow, consider a
  monitor-owned `festival --server` child.
- **Descendant containment:** `text2wave` is a wrapper script that launches
  `festival`, which is exactly the grandchild case M1 did not cover. Run each
  engine and player in its own process group. On monitor exit, kill the group.
  For crash or SIGKILL, use parent-death signaling or another mechanism that
  reaches the whole group. Add a test that SIGKILLs the owner with a real
  Festival child and asserts no `festival` process is left.
- Canary scripts stay under `scripts/canary_tts/`; the product code is Go.

### M2 results (2026-09-24)

- **First audio:** `scripts/canary_tts/first_audio/` measured 2.328 s from
  starting a multi-paragraph reply to the first Festival WAV being ready and a
  silent `pw-play` test process starting. It emitted no audio. The next
  paragraph finished synthesis while that player process was still active,
  confirming prefetch overlap. This measures readiness/player launch, not
  acoustic onset on a physical output device.
- **Crash containment:** `scripts/canary_tts/crash/`, run against a locally
  built `voxi`, started a real `text2wave` Festival descendant, then SIGKILLed
  its owner. The canary passed: Festival exited and its process group was
  empty. The hidden `voxi __tts-supervise` subcommand sets itself as a child
  subreaper; the monitor starts it with `PR_SET_PDEATHSIG`. Normal shutdown
  also stops and reaps the engine/player process groups.
- **MVP:** `voxi say` accepts arguments or stdin only while `voxi monitor -w`
  owns the queue socket. It queues sentence/paragraph chunks; the monitor panel
  exposes playback, previous/next, stop and clear controls. The runtime socket
  is removed on normal exit, stale sockets are replaced on next start, and a
  missing/stale socket returns a bounded “no monitor” error.

### M2 delivered (8e07ce3): monitor-gated MVP
Host review: the diff is scoped. The runtime-path helper also replaced the
agent socket's hand-rolled path, which is harmless. `go test ./...` is green.
Live: with no monitor running, `voxi say` refuses with exit 1 and a clear hint.
Monitor keys: m play/pause, b/n prev/next, x stop, k clear.

### M3 Pre-Work / Required Refinements
- Wait for the user's live check (audible playback, the keys, and quitting the
  monitor stopping speech) before starting M3. Record any findings here.
- **User live check: LGTM.** Audio, keys and quit-stops-speech all work.
  First audio in the live panel was 1.321 s.
- **Bug: TTS panel leftovers.** When a line gets shorter, the old text is not
  erased. Observed:
  `TTS · idleingizing` / `Now: (idle)nd paragraph to test skipping.` /
  `Queue: emptynk(s)`. Clear each rendered TTS line to end of line (or pad it
  to the panel width, measuring display width) the way the other monitor rows
  do. Add a render test where a long state is followed by a shorter one.

### M3 results (2026-09-24)

- **Panel line clearing:** every TTS panel row now erases to end-of-line; a
  render regression test emits a long state followed by a shorter idle state.
- **MPRIS:** `voxi monitor -w` owns `org.mpris.MediaPlayer2.voxi` on the session
  bus. It exports the MPRIS root/player properties and PlayPause, Play, Pause,
  Previous, Next, and Stop methods; playback status and current chunk metadata
  follow the monitor queue. A session-bus integration test verified discovery
  and that Next reaches the queue controller.
- **Recording/self-transcription:** `voxi record start` and `toggle` send Stop
  to the monitor-owned TTS queue before starting the selected capture backend.
  With no monitor, the bounded no-monitor response is ignored and recording
  proceeds normally. Super+X uses this same `voxi record toggle` path.
- **Verification:** `go test ./...` and `make install` passed. The test stops
  playback before capture; no recording of Voxi's own TTS is accepted by the
  recording path.

### M3 delivered (4724490): MPRIS media keys, Super-x stops TTS, panel fix
Host review: the diff is scoped, adds godbus/dbus v5, and `go test ./...` is green.

### M4 Pre-Work / Required Refinements
- **Dictation must never depend on TTS.** `stopTTSForRecording` returns an
  error for any failure other than no-monitor, which aborts the recording. A
  TTS socket error or a 2 s timeout must only log a warning; recording goes
  ahead. Also reduce the time recording can wait on TTS: a wedged monitor
  must not add up to 2 s to the Super-x start. Test both.
- Await the user's live media-key and Super-x check. Record findings here.
