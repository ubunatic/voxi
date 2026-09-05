# 056: End-to-End Stress Session Integration Testing with Interleaved Acoustic Noise and CPU/GPU Contention

**Status**: In Progress — Phase 1 Complete and Deterministic; Phase 2 Noise-Rejection Assertion Confirmed Flaky (see §8); CPU/GPU contention deferred
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Performance
**Related**: [052 cpu-gpu priority under load](052-cpu-gpu-priority-under-load.md), [054 short pause acoustic gating](054-short-pause-acoustic-gating-and-context-priming.md), [internal/audio/audio.go](../internal/audio/audio.go), [internal/eager/eager.go](../internal/eager/eager.go)

---

## 1. Problem & Motivation

Unit tests and isolated dev-sample benchmarks evaluate transcription accuracy and acoustic gating on individual, pre-cut WAV utterances. However, in real-world dictation workflows:
1. **Interleaved Transients & Pauses**: Dictation sessions span minutes, during which users pause between thoughts, type on their keyboard (creating click spikes and transients), cough, or bump the desk. Acoustic segmenters must properly reject non-speech spikes, retain real speech segments, and maintain stream alignment without drifting or emitting hallucinations across multiple consecutive utterances.
2. **System Contention & Pipeline Stalling**: Transcription frequently runs while the workstation is under heavy CPU and GPU load (e.g. compiling large projects, browser tabs running WebGL, local LLM inference). Resource starvation can delay reading audio from PipeWire/SoX, stall the ASR worker pipeline, increase latency, and cause frame drops or queue buildup.

We need a repeatable, scriptable integration/stress test harness that concatenates realistic speech samples with interleaved acoustic transients (such as keyboard click smash artifacts), runs continuous eager streaming playback, and optionally exerts synthetic CPU/GPU contention to observe session resilience and transcript integrity.

---

## 2. Proposed Experiment Design

### 2.1 Synthetic Session Composition (Audio Stitching)
Create an integration test harness or script (e.g. `scripts/stress_session_bench.go` or an integration test suite under `internal/eager`):
- Take a corpus of known speech fixtures (`testdata/speech-context/corpus.tsv`, dev samples).
- Splice them into a single continuous multi-minute audio stream with:
  - Variable pauses (e.g. 500ms, 1.2s, 3s).
  - Interleaved non-speech transient clips inserted during pauses (e.g. `artifact-keyboard-smash.wav`, single key clicks, breaths).
- Define the ground truth expected full transcript (the concatenation of accepted speech utterances, asserting that none of the noise transients produce typed tokens).

### 2.2 Feeding into Eager Streaming
Feed the spliced PCM stream directly into `voxi eager` or an in-process mock capture pipeline (simulating stdin/pipe read from audio capture):
- Verify:
  - Speech utterances trigger and transcribe accurately.
  - Interleaved transients trigger `rej:low_energy_transient` or are absorbed as silence without invoking `voxtype`.
  - Consolidated output transcript matches expected text without inserted hallucinated words (e.g. "Thanks for watching!", "Switch Boss.", "andcom.").
  - Chunks in the ring buffer accurately distinguish accepted speech from rejected noise chunks.

### 2.3 Synthetic Resource Contention (CPU & GPU Load Injection)
To simulate real-world workstation load and observe pipeline behavior under stall conditions:
- **CPU Stress**: Spawn background worker threads (e.g. `stress-ng --cpu N` or in-Go compute loops) to saturate CPU cores.
- **GPU Stress**: Run a lightweight GPU kernel or compute workload (e.g. compute shader, Vulkan/OpenCL matrix multiplication, or concurrent `voxtype`/llama-cli instance) competing for VRAM and GPU compute queues.
- **Metrics to Measure**:
  - Utterance latency and Real-Time Factor (RTF) distribution under load vs. idle.
  - Whether `voxtype` times out or stalls worker queue channel buffers.
  - Whether audio frames are dropped by the audio reader when the worker is busy.

---

## 3. Implementation & Experiment Plan

1. **Audio Splicer / Session Generator**:
   - Write a helper utility that reads WAV files from `testdata/` and generates a single concatenated test stream with configurable pauses and noise events.
