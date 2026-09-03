# 051: Warm-Model Prior Art Research (How Other Whisper Dictation Tools Avoid Per-Utterance Reload)

**Status**: Research Complete — 050 Reopened With Narrower Scope (gate items 1-4 outstanding)
**Priority**: P3 (Low)
**Severity**: Informational
**Category**: Research
**Related**: [050 optional warm-model/daemon-mode transcription](050-optional-warm-model-daemon-transcription.md) (closed, no viable mechanism found in the installed `voxtype` binary)

---

## 1. Problem & Motivation

050 closed with a documented negative finding: the installed `voxtype 0.7.5`
CLI has no resident/session mode reachable from `internal/eager`'s
VAD-driven capture loop — `transcribe` is always a one-shot process-per-file
call, and `daemon` does its own mic capture and can't be handed an external
WAV. 050 floated "drive whisper.cpp's server mode" as an alternative but
dismissed it as out of scope, on the assumption it meant vendoring an
unrelated, unverified third-party server binary.

The user asked for a web survey of how other local Whisper-based dictation
tools solve this exact problem — keeping the model resident across
utterances instead of reloading it per call — before deciding whether to
revisit 050. This ticket is that survey. **No code was changed.**

## 2. Findings

### 2.1 whisper.cpp ships its own resident-model HTTP server

`whisper.cpp` (the upstream project `voxtype` itself wraps/vendors) includes
a bundled `examples/server` component (binary commonly called
`whisper-server`), separate from the CLI `main`/`transcribe` binary:

- Loads the model once at startup and keeps it resident for the life of the
  process — the same "preload once, serve many" behavior `voxtype daemon`
  already demonstrates for its own mic-driven flow, but exposed as a
  callable API instead of a self-contained hotkey app.
- Exposes an `/inference` HTTP endpoint that accepts a WAV file via
  multipart upload and returns the transcript in the response — i.e.
  exactly the "transcribe this externally-supplied file using an
  already-loaded model" entry point that 050 found `voxtype transcribe`
  and `voxtype daemon` both lack.
- Concurrency model: httplib-based HTTP layer, but inference itself is
  serialized behind a mutex around the shared `whisper_context` (one
  in-flight inference at a time per process) — the *same* sequential
  constraint `internal/eager`'s `jobChan`/single transcription worker
  already imposes today, so this is not a regression from current behavior.
- Requires 16kHz mono PCM input, matching what `internal/eager` already
  produces (`audio.WriteWAVAudio`, 16kHz), so no new resampling step.

This changes 050's framing: this is not "vendor an unrelated third-party
server," it's "build a second binary from the same upstream `whisper.cpp`
sources that `voxtype` already bundles" (or, if `voxtype`'s own build
doesn't expose it, build `whisper.cpp`'s `examples/server` directly against
the same GGUF/GGML model files already installed for `small.en`). Whether
the currently-installed `voxtype` distribution actually includes/exposes
this server binary is unverified — see Section 4, Gate 1.

### 2.2 Other local dictation tools already use "local VAD + warm remote transcriber" as their standard shape

- `faster-whisper-dictation` (bhargavchippada): captures mic audio, runs VAD
  locally, sends each detected utterance to a separately-running
  `faster-whisper-server` process, and types the returned text — the same
  split `internal/eager` already has (local `audio.NewAudioSegmenter` VAD +
  a transcription call per utterance), just with an HTTP call to a
  long-lived server instead of a process spawn.
- WhisperLiveKit-based setups follow the identical shape: local Silero VAD
  detects utterance boundaries, each utterance ships over WebSocket to a
  persistent server, response is typed at the focused window.
- This confirms `internal/eager`'s existing architecture (local segmenter,
  per-utterance dispatch) is already the right shape for a warm-server
  design — only the dispatch transport (process-spawn vs. HTTP/WebSocket
  call to an already-loaded model) would need to change, not the
  VAD/segmentation pipeline itself. This directly contradicts 050's
  Section 6 conclusion that any warm mode requires "replacing
  `internal/eager`'s own VAD/segmentation-driven audio pipeline" — prior
  art shows the pipeline can stay, only the per-utterance transcribe call
  target changes.
- Closed-source/native tools surveyed (VoiceInk, Buzz, OpenSuperWhisper) are
  documented as using `whisper.cpp` locally but none publish enough
  architectural detail to confirm whether they keep a resident
  server process or also pay a reload cost per session; not a useful data
  point either way.

## 3. Revised Assessment vs. 050's Closure

050's closure reasoning (Section 6, "Decision") assumed the only two paths
were (a) rip out `internal/eager`'s capture pipeline for `voxtype daemon`'s
mic-capture flow, or (b) take on an unrelated new external dependency. This
research surfaces a **third path that existing prior art already validates**:
run a `whisper.cpp`-family HTTP server (same upstream project, same model
files) alongside `internal/eager`'s unchanged VAD/segmentation loop, and
swap only the per-utterance dispatch in `runEagerCaptureSession`
(`internal/eager/eager.go` around line 290) from
`exec.CommandContext(...transcribe...)` to an HTTP POST to that server —
behind the same opt-in flag 050 already specified, with the existing
per-utterance-process path remaining the unconditional default.

This is **not** a recommendation to implement — 050's own canary-first
discipline applies here too, and the concrete unknowns in Section 4 below
are unresolved.

## 3.1 Clarification: this does not mean dropping `voxtype`

