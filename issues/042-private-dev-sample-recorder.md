# 042: Private Development Sample Recorder (Named Utterance + Corrected Text)

**Status**: Implemented
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Feature
**Related**: [032 small.en vocabulary biasing](032-small-en-project-vocabulary-biasing.md) (Section 7 remaining measurement gate), [038 vocabulary feedback command](038-vocabulary-feedback-command.md) (feedback-command precedent), [040 GBNF grammar-constrained vocabulary](040-whisper-cpp-grammar-constrained-vocabulary.md), [041 sherpa-onnx hotwords canary](041-sherpa-onnx-hotwords-canary.md)

---

## 1. Problem & Motivation

Every accuracy canary run so far (issues 032, 040, 041) has relied on
synthesized eSpeak NG audio because Voxi has no genuine recorded speech
fixtures to test against, and issue 032's Section 7 real-microphone
measurement gate has stayed unresolved for exactly this reason: nothing in
the repo can capture a real utterance, pair it with a known-correct
transcript, and keep it around for reuse. Repo `testdata/` is unsuitable for
this — it is a public tree (only `testdata/speech-context/corpus.tsv` text is
committed; WAVs are gitignored there deliberately as a shared-benchmark
convention). What's needed instead is a small, entirely local, personal
sample set the developer builds up over time on their own machine, reusable
across speech-context, grammar, and future engine-comparison work without
recording a fresh clip for every canary.

## 2. Desired Design

Add a new command that records one real utterance from the microphone, then
prompts for a short name and the manually corrected ground-truth text, and
stores both under `~/.config/voxi/samples/`:

```text
voxi feedback sample record <name>
voxi feedback sample list
voxi feedback sample play <name>
voxi feedback sample remove <name>
```

Flow for `record`:

1. Reuse Voxi's existing audio capture path (the same VAD/utterance
   segmentation `internal/eager` already uses to produce a completed WAV, or
   the simpler start/stop capture behind `internal/record`/`voxi record`,
   whichever is the smaller integration — evaluate both before choosing) to
   capture one utterance to a temporary WAV.
2. Play back or otherwise let the user confirm the recording (at minimum,
   print duration; a `--play` confirm loop is nice-to-have, not required for
   v1).
3. Prompt for the corrected transcript text — what the user actually said,
   typed/pasted exactly, not the raw ASR output. Accept it via stdin prompt
   or an editor invocation (`$EDITOR`), whichever fits Voxi's existing CLI
   conventions best.
4. Persist atomically: `~/.config/voxi/samples/<name>.wav` (mode `0600`) and
   a manifest entry (e.g. a `<name>.txt` sidecar, or a single
   `samples.tsv`/`samples.json` manifest — match whatever pairing scheme
   `testdata/speech-context/corpus.tsv` already establishes for name/text
   pairs, for consistency) recording name, corrected text, and capture
   timestamp.
5. Reject/require confirmation on overwrite of an existing `<name>`.

`list` prints name, corrected text (or a truncated preview), and timestamp.
`play` re-plays the stored WAV through the existing audio-out path if Voxi
already has one, otherwise shells out to a standard local player. `remove`
deletes both the WAV and its manifest entry.

Constraints:

- Local-only: no network calls, no telemetry, no automatic upload anywhere.
- The whole `~/.config/voxi/samples/` directory and its contents must be
  treated as sensitive (private speech + possibly personal text) — private
  file/directory permissions, and explicitly excluded from anything Voxi
  already syncs, backs up, or bundles (verify `history`'s existing "local
  only, treat as sensitive" handling and match it).
- This is a developer/testing convenience, not a user-facing dictation
  feature — no interaction with the eager dictation pipeline's typing
  output, hallucination filtering, or history.
- Sample names use the same sanitizer style as issue 038's vocabulary terms
  (reject path separators, control characters, empty/overlong names).

## 3. Implementation Plan

1. Canary-first: confirm which existing capture path (`internal/eager`'s
   VAD segment producer vs. `internal/record`'s start/stop control) is
   easiest to reuse for a single manually-triggered one-shot recording
   without pulling in the full eager daemon/typing pipeline.
2. Implement the `voxi feedback sample` subcommands with unit tests for
   sanitization, atomic persistence, list/remove behavior, and overwrite
   protection.
3. Wire an opt-in consumer: extend (or document how to point)
   `scripts/speech_context_bench` at `~/.config/voxi/samples/` as an
   additional, private, real-audio corpus source alongside
   `testdata/speech-context/`, so recorded samples directly help close
   issue 032's Section 7 gate and any future engine-comparison canary (040,
   041, and successors) without re-synthesizing audio each time.
4. Document the workflow (record a handful of representative phrases once,
   reuse for every future accuracy canary) in this ticket or a short
   `docs/` note.

## 4. Acceptance Criteria

- [x] `record`, `list`, `play`, `remove` operate correctly on
  `~/.config/voxi/samples/`.
- [x] Recorded WAV and its corrected-text pairing are both required before a
  sample is considered complete; a crashed/interrupted `record` leaves no
  half-written sample.
- [x] Files and directory are created with private permissions (`0600`/`0700`)
  and are excluded from git, backups, and any existing sync tooling.
- [x] Unit tests cover sanitization, persistence, listing, and removal.
- [x] `go test ./...`, `make check`, `make install` pass.
- [x] At least one real recorded sample is captured and used to advance issue
  032's Section 7 measurement gate as a demonstration of the intended use.
  **Done on 2026-09-02** — the user recorded three real microphone samples
  (`hello-voxi-thinkpad`, `hello-voxi-webcam`, `my-toolchain`) via `voxi
  feedback sample record <name>` on their own machine. `go run
  ./scripts/speech_context_bench -corpus ~/.config/voxi/samples` ran
  end-to-end against them with no code changes required — see Section 7
  below and issue 032 Section 7 for the recorded results.

## 5. Non-goals

- No cloud storage, sharing, or export of recorded samples.
- No automatic corpus curation, dedup, or accuracy scoring in this ticket —
  scoring reuses issue 032's existing `scripts/speech_context_bench` runner.
- No change to the eager dictation pipeline's runtime behavior.

## 6. Implemented Behavior

```sh
voxi feedback sample record <name>   # capture + prompt for corrected text
voxi feedback sample list
voxi feedback sample play <name>
voxi feedback sample remove <name>
```

**Capture path chosen**: neither `internal/eager`'s VAD/utterance segmenter
nor `internal/record`'s remote-control-of-an-already-running-daemon fit a
manually-triggered, one-shot recording cleanly — the segmenter pulls in the
full transcription/typing/history pipeline, and `internal/record` only sends
control messages to an already-running `voxtype`/agent process rather than
capturing anything itself. The new `internal/devsample` package instead
mirrors `internal/eager`'s tool-selection logic directly (`pw-record`
preferred, `arecord` fallback, 16kHz mono S16LE) and reuses
`internal/audio.WriteWAVAudio` for the WAV header, but drives capture with an
explicit manual start/stop: it starts the recorder subprocess and stops on
whichever comes first of the user pressing Enter on stdin or the context
being canceled, entirely independent of eager's VAD, transcription worker, or
typing/history side effects.

**Manifest format**: `~/.config/voxi/samples/corpus.tsv`, deliberately
byte-for-byte compatible with `testdata/speech-context/corpus.tsv`'s
`id<TAB>wav file<TAB>expected transcript<TAB>keyterms` shape, so
`scripts/speech_context_bench -corpus ~/.config/voxi/samples` works
unmodified (see `testdata/speech-context/README.md`). Per-sample capture
timestamps ride along as `#ts <name> <RFC3339>` comment lines, which a plain
corpus.tsv reader already skips (any line starting with `#`).