2. **Integration Test Suite**:
   - Add a test (e.g. behind a build tag or `-test.run TestStressSession -test.short=false`) that runs the session through `AudioSegmenter` and worker channels.
3. **Contention Harness**:
   - Provide command-line flags or a script to enable synthetic CPU/GPU load during the session run.
4. **Validation & Assertions**:
   - Compare final concatenated transcript against expected text via Word Error Rate (WER) or exact sequence match.
   - Assert zero accepted noise artifacts in the session manifest.

---

## 7. Phase 1 & 2 Implementation (2026-09-05)

**What was built**: per the user's explicit "start simple, then extend"
instruction, this pass built a real (not mocked, not reimplemented) replay
harness rather than attempting the full stress-session design in one shot.

New file: `internal/eager/e2e_pipeline_test.go` (package `eager`, gated by
`VOXI_E2E=1` so it never runs as part of `go test ./...`'s fast path — see
"Verification" below). No production code (`internal/eager/eager.go`) was
changed; the harness drives the existing, unmodified
`runEagerCaptureSession` directly.

**Phase 1 — single-fixture real replay**
(`TestEagerCaptureSessionEndToEndSingleFixture`): substitutes
`runEagerCaptureSession`'s normal `pw-record`/`arecord` audio-source
subprocess for one that streams a corpus WAV's raw PCM instead (`sox`
preferred per the original design, `ffmpeg` as a canary-verified fallback —
this dev machine has `ffmpeg` but not `sox`; `resolvePCMStreamCmd` probes via
`d.LookPath` before building either command line, per `docs/Canary.md`).
This drives the real `audio.AudioSegmenter`, the real chunk-dispatch worker,
and a real `voxtype transcribe` subprocess call against the real installed
`small.en` model — nothing about VAD, chunk dispatch, or transcription is
reimplemented or mocked.

For the typing side: `deps.Dependencies.RunStdin` is swapped for a capture
function while `LookPath` is left as the real resolver, so
`internal/typing.TypeText` still resolves the real `dotoolc`/`dotool`
binaries and takes its normal, non-degraded code path — it just never
executes the rendered dotool script, so no keystrokes reach the real
focused window. `HOME`/`XDG_RUNTIME_DIR` are also redirected to a scratch
dir via `deps.Dependencies.Getenv` so the session's chunk ring buffer,
feedback overrides, speech vocabulary, and dictation history all land in a
throwaway location instead of the real user's (two production helpers,
`writeVoxtypeState` and `recordEagerStat`, read `XDG_RUNTIME_DIR` via
`os.Getenv` directly rather than through `deps.Dependencies`, so the test
still writes ephemeral, self-resetting state files under the real runtime
dir — harmless, since the session's own deferred `writeVoxtypeState("idle")`
resets it, but worth knowing if `voxi monitor` flickers "recording" during a
test run).

WER is computed with a small package-local Levenshtein word-edit-distance
helper (`wordErrorRate`) matching `scripts/speech_context_bench`'s helper of
the same name and algorithm — that helper is unexported in `package main` so
could not be imported directly; duplicating the same well-tested approach
was judged simpler and lower-risk than exporting it or adding a new shared
package for one function.

**Phase 2 — spliced multi-utterance session with real noise interleaving**
(`TestEagerCaptureSessionEndToEndSplicedNoiseSession`): a minimal audio
splicer (`buildSpliceStream` + `readWAVDataChunk`, both new and
package-local to the test) concatenates two speech fixtures' raw PCM with
synthetic silence gaps (500ms/1500ms/1200ms/500ms) and, in between them, the
corpus's real `artifact-keyboard-smash` fixture — a genuine recorded
keyboard-click noise transient, not synthetic all-zero silence — into one
spliced WAV, then feeds that through the same real-pipeline path as Phase 1.
Assertions use the session's own chunk ring buffer
(`internal/chunks.Buffer.List`) as the source of truth: at least one chunk
must be rejected (exercising the real hallucination/silence-artifact
rejection path on real noise audio), and each speech fixture's expected text
must best-match (WER ≤ 0.4) some accepted chunk's `CleanedTranscript`. An
earlier version of this assertion incorrectly WER-compared each fixture's
short expected text against the *whole* concatenated transcript, which
spuriously inflated WER past 1.0 by counting the other fixture's words as
insertions — fixed by matching against individual accepted chunks from the
ring buffer instead.

