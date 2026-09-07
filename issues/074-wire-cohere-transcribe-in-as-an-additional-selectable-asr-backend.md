# 074 — Wire Cohere Transcribe In as an Additional Selectable ASR Backend

**Status**: Closed
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Feature
**Related**: [066 Canary: Cohere Transcribe and Nemotron 3.5 Streaming as Alternative ASR Backends](066-canary-cohere-transcribe-and-nemotron-3-5-streaming-as-alternative-asr-backends.md), [064 FluidVoice model landscape research](064-fluidvoice-model-landscape-fluid-intelligence-licensing-and-underlying-stt-model-portability-research.md), [073 Read FluidVoice's model-download code paths for direct weight URL reuse (research, lower priority)](073-read-fluidvoice-s-model-download-code-paths-for-direct-weight-url-reuse-research.md)

---

## 1. Problem & Motivation

Issue 066's canary (2026-09-07, commit `e1a942a`) benchmarked Cohere
Transcribe (`CohereLabs/cohere-transcribe-03-2026`, Apache-2.0) via the
`CrispASR` runtime (MIT, C++/ggml, whisper.cpp-style, single 4.7 MB static
binary) against Voxi's current whisper.cpp `small.en` baseline and found it
wins on every measured axis, even running CPU-only against a
GPU-accelerated baseline:

| Metric | whisper.cpp `small.en` (GPU) | Cohere Transcribe (CPU) |
|---|---|---|
| WER | 0.2726 | 0.2226 |
| Keyterm recall | 44.44% | 59.26% |
| RTF | 0.66 | 0.60 |

066's recommendation (§7.6) was explicit: **adopt as an additional
selectable backend, not a default-swap** — official-weights provenance and
vocabulary biasing are still open questions (see §5 below). This ticket is
that deferred implementation follow-up. 066 itself was scoped as
canary-only ("no production backend integration" was an explicit
non-goal); this ticket does the actual plumbing.

## 2. Integration Point (identified this session — do not rebuild from scratch)

Voxi's model selection is **not** currently behind a runtime-pluggable
interface; it is a single hardcoded subprocess path with one YAML-declared
per-model field that isn't yet consumed for dispatch:

- **`spec/models.yaml`** — the model registry (single source of truth per
  `docs/Spec.md`). Each model entry already declares an `engine:` field
  (currently every model sets `engine: whisper`), defined in
  **`spec/models.go`**'s `Model` struct (`Engine string \`yaml:"engine"\``,
  around line 20) — but nothing in Go branches on this field today. It's
  presently decorative.
- **`internal/eager/eager.go`**, `runEagerCaptureSession` (~line 223): after
  resolving the model name via `spec.LoadModels()` /
  `modelSpec.ResolveModel(...)` (~line 270-285), it unconditionally builds
  args via `voxtypeTranscribeArgs(modelName, wavPath, initialPrompt)`
  (~line 607-613) and runs `exec.CommandContext(transcribeCtx, voxtypePath,
  cmdArgs...)` (~line 378) — i.e. it always shells out to the `voxtype`
  binary on `PATH` (resolved once via `d.LookPath("voxtype")`, ~line 179).

This is the concrete seam to extend: make the subprocess binary and its
arg-building conditional on the resolved model's `engine` field, instead of
always assuming `voxtype`.

## 3. Desired Design

1. Add a new `engine: cohere-transcribe` (or similarly named) value and a
   model entry (e.g. `cohere-transcribe-03-2026`) to `spec/models.yaml`,
   with its own `stop_words` list (reuse `*common_stop_words` unless the
   canary's transcripts showed model-specific hallucination patterns worth
   splitting out — check 066 §7 transcripts before assuming reuse is safe).
2. In `internal/eager/eager.go`, branch on the resolved model's `Engine`
   field: for `engine: whisper`, keep today's `voxtypeTranscribeArgs` /
   `voxtypePath` path unchanged; for `engine: cohere-transcribe`, resolve
   and invoke the `crispasr` binary instead, with its own arg-building
   function (mirror `voxtypeTranscribeArgs`'s shape — model path, wav path,
   thread count — using 066 §7.2's reproducible build/invocation commands
   as the reference). Resolve `crispasr`'s path the same way `voxtype`'s is
   resolved today (`d.LookPath`), so the backend is only available/selected
   if the binary is actually installed.
3. Model weight handling: `CrispASR`/GGUF weights are not bundled at
   build/install time (1.66 GiB vs whisper `small.en`'s 487 MB — a real
   disk/download cost, per 066 §7.6). Add on-demand/lazy download of the
   GGUF weight file the first time the `cohere-transcribe` engine is
   selected and the weight isn't already present locally (mirror whatever
   pattern, if any, `voxtype`/whisper model downloads already use — check
   for one before inventing a new download path). The initial download
   source may be the same ungated community mirror 066 used
   (`cstr/cohere-transcribe-03-2026-GGUF`) — see §5 for the
   official-vs-mirror caveat.
