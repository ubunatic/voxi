# 041: sherpa-onnx Hotwords Canary (Contingent Engine Alternative)

**Status**: Canary Complete — Recommend Reject
**Priority**: P3 (Low)
**Severity**: Informational
**Category**: Research / Feature
**Related**: [032 small.en vocabulary biasing](032-small-en-project-vocabulary-biasing.md), [040 GBNF grammar-constrained vocabulary](040-whisper-cpp-grammar-constrained-vocabulary.md), [039 OSS STT landscape research](039-oss-stt-landscape-and-custom-vocabulary-research.md)

---

## 1. Problem & Motivation

Issue 039's research identified `sherpa-onnx` (Apache-2.0, open weights,
portable C++/ONNX runtime, Go bindings available) as the best-justified next
canary *if* whisper.cpp's own biasing mechanisms (issue 032's initial-prompt,
issue 040's GBNF grammar) turn out insufficient for exact recognition of
Voxi's technical vocabulary. `sherpa-onnx` exposes a documented, first-class
`--hotwords-file` / `modified_beam_search` decoding API — a stronger and more
purpose-built biasing mechanism than either whisper.cpp approach, but it is a
different inference engine and runtime entirely, so it is a materially larger
change than issues 032/040.

## 2. Explicit Gate — Do Not Start Implementation Until

- Issue 032's remaining measurement gate (Section 7: three local WAV fixtures,
  `scripts/speech_context_bench`) is recorded, **and**
- Issue 040's three-way benchmark (unprompted / prompt-only / grammar) is
  recorded, **and**
- Those results show *either* prompt-only or grammar-constrained decoding
  still misses a material fraction of exact technical terms Voxi cares about
  (i.e., neither in-engine technique is "good enough").

If issue 040's grammar mode reaches acceptable exact-term recall with no
material regression, this ticket should be closed as unnecessary rather than
implemented — do not swap engines for its own sake.

## 3. Desired Design (If Gate Is Met)

A throwaway, isolated canary only — not production plumbing in this ticket:

1. Build/install `sherpa-onnx` locally (CPU build) with a comparable
   English streaming model (e.g. a Zipformer/Conformer-transducer checkpoint
   sherpa-onnx documents), independent of Voxi's existing whisper.cpp
   pipeline.
2. Reuse issue 032's fixture corpus (`testdata/speech-context/`, private WAVs
   git-ignored) to run the same paired comparison: baseline vs. hotwords-file
   biasing using the same vocabulary terms as issues 032/040.
3. Record WER, exact keyterm recall, latency, RTF, and model/binary size
   against the whisper.cpp `small.en` baseline and against issue 040's
   grammar-constrained results, on the same hardware issue 032 canaried on.
4. Explicitly evaluate operational cost: new runtime dependency, Go bindings
   maturity, model download/licensing for the chosen checkpoint, and CPU
   resource use versus the already-installed `voxtype`/whisper.cpp binary.

## 4. Acceptance Criteria (for the canary itself, not adoption)

- A working, reproducible sherpa-onnx canary command and its exact flags are
  recorded in this ticket.
- The three-way (or four-way, including whisper.cpp baseline) comparison
  table is recorded with hardware and model details, matching the format
  issues 032/040 already use.
- A clear recommendation is written: adopt (with a new implementation ticket
  scoping the `internal/eager` integration point), or reject with reasons.
- No production code changes land in this ticket regardless of outcome — a
  separate implementation ticket is required if adoption is recommended.

## 5. Non-goals

- No replacement of `small.en`/whisper.cpp as Voxi's default engine without
  an explicit, separately-approved implementation ticket.
- No cloud dependency — sherpa-onnx canary must run fully local/offline.
- No abandoning issues 032/040's simpler in-engine techniques before they are
  actually measured and found insufficient.

## 6. Canary Results (2026-09-02) — Recommend Reject

### 6.0 Gate status note (why this ran despite Section 2's literal wording)

Section 2 literally requires both issue 032's Section 7 real-microphone
measurement gate and issue 040's three-way benchmark before starting. Neither
condition is met as originally written:

- **Issue 040 is structurally blocked, not benchmarked.** Its own
  Section 6 (recorded 2026-09-02) found that the installed `voxtype 0.7.5`
  exposes **no grammar-constrained-decoding mechanism at all** — no
  `--grammar-file` flag, no `gbnf` string anywhere in the binary, no
  `grammar` key in the config dump, and no forwarding path even through the
  `whisper-cli` subprocess backend (which isn't installed, and whose flag
  allowlist in `src/transcribe/cli.rs` doesn't include grammar anyway). This
  forecloses whisper.cpp's own hard-constraint option entirely — it isn't a
  case of "grammar exists but is insufficient," there is no grammar
  mechanism to measure. No three-way benchmark could be run because the
  third leg (grammar mode) does not exist in this install.
- **Issue 032's Section 7 real-microphone gate (private WAV fixtures,
  `scripts/speech_context_bench`) is still separately unresolved** and was
  not run as part of this ticket. It requires genuine recorded audio with a
  representative speaker/microphone, which this canary does not attempt to
  substitute for.