**Real verification performed** (this session, live on this machine, not
just "compiles"): the committed public `testdata/speech-context/corpus.tsv`
fixtures' WAV files are private and gitignored and were not present on this
machine except `artifact-keyboard-smash.wav`, so both tests were run against
the equivalent private dev-sample corpus at `~/.config/voxi/samples`
(recorded via `voxi feedback sample record`, see
`testdata/speech-context/README.md` "Using recorded dev samples") by
pointing `VOXI_E2E_CORPUS` at it — the harness itself defaults to the public
corpus and needs no code change to point elsewhere, matching the existing
`speech_context_bench` convention:

```
VOXI_E2E=1 VOXI_E2E_CORPUS=~/.config/voxi/samples \
  VOXI_E2E_FIXTURE=kt-core \
  VOXI_E2E_FIXTURE_A=kt-core VOXI_E2E_FIXTURE_B=kt-daemon \
  VOXI_E2E_NOISE_FIXTURE=artifact-keyboard-smash \
  go test ./internal/eager/ -run TestEagerCaptureSessionEndToEnd -v -timeout 180s
```

Both tests passed against the real installed `voxtype`/`small.en` model:

- Phase 1: expected `"Voxi uses voxtype with dotool on PipeWire and
  Wayland."`, got `". Voxi uses VoxType with dotool on PipeFire and
  Wayland."`, WER 0.111.
- Phase 2: the spliced session produced 5 chunks; chunk #3 (the interleaved
  keyboard-smash noise) was rejected with reason `stop_word`; chunk #2 best-
  matched fixture A at WER 0.111 and chunk #5 best-matched fixture B
  (`"The systemd user service restarts the voxi-agent daemon."`) at WER
  0.000 (exact).

