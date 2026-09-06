# 066: Canary: Cohere Transcribe and Nemotron 3.5 Streaming as Alternative ASR Backends

**Status**: Canary Complete — Recommend Adopt Cohere Transcribe as Additional Backend; Reject Nemotron 3.5 Streaming
**Priority**: P2 (Medium)
**Severity**: Informational
**Category**: Research / Feature
**Related**: [064 FluidVoice model landscape research](064-fluidvoice-model-landscape-fluid-intelligence-licensing-and-underlying-stt-model-portability-research.md), [039 OSS STT landscape research](039-oss-stt-landscape-and-custom-vocabulary-research.md), [041 sherpa-onnx hotwords canary](041-sherpa-onnx-hotwords-canary.md), [050 Optional warm-model daemon transcription](050-optional-warm-model-daemon-transcription.md), [073 FluidVoice model-download URL research (lower priority, do this canary first)](073-read-fluidvoice-s-model-download-code-paths-for-direct-weight-url-reuse-research.md)

---

## 1. Problem & Motivation

Issue 064's research (triggered by evaluating `altic-dev/FluidVoice`'s
model lineup) turned up two STT models that are both **open-weight,
Apache-2.0-style licensed, and Linux-portable** — a materially stronger
combination than anything issue 039's landscape survey found at the time:

- **Cohere Transcribe** — open-sourced ~March 2026, Apache-2.0, ~2B
  params, tops the Hugging Face Open ASR Leaderboard, reportedly beating
  Whisper large-v3 (5.42% WER). Has a dedicated whisper.cpp-style C++
  runtime (`CrispASR`) plus ONNX exports.
- **Nemotron Speech 3.5 (streaming variant)** — NVIDIA, open weights
  (OpenMDW-1.1), streaming architecture that matches Voxi's own eager
  streaming pipeline more closely than a batch model would. A community
  `sherpa-onnx` INT8 export already exists, meaning it could reuse the same
  sherpa-onnx Go bindings path issue 039/041 already scoped.

Both are candidates to sit alongside (or replace) Voxi's current whisper.cpp
`small.en` baseline. This is unverified secondhand research (web search,
not a hands-on build) — this ticket exists to actually run both models
against Voxi's existing benchmark harness before any adoption decision.

## 2. Explicit Gate — Do Not Start Implementation Until

- Both models are confirmed installable/runnable locally under a CPU or
  single-GPU budget comparable to Voxi's current whisper.cpp setup (license
  terms, weight download source, and runtime dependency all re-verified
  first-hand, not taken on issue 064's secondhand research alone).
- The canary itself (Section 3) is run and recorded — this ticket does not
  pre-authorize a backend swap, only a measurement.

## 3. Desired Design (Canary Only — Not Production Plumbing)

1. **Verify licensing/artifact provenance first-hand**: re-confirm Cohere
   Transcribe's Apache-2.0 license and Hugging Face weight availability, and
   Nemotron 3.5 streaming's OpenMDW-1.1 terms and the community sherpa-onnx
   INT8 export's provenance/trustworthiness (who published it, is it
   reproducible from the official checkpoint).
2. **Build/install** each candidate locally, independent of Voxi's existing
   whisper.cpp pipeline — `CrispASR` (or ONNX runtime) for Cohere Transcribe,
   sherpa-onnx for Nemotron 3.5 streaming (reusing whatever sherpa-onnx
   install/Go-bindings groundwork issue 041 already did, if any survives).
3. Reuse issue 032's fixture corpus (`testdata/speech-context/`, private
   WAVs, git-ignored) to run the same paired comparison Voxi has used for
   every prior engine/vocabulary canary: baseline vs. candidate, same
   technical-vocabulary terms.