Because the only in-engine hard-constraint alternative (040) is structurally
unavailable rather than merely weak, the spirit of Section 2's gate — "only
reach for sherpa-onnx once whisper.cpp's own mechanisms are known
insufficient" — is satisfied even without a literal benchmark: there is
nothing further to wait on before running this canary. 032's Section 7 gate
remains open independently and is unaffected by this ticket.

### 6.1 Installation

`sherpa-onnx` was not previously installed (not on `PATH`, no Python module,
no Go module reference in `go.mod`). Installed via pip, which was fast and
required no source build:

```sh
pip3 install --user sherpa-onnx
```

Result: `sherpa-onnx-1.13.7` + `sherpa-onnx-core-1.13.7`, prebuilt
`manylinux2014_x86_64` wheels for `cp314` (matching the system's
Python 3.14.7) — no compilation needed, installed in seconds.

### 6.2 Model

Used sherpa-onnx's own documented streaming Zipformer2 English model,
`sherpa-onnx-streaming-zipformer-en-2023-06-26` (LibriSpeech + GigaSpeech
trained, chunk 16/left-context 128), downloaded from the project's GitHub
release assets:

```sh
curl -sL -o model.tar.bz2 \
  "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-streaming-zipformer-en-2023-06-26.tar.bz2"
tar xjf model.tar.bz2
```

Download size 310 MB (not multi-GB). This model was chosen over the
smaller `sherpa-onnx-streaming-zipformer-en-20M-2023-02-17` (128 MB, ~20M
params) because only this one ships the `bpe.model` SentencePiece file
needed to encode hotword phrases into the model's BPE vocabulary — the 20M
model's release has no BPE artifact, so its hotwords could not be encoded at
all. Used the int8-quantized encoder/decoder/joiner
(`encoder-epoch-99-avg-1-chunk-16-left-128.int8.onnx`, 67 MB) for the
primary runs; a fp32 sanity re-run on one fixture (see 6.4) produced an
identical transcript, confirming quantization was not the source of the
accuracy gap reported below.

### 6.3 Fixtures and hotwords file

Reused issue 032's `testdata/speech-context/corpus.tsv` phrasing, synthesized
locally with eSpeak NG 1.52.0 (same tool issue 032 used) and resampled to
16 kHz mono PCM with `ffmpeg` (sherpa-onnx requires 16 kHz input; eSpeak NG's
native output is 22.05 kHz):

```sh
espeak-ng -v en-us -s 150 -w technical-core.wav \
  "Voxi uses voxtype with dotool on PipeWire and Wayland."
espeak-ng -v en-us -s 150 -w technical-files.wav \
  "Update models.yaml and context_test.go with Cobra and systemd."
ffmpeg -y -i technical-core.wav  -ar 16000 -ac 1 technical-core-16k.wav
ffmpeg -y -i technical-files.wav -ar 16000 -ac 1 technical-files-16k.wav
```

sherpa-onnx's `hotwords_file` for a BPE-modeling-unit model does **not**
accept plain words directly with `hotwords_score` alone; it additionally
needs `modeling_unit="bpe"` and a `bpe_vocab` file (a `<piece>\t<score>`
table exported from `bpe.model`) so the C++ side can re-tokenize hotword
text into the same subword units as `tokens.txt`. A first attempt at manually
pre-tokenizing hotwords with the `sentencepiece` Python package failed with
"Cannot find ID for token ..." errors, because manual `sp.encode()` output
does not always match the piece boundaries the C++ encoder falls back to.
Passing plain words plus `modeling_unit`/`bpe_vocab` and letting sherpa-onnx
do its own encoding internally worked cleanly:

```
hotwords_plain.txt:
VOXI
VOXTYPE
DOTOOL
PIPEWIRE
WAYLAND
COBRA
SYSTEMD
MODELS.YAML
CONTEXT_TEST.GO
```

```python
sherpa_onnx.OnlineRecognizer.from_transducer(
    tokens=..., encoder=..., decoder=..., joiner=...,
    decoding_method="modified_beam_search", max_active_paths=4,
    hotwords_file="hotwords_plain.txt", hotwords_score=2.0,
    modeling_unit="bpe", bpe_vocab="bpe.vocab",
)
```

### 6.4 Transcripts and timing

Canary environment: same machine as issues 032/040 — AMD Ryzen 5 PRO 5650U
(12 logical CPUs), Linux 7.1.9 x86_64, CPU-only ONNX Runtime (no GPU used by
sherpa-onnx here), `sherpa-onnx 1.13.7`.

| Fixture | Mode | Transcript | Wall time | RTF |
|---|---|---|---|---|
| technical-core (4.45s) | baseline | `USE IN MOCCUS TYPE WITHIN LONG PIPE WIRE AND WHEYLAND` | 0.362s | 0.081 |
| technical-core | hotwords | `USE IN MOCCUS TYPE WITHIN LONG PIPE WIRE AND WAYLAND` | 0.374s | 0.084 |
| technical-files (5.71s) | baseline | `GREAT MARBLE'S ARCHIAML AND CONTEXT TEST NOTE WITH COALER AND SISTER THE` | 0.438s | 0.077 |
| technical-files | hotwords | `GREAT MARBLE'S ARCHIAML AND CONTEXT TEST NOTE WITH COALER AND SYSTEM THE` | 0.402s | 0.071 |
| technical-core (fp32 sanity check) | baseline | `USE IN MOCCUS TYPE WITHIN LONG PIPE WIRE AND WHEYLAND` (identical to int8) | 0.441s | 0.099 |