**Observed but out of scope for this pass**: both runs also produced a tiny
accepted chunk containing only `"."` (chunks #1 and #4 in the Phase 2 log)
immediately before each real sentence — likely the segmenter capturing a
short pre-roll/lead-in fragment of the source recording as its own
utterance, transcribed by Whisper as a bare period. This is pre-existing
pipeline behavior surfaced by the harness, not introduced by it, and is
low-impact (a single typed `.` character); not investigated further here
per the constraint against touching `internal/eager/eager.go` beyond what's
strictly needed to make it testable (nothing was needed here — the harness
only added test code).

**Deferred to a later pass** (explicitly, per the user's incremental
instruction — not attempted this session):
- CPU/GPU synthetic contention injection (`stress-ng`, a Vulkan/OpenCL
  compute workload, or a concurrent `voxtype`/llama-cli instance) and RTF/
  latency-under-load measurement (Section 2.3). Sketch for a future pass:
  the harness's existing `context.WithTimeout` wrapper around
  `runEagerCaptureSession` is already the natural place to start a
  contention workload just before the call and stop it in a `defer` after;
  the interesting new measurement would be per-chunk `TranscribeDurationSec`
  and `RTF` already recorded in `chunks.Chunk` (visible via
  `chunkBuf.List()`, exactly as this pass's noise-rejection assertion reads
  it) compared idle-vs-loaded, rather than inventing a new metrics path.
- Full-corpus WER validation across every fixture (this pass exercises one
  or two fixtures per test, chosen by env var, not the whole corpus).
- A configurable CLI/flag surface (the ticket's `-cpu`/`-gpu` flags); this
  pass is a Go test harness, not yet a script with flags.
- Variable-pause sweep (500ms/1.2s/3s as separate cases) — this pass uses
  one fixed set of gaps for the splice.

**Verification**: `go build ./...`, `go vet ./...`, `go test ./...` (fast
path, unaffected — both new tests `SKIP` cleanly without `VOXI_E2E=1`),
`make check`, and `gofmt -l` all pass. `make restart-service` was **not**
run: no code the live daemon executes (`internal/eager/eager.go`,
`internal/agent`, etc.) was modified, only a new gated test file was added.

## 8. Independent Re-Verification (2026-09-05) — Real Finding, Not Just a Rubber Stamp

Per this repo's review discipline, the host orchestrator re-ran both new
`VOXI_E2E=1` tests independently against the same private
`~/.config/voxi/samples` corpus rather than trusting the single-run "PASS"
reported above. Results diverged in a real, useful way.

**Phase 1 (`TestEagerCaptureSessionEndToEndSingleFixture`, fixture
`kt-core`)**: reproducibly (3/3 runs) got a *different*, worse transcript
than the one reported above: `"If you enjoyed this video, Voxi uses VoxType
with dotool on PipeFire and Wayland."` (WER 0.667, failing the 0.4
threshold) instead of `". Voxi uses VoxType with dotool on PipeFire and
Wayland."` (WER 0.111, passing). "If you enjoyed this video" is a genuine,
previously-uncaught Whisper YouTube-outro hallucination prefix — a new
variant in the same family `spec/models.yaml`'s existing `in-video`/
`in-this-video`/`in-todays-video` entries already cover, just not this exact
phrasing. Added `{ id: if-you-enjoyed-this-video, pattern: "if you enjoyed
this video" }` to `spec/models.yaml`'s `common_stop_words` (same style/risk
level as the existing entries — a content-list addition, not a design
change). Re-ran Phase 1 three more times after the fix: all three
deterministically produced the WER-0.111 transcript with the hallucination
correctly stripped. **This was the E2E harness catching a real, previously
unknown hallucination-coverage gap on the first real independent run against
this exact recording — exactly what issue 056 exists to surface.**
`go test ./...` re-confirmed green after the `spec/models.yaml` change.

**Phase 2 (`TestEagerCaptureSessionEndToEndSplicedNoiseSession`)**: re-run
independently (after the Phase 1 fix above) **failed**: the interleaved
`artifact-keyboard-smash` noise segment transcribed as a bare `"."` in this
run and was accepted (not rejected), rather than matching a known stop-word
and being rejected as reported in §7. The two speech fixtures still
correctly best-matched accepted chunks (WER 0.111 and 0.000, same as §7).
Root cause: this pipeline has **no built-in stop-word for a bare `"."`**
transcript — issue 034's silence-artifact mechanism *can* filter an exact
whole-utterance match like `"."`, but only once a user has explicitly opted
in via `voxi feedback silence-artifact add "."` (034's own design explicitly
keeps this empty by default, on the safety principle "with no configured
artifacts, existing transcript behaviour must be byte-for-byte unchanged").
The test harness runs against a fresh, empty scratch config, so this default
applies. Separately, §7 already flagged (as "out of scope") that this same
corpus/pipeline combination produces stray bare-`"."` chunks around real
sentences even outside the noise-rejection scenario — this is the same
underlying gap, now confirmed to also make the *noise-rejection assertion
itself* flaky, since whether the noise segment's transcription happens to
land on a known stop-word (rejected) or a bare `"."` (accepted, no
stop-word for it) varies run to run for reasons not yet isolated (whisper.cpp
decode variance on this low-confidence audio, not confirmed as GPU
nondeterminism specifically).

**This was not fixed in this pass.** Unlike the Phase 1 gap, this is a
design question, not a drop-in content-list fix: making bare `"."` (or very
short low-confidence transcripts generally) a *built-in* default stop-word
would change 034's deliberate "byte-for-byte unchanged with no configured
artifacts" default behavior for every user, not just this test's scratch
config, and deserves its own consideration rather than being decided as a
side effect of stabilizing a test. Recorded here as a known, currently
flaky assertion in `TestEagerCaptureSessionEndToEndSplicedNoiseSession`, not
as a silently-accepted false pass. A future pass should either (a) make the
test's own scratch-config setup explicitly opt in a `"."` silence-artifact
so the assertion is deterministic without touching any user's real default,
or (b) open a separate ticket asking whether bare-period/very-short
low-confidence outputs should get a built-in default filter — this ticket
does not decide that question.

**Revised status**: Phase 1's fix is verified deterministic (3/3 clean
reruns) and safe (an additive, same-style stop-word entry). Phase 2's
harness code itself is sound (splicer, ring-buffer inspection, WER matching
all worked correctly) but its noise-rejection assertion is confirmed flaky
against real hardware for the reason above — treat Phase 2 as "harness
built, assertion not yet stable" rather than "complete," until one of the
two options above is implemented.