4. Record WER, exact keyterm recall, latency/RTF (streaming latency matters
   more for Nemotron given Voxi's eager pipeline), and model/binary size
   against the whisper.cpp `small.en` baseline, using the same hardware
   prior canaries (032/040/041) ran on.
5. Evaluate operational cost: new runtime dependency footprint, Go bindings
   maturity (sherpa-onnx path) or CGo/C++ bridging cost (`CrispASR`),
   model download size, and whether either integrates with Voxi's existing
   `internal/speechcontext` vocabulary-biasing mechanism at all (Cohere
   Transcribe and Nemotron may not support prompt-based biasing the way
   whisper.cpp does — check before assuming parity).

## 4. Acceptance Criteria (for the canary itself, not adoption)

- A working, reproducible install/run command for each candidate is
  recorded in this ticket.
- The comparison table (WER, keyterm recall, latency/RTF, size, dependency
  cost) is recorded against the whisper.cpp baseline, for both candidates,
  with hardware details.
- An explicit recommendation: adopt one/both as an additional selectable
  backend, adopt as a default-swap candidate (only if a clear win with no
  material regression), or reject with reasons — matching the verdict style
  of issues 041/051.

## 5. Non-Goals

- No production backend integration in this ticket — canary/benchmark only.
- Not re-litigating issue 064's licensing research — this ticket's job is
  to verify it hands-on and measure accuracy/latency, not redo the survey.
- Not evaluating Cohere Transcribe/Nemotron's decoder-level vocabulary
  biasing support in depth beyond a yes/no check — a deeper biasing
  investigation (if either is adopted) would be its own follow-up, mirroring
  how 032/040/041 handled whisper.cpp/sherpa-onnx.

## 6. Background

Raised 2026-09-06, following the FluidVoice evaluation sprint (issues
062-065) and specifically issue 064's model-landscape research, at the
user's request to canary the two highest-value model findings before
treating them as anything more than research.

**See also**: [064](064-fluidvoice-model-landscape-fluid-intelligence-licensing-and-underlying-stt-model-portability-research.md)
for the full per-model verdict table and licensing detail behind §1's
summary, and the session retrospective at
[docs/studies/2026-09-07-fluidvoice-review-and-chunk-diagnostics.md](../docs/studies/2026-09-07-fluidvoice-review-and-chunk-diagnostics.md).

---

## 7. Canary Results (2026-09-07)

Hardware: same machine as issues 032/040/041 — AMD Ryzen 5 PRO 5650U
(12 logical CPUs), 23 GiB RAM, Linux 7.1.12 x86_64. No NVIDIA GPU present
(AMD Vega iGPU only). `voxtype 0.7.5` baseline runs with its installed
Vulkan-GPU backend (AMD iGPU); both candidates below ran **CPU-only**
(neither `CrispASR` nor `sherpa-onnx` was built/configured with a GPU
backend for this canary) — noted per-row below since it affects the
latency comparison.

### 7.1 Licensing/provenance re-verification (first-hand, not taken from 064 as-is)

**Cohere Transcribe 03-2026** (`CohereLabs/cohere-transcribe-03-2026`):
confirmed live on Hugging Face, Apache-2.0, 2B params (Conformer encoder:
48 layers/d_model 1280; Transformer decoder: 8 layers/d_model 1024 — read
directly out of the GGUF header, see 7.2), safetensors on the official
repo. One correction to 064: **the official `CohereLabs` repo is gated**
(`gated: "auto"` via the HF API — requires an HF account and accepting
terms before download), not freely downloadable as 064 implied. The
`vigneshlabs` ONNX repo 064 cited was not directly used or re-verified;
instead this canary used `CrispStrobe/CrispASR` (MIT, the same whisper.cpp-
style runtime 064 identified) built from source, paired with the
**ungated** community GGUF mirror `cstr/cohere-transcribe-03-2026-GGUF`
(README states "Apache 2.0, inherited from source model," converted from
`CohereLabs/cohere-transcribe-03-2026`). That mirror's README does **not**
disclose its conversion tool/script or report WER for the quantized
variants — an unverified-provenance caveat of the same shape 064 flagged
for the Nemotron sherpa-onnx export, just on the Cohere side instead. GGUF
metadata (`general.architecture = cohere-transcribe`, `general.name =
Cohere Transcribe 03-2026`) matches the official model's stated
architecture, which is at least consistent with a genuine conversion
rather than a mislabeled file.

**Nemotron Speech 3.5 streaming** (`nvidia/nemotron-3.5-asr-streaming-0.6b`):
confirmed live on Hugging Face, OpenMDW-1.1, 600M params, Cache-Aware
FastConformer-RNNT, ungated. This canary used
`apbaxel/sherpa-onnx-nemotron-3.5-asr-streaming-0.6b-int8` — **correction
to 064's framing**: this is not an unofficial/community re-export. Its
README states it mirrors the `k2-fsa/sherpa-onnx` project's own official
release package (`sherpa-onnx-nemotron-3.5-asr-streaming-0.6b-560ms-int8-
2026-06-11`, packaged against sherpa-onnx v1.13.4) "exactly as packaged
upstream — no re-quantization, no re-export, no modification." So the
sherpa-onnx *project itself* produced and hosts this export, which is a
materially stronger provenance story than 064's "community, unverified
maintenance" framing suggested — this is closer to first-party than
third-party.

### 7.2 Install/build — reproducible commands

**Cohere Transcribe via CrispASR:**

```sh
# Runtime (MIT, CPU-only build; add -DGGML_VULKAN=ON for GPU, not tried here)
git clone --recursive --depth 1 https://github.com/CrispStrobe/CrispASR
cd CrispASR
cmake -B build -DCMAKE_BUILD_TYPE=Release
cmake --build build -j$(nproc)        # ~7 minutes on this hardware; builds
                                       # 119 unrelated ASR/TTS backends too —
                                       # a much heavier source tree (470 MB)
                                       # than whisper.cpp's, though the final
                                       # crispasr binary itself is only 4.7 MB

# Weights (community GGUF mirror, ungated; official CohereLabs repo is
# gated and requires an HF login/token, not attempted here)
python3 -c "
from huggingface_hub import hf_hub_download
hf_hub_download(repo_id='cstr/cohere-transcribe-03-2026-GGUF',
                 filename='cohere-transcribe-q5_0.gguf', local_dir='.')
"   # 1.66 GiB download

# Run
./build/bin/crispasr -m cohere-transcribe-q5_0.gguf --backend cohere \
  -np -nt -t 6 -f <wav file>
```

Note: verbose output (`-v`) shows a small bundled `ggml-tiny.bin` (77 MB,
genuine Whisper-tiny) loading alongside the Cohere weights — this is
CrispASR's language-ID sub-model, not a silent fallback of the main ASR;
the actual transcript comes from the 1.66 GiB Cohere weights (confirmed by
GGUF-header inspection and by the transcript quality/timing tracking the
larger model, not the 77 MB one).

**Nemotron 3.5 streaming via sherpa-onnx (Python bindings, matching issue
041's approach):**

```sh
pip3 install --user sherpa-onnx huggingface_hub   # sherpa-onnx 1.13.7, prebuilt wheel

python3 -c "
from huggingface_hub import hf_hub_download
for f in ['encoder.int8.onnx','decoder.int8.onnx','joiner.int8.onnx','tokens.txt']:
    hf_hub_download(repo_id='apbaxel/sherpa-onnx-nemotron-3.5-asr-streaming-0.6b-int8',
                     filename=f, local_dir='.')
"   # 682 MB total

python3 scripts/canary_066_alt_asr/nemotron_run.py <model_dir> <wav files...>
```

`scripts/canary_066_alt_asr/nemotron_run.py` (new, throwaway canary
script, not production code) feeds each WAV to `sherpa_onnx.
OnlineRecognizer.from_transducer(...)` in 100 ms chunks via
`accept_waveform`/`decode_stream`, mirroring a real streaming ingestion
pattern. `scripts/canary_066_alt_asr/score.py` reimplements
`speech_context_bench`'s exact WER (word-level Levenshtein) and
keyterm-containment logic in Python, since the Go harness is
voxtype/whisper.cpp-specific and could not be reused directly against
either candidate's own CLI/API — kept as a general-purpose scorer for
`corpus.tsv`-format fixtures against any `{id/file, transcript}` JSONL.

### 7.3 Fixture corpus used

Issue 032's canonical `testdata/speech-context/` corpus is present but its
git-ignored WAVs (`ordinary-jfk.wav`, `technical-core.wav`,
`technical-files.wav`) are **not recorded on this machine** — only
`artifact-keyboard-smash.wav` exists there, and it has no expected
transcript/keyterms in `corpus.tsv`. This did not block the canary: issue
032's/042's alternate private corpus at `~/.config/voxi/samples/`
(real recorded microphone audio, its own `corpus.tsv` in the same format)
was available and used instead, per that corpus's own documented purpose
("use it to include private recordings... point `-corpus` at this
directory instead"). 16 fixtures with real recorded speech and non-empty
expected transcripts were used (`bug-d-etc`, `hello-voxi-thinkpad`,
`hello-voxi-webcam`, `kt-cli`, `kt-config`, `kt-core`, `kt-daemon`,
`kt-formats`, `kt-git`, `kt-mixed-1`, `kt-mixed-2`, `kt-ordinary-1`,
`kt-ordinary-2`, `kt-sentences-plus-keyboard-noise`,
`kt-sentences-plus-silence`, `my-toolchain`) — 27 keyterm instances total
across the technical-vocabulary fixtures.

### 7.4 Comparison table

All three engines ran unprompted/without vocabulary biasing (candidates
don't support it — see 7.5), so this is a fair apples-to-apples baseline
comparison, not a biased-vs-unbiased one like 032/040's own tables.

| Engine | Mean WER | Keyterm recall | Median latency | Median RTF | Model/weights size | Runtime dependency |
|---|---|---|---|---|---|---|
| whisper.cpp `small.en` (voxtype 0.7.5, **Vulkan GPU**) | 0.2726 | 0.4444 (12/27) | 4926 ms | 0.66 | 487 MB | Already installed (voxtype binary 59 MB) |
| **Cohere Transcribe 03-2026** (CrispASR, **CPU-only**) | **0.2226** | **0.5926 (16/27)** | 4692 ms | **0.60** | 1.66 GiB (q5_0 GGUF) | New: `crispasr` binary (4.7 MB, static, MIT) + 1.66 GiB weights |
| Nemotron 3.5 streaming (sherpa-onnx int8, Python, **CPU-only**) | 0.3582 | 0.4074 (11/27) | 8510 ms | 1.24 (slower than real time) | 682 MB (int8 ONNX) | New: Python + `sherpa-onnx` pip package (37 MB) + 682 MB weights; no standalone binary exercised in this canary (Go bindings exist per issue 039 but were not built here) |

Cohere Transcribe beats the whisper.cpp baseline on **every** measured
axis (lower WER, higher keyterm recall, comparable-or-better latency)
while running CPU-only against a baseline that had GPU (Vulkan/AMD iGPU)
acceleration — a harder bar than a same-hardware-class comparison would
have been. Nemotron 3.5 streaming loses to the baseline on WER, keyterm
recall, and is meaningfully slower than real time in this configuration.

Representative transcripts (fixture `kt-core.wav`, expected "Voxi uses
voxtype with dotool on PipeWire and Wayland."):

- whisper.cpp small.en: *"Foxy uses FoxType with two tools on Pyfire and Waydend."*
- Cohere Transcribe: *"Voxey uses VoxType with DoTool on Pipefire and Wayland."*
- Nemotron 3.5 streaming: *"Foxy uses Vox type with two tool on pipe wire and wayland"*

None of the three gets Voxi's own proper nouns exactly right unprompted
(expected, since none had vocabulary biasing active — see 7.5), but
Cohere's errors are consistently closer (e.g. "DoTool"/"Pipefire" vs
whisper/Nemotron's "two tool(s)"/"Pyfire"/"pipe wire").

One qualitative note not captured by the WER metric: on the very short
(0.64 s) `bug-d-etc` fixture, whisper.cpp hallucinated a fluent but
entirely wrong "Thank you." while Nemotron returned nothing and Cohere
returned a fragment ("you") — for a dictation tool, silence/fragments on
ambiguous short audio are arguably safer failure modes than confident
hallucination, though this is a single data point.

### 7.5 Vocabulary-biasing check (yes/no only, per non-goals)

- **Cohere Transcribe (CrispASR)**: **No.** `--prompt` and `--hotwords`
  are accepted without error but produce byte-identical output to the
  unprompted run on `kt-core.wav` — CrispASR's own `--help` text
  annotates those flags as backend-specific ("granite: KWB prompt",
  "vibevoice-asr only"); Cohere's Conformer-encoder/Transformer-decoder
  architecture (trained from scratch for transcription, not a
  general-purpose LLM decoder) isn't listed for either and empirically
  ignores both. No integration path to `internal/speechcontext`'s
  prompt-building exists for this engine as installed.
- **Nemotron 3.5 streaming (sherpa-onnx)**: **Partial/untested-in-depth.**
  `sherpa_onnx.OnlineRecognizer.from_transducer(...)` exposes the same
  generic `hotwords_file`/`hotwords_score`/`modeling_unit`/`bpe_vocab`
  parameters issue 041's Zipformer canary used successfully — the API
  surface for hotword biasing is present on this export (13,088-entry
  BPE `tokens.txt` shipped). Per this ticket's non-goals, actually
  wiring and testing hotword recall was not attempted; flagged as
  plausible-but-unverified for a future ticket if Nemotron were ever
  reconsidered.

### 7.6 Recommendation

- **Cohere Transcribe: adopt as an additional selectable backend**
  (not a default-swap in this ticket). It wins on every measured axis
  against whisper.cpp `small.en` even while running CPU-only against a
  GPU-accelerated baseline, ships a whisper.cpp-equivalent single static
  binary (MIT `CrispASR`, 4.7 MB) with no Python/PyTorch runtime, and its
  license (Apache-2.0) is compatible with Voxi. Caveats that keep this
  short of a default-swap recommendation: (1) the weights used here came
  from an **ungated community GGUF mirror** with undisclosed conversion
  tooling and no reported quantized-variant WER — the *official* gated
  `CohereLabs` repo should be re-verified against this canary's numbers
  before any default-swap decision, ideally by converting from the
  official safetensors checkpoint directly rather than trusting the
  mirror; (2) no vocabulary-biasing integration exists (7.5) — Voxi's
  `internal/speechcontext` mechanism would need a new decoder-side
  approach (e.g. post-hoc term substitution) rather than reusing the
  initial-prompt path; (3) weights are 3.4x whisper `small.en`'s size
  (1.66 GiB vs 487 MB), a real disk/download cost for a single-binary
  desktop tool. A follow-up implementation ticket should scope the
  `internal/eager`/`internal/asr` integration point, the official-weights
  re-verification, and a decoder-side biasing story before this becomes
  Voxi's default.
- **Nemotron 3.5 streaming: reject** (do not pursue further, even as an
  additional backend). It underperforms the whisper.cpp baseline on both
  accuracy axes measured here and ran slower than real time (RTF 1.24)
  in this CPU/Python configuration, despite being marketed as a streaming
  model that should fit Voxi's eager pipeline well. The provenance
  finding in 7.1 (it's an official sherpa-onnx-project export, not an
  unverified community one) removes the *provenance* objection 064
  worried about, but the *measured* accuracy/latency result is the
  actual blocker now that both are known. Two mitigations were not
  tried and would need to precede any reconsideration: (a) a native
  C++ sherpa-onnx binary or its Go bindings instead of the Python API
  used here, which added per-chunk Python/GIL overhead that likely
  inflates the RTF; (b) the model's native 560 ms cache-aware chunking
  (this canary fed naive 100 ms chunks) — but the WER/keyterm-recall
  deficit versus whisper.cpp is independent of the latency measurement
  and would need to close regardless.

Neither candidate should be treated as pre-approved for production
integration by this ticket — per §5's non-goals, a separate
implementation ticket is required if Cohere Transcribe is pursued.