Follow-up discussion asked what `voxtype` buys over calling whisper.cpp
directly, since 051's proposal is to add a whisper.cpp server binary.
Answer, for the record so this doesn't get re-litigated later: `voxtype`
is a separate third-party Rust CLI (`~/.local/bin/voxtype`, not part of
this repo), and it does much more than wrap whisper.cpp's inference loop —
multi-engine support (Whisper *and* Parakeet via ONNX/CUDA/MIGraphX,
switchable), GPU backend auto-detection (`voxtype info variants` picked
Vulkan for this machine's AMD GPU), model lifecycle management
(`setup model`/`setup gpu`/`setup onnx`, download flows), its own
daemon/hotkey/record protocol with systemd/Waybar/DMS/Quickshell
integration, a `meeting` mode, and a `--whisper-mode remote` client. None
of that is being replaced. The proposal here is additive: a second, narrow
whisper.cpp-server binary used *only* for the warm-model transcribe path
in `internal/eager`, while `spec/models.yaml`, model resolution, and every
other `voxtype` subcommand `voxi` already relies on stay exactly as they
are today.

## 3.2 Fresh live evidence (2026-09-03, same-day follow-up)

While this ticket was being discussed, live `eager-metrics.json` data from
the running `voxi-agent.service` (captured mid-conversation, not a
constructed benchmark) reproduced the exact short-utterance RTF spike
pattern 050's original Section 1 diagnosed:

| text | audio | transcribe | RTF |
|---|---|---|---|
| "com." | 0.38s | 7.29s | 19.18x |
| "Mwah!" | 1.16s | 5.42s | 4.67x |
| "and all that." | 1.28s | 4.72s | 3.69x |

Normal (non-trivial) utterances in the same window transcribed at
RTF 0.2-0.6x as expected. This is live confirmation the underlying
per-utterance-reload cost 050/051 are scoped around is a real, currently
reproducible cost on this machine, not a one-off from 050's original
session — it strengthens the case for the reopening gate below. The user
also separately noted dictation "typed with big delay" and "something got
worse since yesterday" in the same session; this has **not** been
diagnosed further here (no root cause identified, no code inspected for
this specific complaint) and should be tracked as its own follow-up rather
than assumed to be the same issue as the warm-model reload cost — the
short-utterance spikes above are a pre-existing, previously-documented
pattern (050 Section 1, from before "yesterday"), not necessarily a new
regression.

## 4. Gate — Before Reopening 050's Implementation Plan

1. Confirm whether the installed `voxtype 0.7.5` build actually bundles/
   exposes a `whisper-server`-equivalent binary or build target, or whether
   one would need to be built from `whisper.cpp` source separately
   (unverified — Section 2.1 is upstream-project documentation, not
   confirmed against this machine's actual install).
2. If a separate build is required, confirm it can be pointed at the same
   `small.en` model file `voxtype` already has installed (no duplicate
   model download).
3. Canary the actual latency win: HTTP request/response overhead plus
   model-already-loaded inference vs. today's full per-utterance process
   spawn + model load, on short/rapid-fire utterances specifically (050's
   own motivating case — the "." / "Thanks." spiky-GPU outliers).
4. Confirm clean lifecycle scoping: the server process must start and stop
   with the `voxi eager`/daemon session that opts into it, with no
   lingering GPU-resident process after the session ends (050's own
   constraint, unchanged).

## 5. Recommendation

Reopen 050 as **Proposed / Research**, narrowed to gate item 1 above
(confirm a same-upstream `whisper.cpp` HTTP server binary is actually
obtainable without a new third-party dependency) as the next canary step,
rather than leaving it closed on the prior assumption that no such path
exists. Do not start implementation until that canary and gate items 2-4
are recorded, per 050's own Section 2/3 discipline. **Done (2026-09-03)**:
050's Status/Related header updated to reflect the reopening and
cross-link here — no code changes, gate items 1-4 remain outstanding.

## 6. Non-goals

- No code changes in this ticket — research and prior-art survey only.
- No commitment to implement the warm-server path; it still needs its own
  canary before any production change, per 050's existing acceptance
  criteria.
- No re-litigation of 050's core finding that `voxtype transcribe`/
  `voxtype daemon` themselves have no resident-mode entry point — that
  finding stands; this ticket only identifies a third architectural option
  neither considered.

## 7. Sources

- [ggml-org/whisper.cpp — examples/server/README.md](https://github.com/ggml-org/whisper.cpp/blob/master/examples/server/README.md)
- [ggml-org/whisper.cpp — examples/server/server.cpp](https://github.com/ggml-org/whisper.cpp/blob/master/examples/server/server.cpp)
- [DeepWiki: ggml-org/whisper.cpp — HTTP Server](https://deepwiki.com/ggml-org/whisper.cpp/3.2-http-server)
- [ggml-org/whisper.cpp discussion #182 — Any chance of keeping the model loaded](https://github.com/ggml-org/whisper.cpp/discussions/182)
- [ggml-org/whisper.cpp PR #3257 — server concurrency/task management refactor](https://github.com/ggml-org/whisper.cpp/pull/3257)
- [bhargavchippada/faster-whisper-dictation (GitHub)](https://github.com/bhargavchippada/faster-whisper-dictation)
- [faster-whisper-dictation (PyPI)](https://pypi.org/project/faster-whisper-dictation/)