Expected transcripts (from `testdata/speech-context/corpus.tsv`):

- `Voxi uses voxtype with dotool on PipeWire and Wayland.`
- `Update models.yaml and context_test.go with Cobra and systemd.`

For comparison, whisper.cpp `small.en` (issue 032 Section 6, same synthesized
phrasing style) with its `--initial-prompt` biasing recovered the technical
core sentence essentially exactly: `Voxi uses voice type with dotool and
PipeWire and Wayland.` — only "voxtype" mis-heard as "voice type" (an
unbiased near-homophone), every other term correct.

### 6.5 Findings

1. **The hotwords mechanism itself works and is measurably real** — not a
   no-op. `--hotwords-file`/`hotwords_score` changed the output on both
   fixtures: `WHEYLAND` → `WAYLAND` (exact fix) and `SISTER` → `SYSTEM`
   (partial fix, still not `SYSTEMD`). This confirms sherpa-onnx's
   `modified_beam_search` + hotwords decoding does bias the beam toward the
   target vocabulary, exactly as issue 039 characterized it.
2. **But this streaming Zipformer2 model's baseline acoustic accuracy on
   Voxi's technical vocabulary is far below whisper.cpp `small.en`'s**, with
   or without hotwords. Every domain term in both fixtures came out wrong in
   both modes except the two hotword-assisted corrections above: `Voxi` →
   `USE IN`, `voxtype` → `MOCCUS TYPE`, `dotool` → `WITHIN LONG`, `PipeWire`
   → `PIPE WIRE` (split, no biasing effect despite being a hotword),
   `models.yaml` → `MARBLE'S ARCHIAML`, `context_test.go` → `CONTEXT TEST
   NOTE`, `Cobra` → `COALER`. Hotword biasing only nudges the beam toward a
   candidate that's already acoustically close; it cannot repair a baseline
   this far off.
3. **Quantization was not the cause** — the fp32 encoder produced an
   identical transcript to the int8 encoder on the fixture retested, only
   ~20% slower.
4. **Latency/RTF is excellent** — both modes ran at roughly RTF 0.07–0.10 on
   CPU (10–14x faster than real time), comparable to or faster than the
   1.46–1.47s wall time whisper.cpp `small.en` recorded on an 11-second clip
   in issue 032 (RTF ≈0.13–0.14). Speed is not what would block adoption.
5. This is a training-data/model-scale gap (LibriSpeech/GigaSpeech-trained,
   general-purpose reading speech), not a decoding-technique gap. A larger or
   differently-trained sherpa-onnx checkpoint might close it, but that is a
   materially different (and likely materially heavier) canary than "swap
   the decoding technique."

### 6.6 Operational cost (Section 3 item 4)

- New runtime dependency: `sherpa-onnx` Python wheel installs cleanly and
  fast via pip (no source build needed on this machine), but production
  integration would need the **Go** bindings (`sherpa-onnx-go`), not the
  Python package used for this canary — an unevaluated additional surface.
- Model licensing: Apache-2.0, same as sherpa-onnx itself; no restriction
  found.
- Model/checkpoint size: 310 MB download for the only English streaming
  model with hotwords support readily available (the smaller 128 MB model
  lacks the BPE vocab file hotwords require) — meaningfully larger than
  `voxtype`'s already-installed `small.en` binary path.
- Would require running two STT engines side by side or fully replacing
  whisper.cpp/voxtype as Voxi's transcription backend — a substantial
  `internal/eager` integration, not a drop-in addition.

### 6.7 Recommendation: Reject (for now)

**Reject adopting sherpa-onnx as a replacement or supplement to
voxtype/whisper.cpp for Voxi's vocabulary-biasing needs.** The hotwords
mechanism is real and does what issue 039 promised, but the specific small
streaming checkpoint practical to canary here is acoustically much weaker on
Voxi's technical vocabulary than the already-installed whisper.cpp
`small.en` + `--initial-prompt` combination issue 032 already ships behind
`voxi eager --speech-context`. Swapping engines would trade a working,
already-integrated, low-cost biasing mechanism for a new runtime dependency,
a larger model download, and materially worse baseline accuracy — with no
Go integration path evaluated. No new implementation ticket is scoped.

This does not close the door permanently: if a future sherpa-onnx checkpoint
with materially better English accuracy becomes available (or Voxi's
technical-vocabulary demands grow beyond what initial-prompt biasing can
carry), revisit with a stronger model. In the meantime, issue 032's own
Section 7 real-microphone measurement gate remains the next actionable step
for validating the currently-shipped approach — it is unaffected by this
ticket's outcome and was not run here.
