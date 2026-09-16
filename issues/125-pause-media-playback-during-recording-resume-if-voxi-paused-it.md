# 125: Pause Media Playback During Recording, Resume Only If Voxi Paused It

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Enhancement
**Category**: Feature
**Related**: [089 voxi-modifierd](089-voxi-modifierd-not-installed-on-this-dev-machine-modifier-gating-currently-inactive.md), [029 Single voxi agent for mode orchestration](029-single-voxi-agent-mode-orchestration.md)

---

## 1. Problem & Motivation

When the user starts dictating, any music/video/podcast playing in the
background keeps playing and gets picked up by the mic — it competes with
speech for the ASR and (per issue 100/056 threads) is a source of
hallucinated/background-voice artifacts. Pausing playback for the duration
of the recording, and only resuming what Voxi itself paused, removes this
source of noise without the user having to manually pause every time they
start talking.

This ticket scopes the **first, device-agnostic increment only**. Two
enhancements are explicitly deferred (Section 5) at the user's request and
must not be built now:

- Detecting the active playback device type (speakers vs. headphones) and
  only pausing for speakers, since headphone audio doesn't leak into the
  mic.
- Per-device named settings (remember specific devices, whether to pause
  for each).

## 2. Desired Design (This Ticket's Scope)

1. **Pause on recording start**: when a recording session begins
   (`eagerSessionManager.Start()` / equivalent non-eager start path in
   `internal/eager/eager.go`), query active media players and pause any
   that are currently playing.
   - On Linux this is standard MPRIS territory —
     `org.mpris.MediaPlayer2.Player` over D-Bus (`PlaybackStatus`,
     `Pause()`), reachable directly via a D-Bus client library or by
     shelling out to `playerctl` if already a common dependency elsewhere
     in the stack (check first — grep found no existing MPRIS/playerctl
     usage in this repo, so this is new integration surface, not a wire-up
     of dormant code).
   - Multiple simultaneous players (e.g. a browser tab and a media app)
     should each be tracked individually.
2. **Track what Voxi paused**: record, per session, exactly which
   player(s) Voxi transitioned from Playing → Paused. Do not record players
   that were already paused/stopped before the session started.
3. **Resume on recording stop**: when the recording session ends
   (`eagerSessionManager.Stop()` / equivalent), resume (`Play()`) only the
   players Voxi itself paused in step 2. If the user manually paused
   something that was already paused when recording started, or paused a
   player mid-recording that Voxi didn't touch, leave it alone.
4. **Fail open**: if D-Bus/MPRIS is unavailable, no players are found, or a
   pause/resume call errors, log and continue — this must never block or
   delay the recording start/stop path itself. Treat it the same way
   existing notification playback failures are handled (see
   `ScheduleNotify` in `internal/eager/eager.go`, which warns and continues
   rather than failing the session).
5. **Config flag**: add an on/off spec setting (following existing patterns
   in `spec/`) so the whole feature can be disabled without removing code —
   default should be discussed, but off-by-default is the safer initial
   rollout given no device-type filtering exists yet (see Section 5) and
   headphone users would otherwise get spurious pauses.

## 3. Non-Goals for This Ticket

- No playback-device enumeration or classification (speaker vs.
  headphone) — deferred, see Section 5.
- No per-device named settings/preferences UI — deferred, see Section 5.
- No pausing of non-MPRIS audio sources (e.g. raw PulseAudio/PipeWire
  streams without an MPRIS interface) — MPRIS-capable players only for
  this increment.

## 4. Open Questions

- Confirm PipeWire/PulseAudio + MPRIS is reachable without extra
  privileges under the same session context `voxi-agent.service` runs in
  (systemd --user, per issue 029) — needs a canary before implementation,
  not assumed.
- Decide whether to shell out to `playerctl` (simple, one more runtime
  dependency, matches the "shell out to a well-known CLI" pattern already
  used for `dotool`) or talk D-Bus directly from Go (no extra runtime
  dependency, more code). Lean toward checking if `playerctl` is an
  acceptable dependency addition first — cheaper canary.
- Should pause/resume be tied to eager-session start/stop specifically, or
  to the broader agent recording state if there are other recording entry
  points? Needs a read of `internal/eager/eager.go`'s session lifecycle
  before implementation to confirm there's one canonical start/stop hook.

## 5. Deferred Follow-Up Work (Explicitly Not This Ticket)

File as separate follow-up tickets when picked up, rather than growing this
one's scope:

1. **Device-type-aware pausing**: detect whether the active/default
   playback sink is speakers or headphones (PipeWire sink properties should
   expose this) and skip pausing entirely when output is headphones, since
   headphone audio doesn't bleed into the mic.
2. **Named-device settings**: let the user register specific playback
   devices by name and configure per-device whether Voxi should pause for
   them, overriding the automatic speaker/headphone heuristic above.

## 6. Verification Guidance

- Canary first (per `docs/Canary.md`): confirm MPRIS pause/resume works
  against at least one real player (e.g. a browser tab playing audio, or
  a media player like `mpv`/Spotify) from within the same systemd --user
  session context the agent runs in, before writing the session-tracking
  logic.
- Unit-test the "only resume what we paused" bookkeeping in isolation
  (fake player states) — this is the part most likely to have edge-case
  bugs (e.g. session crashes before stop, multiple overlapping recording
  sessions if that's even possible today).
- Live end-to-end check: start music, start dictation, confirm pause;
  manually pause a second, already-Voxi-paused-irrelevant player during
  the session; stop dictation; confirm only the originally-playing
  player(s) resume.