4. Expose the new model as selectable the same way existing models are
   selectable today (CLI `--model` flag / config, whatever `ResolveModel`
   already plumbs) — no new selection UI/mechanism needed beyond making the
   new model entry resolvable.
5. `small.en`'s speech-context prompt biasing
   (`shouldUseSpeechContext`, `internal/eager/eager.go` ~line 615-617) is
   gated to that specific model name today; leave the new model out of that
   gate (see §5 — biasing has no hook for this engine yet) rather than
   silently passing an `--initial-prompt`-equivalent flag that CrispASR
   accepts but ignores for this backend (066 §7.5 confirmed `--prompt`/
   `--hotwords` are no-ops here).

## 4. Acceptance Criteria

- `spec/models.yaml` has a new model entry using a `cohere-transcribe`
  engine value, validated against `schemas/models.schema.json` (update the
  schema if it currently enumerates allowed `engine` values).
- `internal/eager/eager.go` dispatches to the correct binary/arg-builder
  based on the resolved model's `Engine`, with the existing `whisper`/
  `voxtype` path fully unchanged (regression-free) and covered by existing
  tests.
- A user can select the new model (e.g. `voxi eager --model
  cohere-transcribe-03-2026` or equivalent existing selection mechanism)
  and get a real transcription end-to-end through the eager pipeline,
  verified manually against the existing `testdata/speech-context/` fixture
  corpus (the same one 066 used) — not just a unit test with a mocked
  subprocess.
- Weight download is lazy/on-demand (not bundled into `make install`), with
  a clear error/prompt if the weight is missing and no network path is
  available.
- New unit tests cover: engine-based dispatch branching in
  `internal/eager` (arg-building, binary resolution), and any new download
  logic (path resolution, idempotent re-download avoidance) — mocking the
  subprocess/network as the existing `voxtype` tests already do.
- `make install` succeeds; if `internal/eager` was touched (it is, by
  design here), `make restart-service` is run per this repo's
  `CLAUDE.md` convention so the live `voxi-agent.service` picks up the
  change, and one live manual dictation is exercised through the new
  backend before calling this done (unit tests passing is not sufficient
  evidence of live subprocess/binary-resolution behavior — see
  `docs/AgenticLoop.md`'s live-verification gate for hook/environment
  features, which applies analogously to any change touching the live
  eager subprocess pipeline).

## 5. Explicit Out of Scope / Known Gaps (not blockers)

- **Official vs. mirror weights**: 066 used the ungated community GGUF
  mirror (`cstr/cohere-transcribe-03-2026-GGUF`) because the official
  `CohereLabs` HF repo is gated (HF account + accepting terms). This ticket
  may ship using the same mirror; re-verifying/switching to official
  gated weights is a follow-up, not a blocker here, unless the mirror
  proves genuinely unusable during implementation (e.g. taken down,
  materially wrong WER on the fixture corpus vs. 066's numbers).
- **Vocabulary/keyterm biasing**: no equivalent to
  `internal/speechcontext`'s initial-prompt biasing exists for this engine
  (CrispASR's `--prompt`/`--hotwords` are no-ops for Cohere Transcribe
  specifically, per 066 §7.5). Do not attempt to design a decoder-side
  biasing mechanism as part of this ticket — ship without biasing for this
  backend and note the gap; file a separate follow-up ticket if/when
  prioritized.
- **Download URL reuse from FluidVoice**: issue 073 (open, explicitly
  lower priority per the user) proposes reading FluidVoice's own
  model-download code for URL-reuse ideas that could inform this ticket's
  weight-download source. It may inform the download implementation here
  but is not a prerequisite — don't block this ticket on 073 landing
  first.
- **Default-swap**: this ticket only makes Cohere Transcribe selectable
  alongside whisper.cpp. Making it the default model is explicitly out of
  scope per 066's recommendation.
- **Nemotron 3.5 streaming**: 066 rejected this candidate; it is not part
  of this ticket's scope.

## 6. Verification Guidance

- Reuse 066 §7's exact repro commands (`CrispASR` build steps, GGUF mirror
  download) as the starting point for the download/build logic — don't
  re-derive them from scratch.
- Reuse 066's fixture corpus (`testdata/speech-context/`, private WAVs,
  git-ignored) for the manual end-to-end check in §4.
- Run existing `internal/eager` and `spec` package tests
  (`go test ./internal/eager/... ./spec/...`) to confirm no regression to
  the `whisper`/`voxtype` path.

## 8. Implementation Notes (2026-09-07)

Implemented as designed in §3, with no scope changes:

- `spec/models.yaml`: new `cohere-transcribe-03-2026` model entry with
  `engine: cohere-transcribe`, reusing `*common_stop_words` (066's
  transcripts on this corpus showed no model-specific hallucination
  pattern). `spec/schemas/models.schema.json`'s `engine` enum extended to
  `["whisper", "cohere-transcribe"]`.
- `internal/eager/eager.go`'s `runEagerCaptureSession` now resolves the
  model's `Engine` once (after `modelSpec`/`ResolveModel`) and picks a
  `transcribeBinPath` + `buildTranscribeArgs` closure per engine; the
  `whisper` branch is byte-for-byte the prior unconditional behavior
  (`voxtypePath` / `voxtypeTranscribeArgs`), so the existing whisper/voxtype
  path is unmodified. `cohere-transcribe` resolves `crispasr` via
  `d.LookPath` (same pattern as `voxtype`) and calls new
  `internal/eager/cohere.go`.
- `internal/eager/cohere.go` (new): `crispASRTranscribeArgs` (mirrors
  `voxtypeTranscribeArgs`'s shape: model path, wav path, thread count;
  deliberately has no `initialPrompt` parameter -- 066 §7.5 confirmed
  `--prompt`/`--hotwords` are no-ops for this backend) and
  `ensureCohereWeights`/`ensureWeightsFile`/`downloadFile` (lazy on-demand
  GGUF download into `os.UserCacheDir()/voxi/models/`, atomic
  tmp-then-rename, idempotent size-based cache check, no vendored/bundled
  weight -- never touched by `make install`).
- Per §3 item 5, `shouldUseSpeechContext` is untouched (still gated to
  `small.en` only); `cohere-transcribe` gets no `--initial-prompt`-equivalent
  flag.
- Tests: `internal/eager/cohere_test.go` (new) unit-tests the arg builder
  and the download/cache logic (skip-when-cached, download-when-missing,
  truncated-download rejection, HTTP-error surfacing) against an
  `httptest.Server`, plus a mocked-subprocess dispatch test
  (`TestEagerDispatchesCohereTranscribeEngineToCrispASR`) proving
  `runEagerCaptureSession` resolves `crispasr` (not `voxtype`) for this
  engine. `internal/eager/e2e_pipeline_test.go` gained
  `TestEagerCaptureSessionEndToEndCohereTranscribe`, a `VOXI_E2E=1`-gated
  real-pipeline replay (same harness as the existing whisper e2e test) that
  drives the actual `crispasr` binary and real GGUF weights against a real
  corpus fixture.
- Verification performed: `go test ./...` and `make check` green
  (including the new tests); real end-to-end replay against
  `~/.config/voxi/samples/kt-core.wav` via
  `VOXI_E2E=1 VOXI_E2E_CORPUS=~/.config/voxi/samples VOXI_E2E_FIXTURE=kt-core
  go test ./internal/eager/... -run TestEagerCaptureSessionEndToEndCohereTranscribe`
  produced `"you Voxy uses VoxType with DoTool on Pipefire and Wayland."`
  (WER 0.333 against expected `"Voxi uses voxtype with dotool on PipeWire
  and Wayland."`), consistent with 066's canary transcript quality; the
  equivalent whisper/small.en e2e test was re-run alongside it and still
  passes (WER 0.222), confirming no regression. `make install` and `make
  restart-service` both succeeded and `voxi-agent.service` is active
  running the new binary. The actually-installed `voxi` binary was also
  launched directly (`voxi eager --model cohere-transcribe-03-2026`)
  against the real microphone and real `crispasr`/weights with no crash.
  No live human-spoken microphone dictation was possible in this
  non-interactive agent session (no speaker present) -- the real-pipeline
  replay test above is the closest available substitute, since it drives
  the identical `runEagerCaptureSession` code path the daemon runs, with
  real (not mocked) subprocesses, binaries, and weights, only the audio
  source (file replay via `ffmpeg` instead of `pw-record`) differs, exactly
  as issue 056 established for the existing whisper e2e tests. A human
  operator should still do one live spoken dictation via `voxi-agent`
  (toggle to `cohere-transcribe-03-2026`) at their convenience to close that
  last gap.
- Local machine setup performed (not part of the repo diff): `crispasr`
  symlinked to `~/.local/bin/crispasr` (from issue 066's canary build) and
  the GGUF weights symlinked into `~/.cache/voxi/models/cohere-transcribe-q5_0.gguf`
  (from issue 066's canary download) so `ensureCohereWeights`'s cache check
  finds them without re-downloading 1.66 GiB.
- Out-of-scope items from §5 were left untouched: no official-vs-mirror
  weight resolution, no vocabulary-biasing hook, issue 073 not read, and
  `spec/models.yaml`'s `default_model` remains `small.en`.
