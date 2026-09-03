# 050: Optional Warm-Model / Daemon-Mode Transcription (Avoid Per-Utterance GPU Reload)

**Status**: Reopened — Proposed / Research (see 051's narrower gate below)
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Enhancement
**Related**: [046 speech-context default-on](046-speech-context-default-on.md) (ruled out as cause), [032 small.en vocabulary biasing](032-small-en-project-vocabulary-biasing.md), [051 warm-model prior art research](051-warm-model-prior-art-research.md) (reopens this ticket with a narrower gate)

---

## 0. Reopened (2026-09-03) — See 051

This ticket's Section 6 "Decision" (below) closed it on the assumption that
avoiding a per-utterance reload required either replacing
`internal/eager`'s VAD/segmentation pipeline outright, or taking on an
unrelated third-party server dependency. Issue 051's prior-art research
found a third option neither considered: `whisper.cpp` (the upstream
project `voxtype` itself already wraps) ships its own `examples/server`
HTTP server that keeps a model resident and accepts externally-supplied
WAV files over `/inference` — same upstream project, unchanged
`internal/eager` capture pipeline, only the per-utterance dispatch target
in `runEagerCaptureSession` would change. See 051 Section 4 for the gate
that must be cleared (starting with confirming the installed `voxtype`
build actually exposes/bundles that server binary) before any
implementation resumes. Sections 1-6 below are the original investigation
and remain accurate history; they are not rewritten.

---

## 1. Problem & Motivation

The user observed a `btop`-style monitor showing very heavy, spiky GPU
activity during an eager dictation session and suspected the new
speech-context pre-prompt (issue 046) was the cause.

Investigation using the live daemon's own metrics
(`/run/user/<uid>/voxi/eager-metrics.json`, `internal/eager.recordEagerStat`)
ruled that out: the worst real-time-factor outliers in the session were on
the *shortest*, near-silent utterances, not the longer prompted ones —
exactly the opposite of what a prompt-cost explanation would predict:

| text | audio | transcribe | RTF |
|---|---|---|---|
| "." | 1.02s | 19.98s | 19.6x |
| "Thanks." | 0.92s | 10.77s | 11.7x |
| "." | 2.62s | 8.03s | 3.07x |
| "So..." (spoken right after noticing the spike) | 2.04s | 1.47s | 0.72x (normal) |

The actual cause is architectural: `internal/eager/eager.go`'s transcription
worker (`runEagerCaptureSession`, around line 290) spawns a **brand-new
`voxtype` process per detected utterance**
(`exec.CommandContext(context.Background(), voxtypePath, cmdArgs...)`),
with no daemon or keep-alive mode. Every utterance — even a bare "." —
pays the full cost of loading the `small.en` model onto the GPU from
scratch via the Vulkan/RADV backend. If the audio segmenter fires several
short utterances in a burst (e.g. VAD tripping on breath/silence gaps),
those model-reload-plus-inference GPU bursts stack up sequentially in the
job queue (`jobChan`), producing exactly the spiky GPU pattern the user's
monitor showed. This predates issue 046 and is unrelated to the
speech-context prompt.

## 2. Desired Design

Add an **optional** warm-model transcription mode that keeps a `voxtype`
process (or equivalent whisper.cpp session) alive and loaded on the GPU
across utterances within one eager session, instead of spawning a fresh
process per utterance.

Hard constraint from the user: **the current per-utterance-process mode
must remain the default and must not change or regress.** This is
explicitly an opt-in mode, not a replacement — the existing behavior is
working today and must not be put at risk.

- New flag (e.g. `voxi eager --warm-model` or `--daemon-transcribe`),
  default `false`, preserving today's per-utterance `exec.CommandContext`
  spawn as the unconditional default path.
- Canary-first: confirm `voxtype` actually supports a warm/resident mode
  (stdin-driven multi-utterance session, RPC/socket mode, or long-lived
  process accepting repeated transcribe requests) before committing to an
  implementation shape — this may not exist in the installed `voxtype`
  version and could require a different mechanism (e.g. driving
  whisper.cpp's server mode, or batching audio into one process's stdin
  loop). If no such mode exists, document that finding and close or
  re-scope the ticket rather than forcing a workaround.
- Whatever the mechanism, GPU-memory-resident state must be scoped to a
  single `voxi eager`/daemon session and torn down cleanly on exit — no
  lingering GPU-resident process after the session ends.

## 3. Implementation Plan

1. Canary-first: investigate whether `voxtype` (or the whisper.cpp it
   wraps) exposes any resident/session mode, before writing production
   code. Record findings in this ticket.
2. If a viable mechanism exists, implement it behind the new opt-in flag,
   leaving the existing per-utterance spawn path (`runEagerCaptureSession`)
   completely unchanged when the flag is off.
3. Measure GPU load and per-utterance latency (RTF) with the warm mode on
   vs. off, particularly for short/rapid-fire utterances, to confirm it
   actually reduces the spiky GPU pattern observed.
4. Unit/integration tests covering: default (flag off) behavior is
   byte-for-byte unchanged from today; the warm-mode path is exercised
   with a fake/mock transcriber since it likely can't be tested against
   real GPU hardware in CI.

## 4. Acceptance Criteria

- Default `voxi eager` behavior (flag unset) is unchanged — same process-
  per-utterance model, same tests passing unmodified.
- New opt-in flag enables warm-model transcription; documented in
  `--help` and any relevant docs.
- Findings on whether `voxtype` supports a resident mode are recorded here
  even if the answer is "no, needs a different approach" or "not
  currently feasible."
- If implemented, measured GPU/latency comparison (warm vs. per-utterance)
  is recorded in this ticket.
- `go test ./...`, `make check`, `make install` pass. Only run
  `make restart-service` if the default path's live behavior actually
  changes (it must not).

## 5. Non-goals

- No change to the default per-utterance transcription path.
- No change to the speech-context prompting mechanism (issues 032/046) —
  this ticket exists precisely because that mechanism was ruled out as the
  GPU-spike cause.
- No requirement to eliminate all GPU load variance — only to avoid
  redundant full model reloads per utterance when the opt-in mode is used.

## 6. Canary Findings (2026-09-03)

Canary-first investigation (per Section 3, step 1) was run against the
installed `voxtype 0.7.5` (`~/.local/bin/voxtype`, source build,
`gpu-vulkan` feature, AMD Vulkan/RADV backend — same binary/backend the
live daemon uses) **before any production code was written**. No code in
this repo was changed; this section documents the findings and the
decision to close the ticket without implementing the opt-in flag.

### What `voxtype` exposes

- `voxtype transcribe <FILE>` — the exact subcommand
  `internal/eager/eager.go`'s `voxtypeTranscribeArgs` /
  `runEagerCaptureSession` (around line 289) invokes today via
  `exec.CommandContext` — is a standalone one-shot CLI invocation. It takes
  exactly one WAV file argument, prints the transcript to stdout, and
  exits. It has no stdin-driven multi-utterance loop, no batch/multi-file
  mode, and no flag to attach to an already-running process.
- `voxtype daemon` (also the bare `voxtype` / `voxtype --no-hotkey daemon`
  invocation) **does** keep a whisper.cpp model resident on the GPU across
  multiple transcriptions — confirmed via `voxtype info variants` /
  daemon startup log ("Preloading primary model 'small.en'" /
  "Model loaded, ready for voice input", loaded once at daemon start).
  However, it is driven exclusively by `voxtype record start|stop|toggle`
  (which send `SIGUSR1`/`SIGUSR2` to the daemon) and by its own built-in
  hotkey detection. In both cases **the daemon does its own audio capture
  from the microphone** between start and stop; it does not accept an
  externally supplied WAV/PCM buffer or file over any socket/RPC, and
  `record stop`'s transcription result is delivered only via the
  daemon's own output drivers (type/paste/clipboard/`--file`) — not
  returned to the calling process's stdout. There is no
  "transcribe this file using the already-loaded daemon" entry point.
- `voxtype status` confirmed no daemon was running by default in this
  environment (`stopped`) — `voxi-agent.service` does not run `voxtype
  daemon`; it only ever shells out to `voxtype transcribe`.
- `--whisper-mode remote` / `--remote-endpoint` exists, implying `voxtype`
  can act as a thin client to an external persistent whisper.cpp-compatible
  HTTP server (i.e., "drive whisper.cpp's server mode", the alternative the
  ticket floated). No such server binary (e.g. `whisper-server`) is
  installed or bundled with this `voxtype` build (`find` for
  `*whisper-server*`/`*whisper.cpp*` on the system found nothing), so using
  this path would mean voxi newly vendoring, launching, and lifecycle-
  managing a separate third-party long-lived server process purely to give
  the existing `voxtype transcribe` one-shot call somewhere warm to talk
  to — a materially larger, riskier addition than an opt-in flag, and one
  that couldn't be verified on this machine without first installing
  unrelated third-party software. Out of scope for this ticket.

### Empirical confirmation

Timed `voxtype --model small.en --threads 6 -q transcribe <1s silent WAV>`
(same args shape as `voxtypeTranscribeArgs`) under three conditions:

| condition | wall time |
|---|---|
| repeated cold one-shot calls, no daemon running | 2.04s, 1.62s, 1.65s |
| one-shot calls while a real `voxtype --no-hotkey daemon` was running in the background with `small.en` preloaded on the GPU | 1.11s, 1.15s, 1.17s |

The daemon's presence made no qualitative difference — the modest, uniform
drop (~1.6s -> ~1.1s) is consistent with warm page-cache/OS disk-cache
effects on the 487MB model file, not GPU-resident reuse; each
`transcribe` invocation still logs its own full
`whisper_init_from_file_with_params_no_state` / Vulkan device init /
"Loading model 'small.en' into cache" sequence regardless of a running
daemon. This confirms `transcribe` never talks to `daemon` and always pays
its own full model-load cost, matching the ticket's original diagnosis.

### Decision

No mechanism exists that lets voxi keep a `voxtype`/whisper.cpp session
resident across eager utterances **without** either (a) replacing
`internal/eager`'s own VAD/segmentation-driven audio pipeline with
`voxtype daemon`'s self-contained record-from-mic/signal-driven flow — a
ground-up architecture change to the always-on capture loop, not an
opt-in addition, and precisely the kind of change the ticket and task
instructions explicitly forbid risking against the protected default path
— or (b) taking on a new unverified external server-process dependency
outside this ticket's scope. Per the ticket's own Section 2 guidance
("If no such mode exists... document that finding and close or re-scope
the ticket rather than forcing a workaround") and Section 4's acceptance
criterion ("Findings... are recorded here even if the answer is 'no'"),
this ticket is closed with this documented negative finding. No files in
`internal/eager` or elsewhere were modified; `runEagerCaptureSession`'s
per-utterance `exec.CommandContext` spawn (the default and only path)
is untouched. No new Cobra flag was added since there is nothing safe for
it to do differently. `make restart-service` was not required (no change
to code the live daemon runs); `make install` was still run to keep the
installed CLI current with the (unchanged) source tree.

### Verification

- `go build ./...` — pass
- `go vet ./...` — pass
- `go test ./...` — pass (unmodified; no test files touched)
- `make check` — pass
- `make install` — pass (binary reinstalled; no behavioral change, since
  no source was modified)
- `make restart-service` — not run; not applicable, no change to code the
  live `voxi-agent.service` daemon executes.