**Sanitization**: `devsample.SanitizeName` rejects path separators outright,
then delegates to `speechcontext.NormalizeTerm` (the same sanitizer issue
038's vocabulary terms use) for control-character stripping and the
empty/overlong checks, collapsing any resulting internal spaces to `-` so the
name is safe to use directly as a filename.

**Atomicity**: the microphone capture stays in memory (never touches disk
half-formed); the WAV is written to a `.tmp` file and only then renamed into
place; the manifest is itself written via temp-file-then-rename. A crash can
at worst leave an orphaned WAV with no manifest entry — it can never leave a
truncated/half-written WAV under a sample's real name.

**Permissions**: `~/.config/voxi/samples/` is created/chmod'd `0700`; the WAV
and manifest are `0600`. The directory lives outside the repo tree, so no
repo `.gitignore` entry is needed (and the repo's existing `*.wav` blanket
ignore already covers any stray WAV that ended up inside the repo by
mistake).

**Update (issue 045, 2026-09-02)**: the corrected-transcript prompt is no
longer a blank line — see issue 045 Section 6 for the full editable-pre-fill
and keyterm-suggestion flow now wired into `Record`. The manifest's 4th
column (`keyterms`) is populated by default for newly recorded samples
instead of always being left blank.

## 7. Live Demo (2026-09-02)

The user recorded three real samples on their own machine with `voxi feedback
sample record <name>`:

```text
$ ls -la ~/.config/voxi/samples/
corpus.tsv
hello-voxi-thinkpad.wav
hello-voxi-webcam.wav
my-toolchain.wav
```

`go run ./scripts/speech_context_bench -corpus ~/.config/voxi/samples` ran
against them unmodified (no bench-runner code changes needed, confirming the
corpus.tsv-compatible manifest format works as designed):

| Fixture | Prompted | Transcript | WER |
|---|---|---|---|
| hello-voxi-thinkpad | no | "Hello Foxy" | 0.50 |
| hello-voxi-thinkpad | yes | "Hello, Voxi." | 0.00 |
| hello-voxi-webcam | no | "Hello, Voxy!" | 0.50 |
| hello-voxi-webcam | yes | "Hello, Voxi!" | 0.00 |
| my-toolchain | no | "My toolchain in the agentic environment uses a harness tool and the human manager." | 0.21 |
| my-toolchain | yes | "My toolchain in the Agenic environment uses a harness tool and the human manager." | 0.29 |

Aggregate: mean WER 0.40 unprompted vs. 0.10 prompted; keyterm recall reads
0/0 for all three because none of these three ad hoc phrases happened to
contain a term from the shipped speech-context vocabulary (`Voxi` alone, as
spoken, isn't in the static term list's exact keyterm set) — this is a real
recorded-corpus result, not a runner bug. This is the first real-microphone
evidence for issue 032's Section 7 gate; see that ticket for the carried-over
finding and next steps (a larger, more deliberately keyterm-dense corpus is
needed before that gate can be considered closed).

No orphaned `.tmp`/`.corpus-*` files were found in the samples directory
after these recordings; the capture → text-prompt → atomic-write flow
completed cleanly end-to-end.

**Command wiring**: `voxi feedback sample {record,list,play,remove}` in
`internal/feedback/command.go`, calling into `internal/devsample`.
`feedback.NewCommand` gained a `deps.Dependencies` parameter (used only by
`sample`) to support real capture/playback and stdin prompting; all other
`feedback` subcommands remain deps-free.

**Not run in this environment**: no microphone/audio device was available
here — `pw-record` is on `PATH` and starts, but produces zero bytes and
`CaptureUtterance` returns a clean "no audio captured" error rather than
hanging, which is unit-tested (`TestCaptureUtteranceNoDeviceAvailable`,
`TestRecordCaptureFailureLeavesNoFiles`). The live end-to-end capture and the
resulting advance of issue 032's Section 7 gate need to be run interactively
by the user on their own machine.

Verified with `go build ./...`, `go vet ./...`, `go test ./...`,
`make check`, and `make install` on 2026-09-02. `internal/eager` and
`internal/record` were read but not modified, so `make restart-service` is
not required for this change.
