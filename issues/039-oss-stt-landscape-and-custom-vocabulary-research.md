# 039: OSS/Source-Available STT Landscape and Custom-Vocabulary Handling Research

**Status**: Research Complete
**Priority**: P3 (Low)
**Severity**: Informational
**Category**: Research
**Related**: [031 Claude Code voice-pipeline research](031-claude-code-and-agent-cli-voice-pipeline-research.md), [032 small.en vocabulary biasing](032-small-en-project-vocabulary-biasing.md), [035 SSH remote transcription server research](035-ssh-remote-transcription-server-research.md), [models specification](../spec/models.yaml)

---

## 1. Goal

Survey the current open-source and source-available STT landscape beyond
Voxi's existing Whisper-family baseline, with two specific angles:

1. How do on-device Android keyboards (Gboard, and any open alternatives)
   architect continuous/streaming STT with custom-word support, given they
   run under tighter latency and memory constraints than a desktop daemon?
2. What other source-available STT engines (any domain, not just mobile)
   deliver good English accuracy *and* a documented, working mechanism for
   biasing/injecting custom vocabulary or technical jargon — comparable to
   or better than Voxi's initial-prompt approach in issue 032?

This is a research-only ticket. No code changes. Output is a written
comparison to inform whether Voxi's model/vocabulary strategy should change.

## 2. Research Questions

### 2.1 Android / mobile keyboard STT architecture

- Gboard's voice typing: is any part of its STT stack (on-device RNN-T model,
  decoder, hotword/contextual biasing mechanism) documented or open-sourced by
  Google (e.g. via research papers, AOSP, or the on-device speech APIs)?
- What class of model do mobile keyboards use for streaming recognition
  (RNN-T / Conformer-transducer / CTC with a light LM) and why that class is
  preferred over Whisper-style encoder-decoder models for low-latency,
  low-memory, always-listening use?
- How is contextual biasing implemented on-device — e.g. shallow-fusion
  n-gram/FST biasing lists built from contacts, installed apps, and recently
  typed words, vs. a static vocabulary?
- Are there open alternatives on Android (e.g. Futo Keyboard/FUTO Voice Input,
  OpenBoard, AnySoftKeyboard voice plugins, Mozilla DeepSpeech-era keyboard
  integrations) that ship a real custom-word mechanism, and what is their
  actual technique (grammar/FST constraints, prompt biasing, post-decode
  fuzzy correction against a user dictionary)?
- What tradeoffs (accuracy, latency, battery, model size) do these mobile
  approaches make that would or wouldn't transfer to a desktop eager-VAD
  pipeline like Voxi's?

### 2.2 Other source-available STT engines with custom-vocabulary support

Survey candidates across categories, for each recording: license, whether
weights are open or only the runtime, approximate English accuracy relative
to Whisper `small.en`/`base.en`, and the *specific* custom-vocabulary
mechanism (if any):

- **Transducer/streaming**: NVIDIA NeMo (Parakeet/Conformer-Transducer,
  Canary), Kaldi/Vosk (grammar-based decoding graphs, dynamic vocabulary via
  FST), Icefall/k2/sherpa-onnx.
- **Whisper-family alternatives/forks**: whisper.cpp (initial-prompt/grammar
  support already used as Voxi's baseline), faster-whisper, WhisperX,
  distil-whisper — note any that add a first-class hotword/biasing API beyond
  the raw initial-prompt trick.
- **Other open engines**: Coqui STT (DeepSpeech successor, supports external
  scorer/KenLM word boosting), wav2vec2-based models with word-boosting via
  beam-search LM rescoring, Mozilla's original DeepSpeech `--lm`/`--trie`
  workflow, Meta's MMS/Seamless models.
- **Commercial-with-open-SDK or source-available-license systems** (only if
  license terms are compatible with a personal/hobby project — flag license
  type clearly rather than assuming permissiveness): Picovoice Leopard/Cheetah
  (source-available, on-device, explicit custom-vocabulary/keyword API),
  Speechmatics on-prem (proprietary but self-hostable), Amazon Transcribe
  custom-vocabulary as a reference for API shape only.
- For each: does "custom vocabulary" mean (a) biasing the decoder toward
  provided words, (b) a hard grammar/FST constraint, (c) post-hoc dictionary
  correction, or (d) fine-tuning/adapter training? Voxi currently only has (a)
  via prompt text — note which alternatives offer (b) or (c) as a lower-risk
  upgrade path.

### 2.3 Comparison to Voxi's current approach

- Where does Voxi's issue-032 initial-prompt biasing rank against the FST/
  grammar-constraint techniques above in terms of reliability for exact
  technical terms (e.g. CLI flags, package names) vs. general fluency?
- Is there a lightweight (no new heavy runtime dependency, no cloud call)
  technique from this survey Voxi could canary next — e.g. KenLM-style word
  boosting compatible with whisper.cpp/faster-whisper, or a small FST-based
  post-decode corrector — without abandoning Whisper as the base model?

## 3. Deliverables

1. A comparison table (engine, license, weights-open?, approx. accuracy
   class, custom-vocabulary mechanism, on-device feasibility) covering at
   least the candidates in 2.1 and 2.2.
2. A short write-up of the Android/mobile-keyboard streaming architecture and
   its contextual-biasing technique, with citations (papers, AOSP source,
   project READMEs).
3. A recommendation: either "no change, current initial-prompt approach is
   adequate" or "canary candidate X" with the specific integration point in
   Voxi's `internal/eager` pipeline it would touch.

## 4. Non-goals

- No implementation, dependency addition, or model swap in this ticket.
- No evaluation of non-English languages.
- No commitment to any commercial/cloud STT API — on-device and
  source-available options only, consistent with Voxi's local-first design.

## 5. Research Findings (2026-09-02)

### 5.1 Comparison table

| Engine | License | Weights open? | Accuracy vs Whisper small.en/base.en | Custom-vocab mechanism | On-device feasibility |
|---|---|---|---|---|---|
| whisper.cpp (Voxi baseline) | MIT | Yes (OpenAI Whisper weights, MIT/permissive) | Baseline | (a) `--initial-prompt` text conditioning; separate GBNF **grammar** support (`grammars/*.gbnf`) constrains the token search with a penalty outside the grammar — a real (b)-style soft constraint, not just prompt biasing | Already integrated; CPU/Vulkan, no new runtime |
| faster-whisper (CTranslate2) | MIT | Yes (same Whisper weights) | Same accuracy as whisper.cpp family, ~4x faster / less memory via CTranslate2 | (a) `initial_prompt`, plus a first-class `hotwords` decode-time parameter (distinct from prompt, does not get "forgotten" after 30s like `initial_prompt` can) | High — drop-in alternative runtime, but a new dependency (Python/CTranslate2 or Go bindings) vs Voxi's existing whisper.cpp binary |
| WhisperX | BSD-2 (wraps faster-whisper) | Yes | Same as faster-whisper + forced alignment | Inherits faster-whisper's `initial_prompt`/`hotwords`; adds word-level timestamps, not vocabulary biasing itself | Adds alignment-model dependency (wav2vec2) Voxi doesn't need |
| distil-whisper | MIT | Yes | Slightly below full Whisper on rare/technical words, faster | Same `initial_prompt` mechanism as whisper.cpp/faster-whisper (encoder-decoder architecture) | No distinct biasing advantage over current baseline |
| NVIDIA NeMo Parakeet-TDT (0.6B/1.1B) | CC-BY-4.0 (weights); NeMo toolkit Apache-2.0 | Yes, open weights on Hugging Face | Materially higher than Whisper small.en (near top of HF Open ASR Leaderboard; RNNT/TDT class) | (a) **Word/phrase boosting** via GPU-accelerated phrase-boosting shallow fusion at decode time (NeMo ≥2.5.0), no retraining; TDT limitation: single boost score for all words, no OOV boosting, no per-word/negative scores | Feasible but heavy: needs NeMo/PyTorch or an ONNX export runtime; no existing lightweight Go/C++ CLI equivalent to whisper.cpp |
| Kaldi / Vosk | Apache-2.0 | Yes (small models: fully open incl. dynamic-vocab graph parts; large "big" models often ship precompiled/static, vocab not modifiable) | Below Whisper small.en on open-domain English; competitive on narrow/controlled-vocabulary tasks | (b) **Hard grammar/FST constraint** — dynamically compiled HCLG.fst with add-on lexicon parts (kaldi's on-the-fly grammar-graph composition; `kaldi-active-grammar` optimizes bulk word addition) | Very feasible on-device (small footprint, real-time), but rebuilding for modern general English accuracy would mean abandoning Whisper entirely |
| k2 / icefall / sherpa-onnx | Apache-2.0 | Yes | Depends on trained model; icefall-trained transducers competitive with small/base Whisper tiers | (a) **Hotwords** via Aho-Corasick automaton shallow-fusion scoring; requires `modified_beam_search` decoding (not plain greedy); `--hotwords-file` / `--hotwords-score` CLI/API params | High — sherpa-onnx ships a portable C++/ONNX runtime (Go bindings exist) comparable in footprint to whisper.cpp; plausible canary target |
| Coqui STT (DeepSpeech successor) | MPL-2.0 | Yes | Well below Whisper small.en on general English (project discontinued 2022–23, unmaintained) | (a) External **KenLM scorer** + explicit **hot-word boosting API** (positive/negative per-word boost values, added at inference call) — closest prior art to a proper "word boost" API in a fully open codebase | Feasible but stale/unmaintained; accuracy regression makes it a non-starter as a base model |
| wav2vec2 + pyctcdecode/KenLM | Apache-2.0 (wav2vec2 models vary; pyctcdecode Apache-2.0) | Yes (many wav2vec2 English checkpoints open) | Roughly Whisper base.en tier depending on checkpoint/fine-tune; more brittle out-of-domain | (a) `hotword_weight` param in pyctcdecode combined with KenLM shallow fusion (alpha/beta LM weighting) — decoder-level boosting, not prompt text | Moderate — needs a CTC beam-search decoder + KenLM stack (Python-centric); no direct Go/CLI equivalent |
| Mozilla DeepSpeech (legacy) | MPL-2.0 | Yes | Well below Whisper small.en; project archived | (a)/(b) `--lm` + `--trie` external scorer/trie word list (predecessor to Coqui's mechanism) | Legacy only, not a live candidate |
| Meta MMS / Seamless(M4T) | CC-BY-NC 4.0 (MMS) / mixed, some SeamlessM4T variants non-commercial | Yes for research use; **non-commercial clause risk** | Competitive to above-Whisper on many languages; English tier close to Whisper small–medium | No first-class custom-vocab API found; would need pyctcdecode-style external LM fusion same as wav2vec2 | License blocks casual reuse in a project without clearing NC terms; not recommended |
| Picovoice Leopard/Cheetah | Source-available under Picovoice's own commercial license (not OSI-approved open source); free tier with usage limits | Runtime source-available in parts (Apache/BSD components), core **acoustic models are NOT open weights** — proprietary, license-gated | Vendor-claimed high accuracy, unverified independently against Whisper here | (a)/(b) explicit **custom-vocabulary + boost-word** authoring via Picovoice Console, compiled into a downloadable custom model file — the most polished first-party UX of anything surveyed | Feasible technically (small on-device SDKs), but license/account dependency conflicts with Voxi's local-first, no-account design goal |

Legend for mechanism column: (a) decoder biasing/shallow fusion toward provided words, (b) hard grammar/FST constraint, (c) post-hoc dictionary correction, (d) fine-tuning/adapter. No surveyed engine documented a pure (c) or (d) path as its primary supported custom-vocabulary mechanism; Voxi's issue-032 prompt technique is (a).

Sources: [whisper.cpp grammars](https://github.com/ggml-org/whisper.cpp/tree/master/grammars), [whisper.cpp hotwords issue #1979](https://github.com/ggml-org/whisper.cpp/issues/1979), [faster-whisper (SYSTRAN)](https://github.com/SYSTRAN/faster-whisper), [NeMo Word Boosting docs](https://docs.nvidia.com/nemo-framework/user-guide/latest/nemotoolkit/asr/asr_customization/word_boosting.html), [NeMo ASR LM & Customization guide](https://docs.nvidia.com/nemo-framework/user-guide/latest/nemotoolkit/asr/asr_language_modeling_and_customization.html), [nvidia/parakeet-tdt-0.6b-v3 model card](https://huggingface.co/nvidia/parakeet-tdt-0.6b-v3), [Vosk model adaptation](https://alphacephei.com/vosk/adaptation), [Vosk language-model adaptation](https://alphacephei.com/vosk/lm), [Kaldi grammar/graph docs](https://kaldi-asr.org/doc/grammar.html), [kaldi-active-grammar](https://github.com/daanzu/kaldi-active-grammar/releases), [sherpa-onnx hotwords docs](https://k2-fsa.github.io/sherpa/onnx/hotwords/index.html), [sherpa-onnx hotwords input format issue](https://github.com/k2-fsa/sherpa-onnx/issues/1697), [Coqui STT hot-word boosting](https://stt.readthedocs.io/en/latest/HotWordBoosting-Examples.html), [Coqui STT scorer/KenLM](https://stt.readthedocs.io/en/latest/playbook/SCORER.html), [pyctcdecode README](https://github.com/kensho-technologies/pyctcdecode/blob/main/README.md), [Picovoice Leopard docs](https://picovoice.ai/docs/leopard/), [Picovoice console tutorial](https://picovoice.ai/blog/console-tutorial-custom-speech-to-text-model/).

### 5.2 Android/mobile-keyboard streaming architecture and contextual biasing

**Model class.** Gboard's on-device recognizer switched from a hybrid CTC-based
pipeline to an all-neural, streaming **RNN-Transducer (RNN-T)** model,
announced in Google's 2019 research post ["An All-Neural On-Device Speech
Recognizer"](https://research.google/blog/an-all-neural-on-device-speech-recognizer/)
(mirrored at
[ai.googleblog.com](https://ai.googleblog.com/2019/03/an-all-neural-on-device-speech.html?m=1)).
The underlying architecture is described in the paper ["Streaming End-to-End
Speech Recognition for Mobile Devices"](https://arxiv.org/pdf/1811.06621)
(He et al., ICASSP 2019, also on
[OpenReview](https://openreview.net/pdf?id=B1eHt2ltDS)). Key points from the
paper/post:

- RNN-T was chosen over encoder-decoder attention models (the Whisper family's
  architecture) specifically because it produces output **incrementally**,
  frame-by-frame, with no need to see the whole utterance before decoding
  starts — a hard requirement for live, always-listening dictation where
  Whisper-style chunked/30-second-window decoding is unsuitable.
- The production model was originally ~450MB; **8-bit quantization** gave a
  4x size/speed win, landing at ~80MB, small enough to ship in the keyboard
  APK/on-device model bundle rather than call a server.
- Google reports on-device WER at parity with their prior server-based
  hybrid model, i.e. no accuracy sacrifice was needed to go on-device and
  streaming.

**Contextual biasing.** Google's own description of the general technique
(used across Assistant/Gboard-class products), independent of the specific
2019 paper: "Contextual biasing is the problem of injecting prior knowledge
into an ASR system during inference, e.g. a user's favorite songs, contacts,
apps or location. Conventional ASR systems perform contextual biasing by
building an **n-gram finite-state transducer (FST)** from a list of biasing
phrases, composed on-the-fly with the decoder graph during decoding, biasing
recognition toward the n-grams in the contextual FST." This is documented in
Google's contextual-biasing patent filing
[WO2020226789A1](https://patents.google.com/patent/WO2020226789A1/en) and
discussed in follow-on academic work such as ["Tree-constrained Pointer
Generator with Graph Neural Network Encodings for Contextual Speech
Recognition"](https://arxiv.org/pdf/2207.00857), which frames Google's FST
shallow-fusion approach as the conventional baseline that neural
tree-constrained pointer-generator methods try to improve on. In short: this
is architecturally the **(b) hard-constraint / shallow-fusion FST class**,
not a text-prompt trick — Gboard biases toward contacts, installed-app names,
and location-derived vocabulary by composing a small per-session FST into the
transducer's decoding graph, not by prepending natural-language text.

- Android's public `RecognizerIntent` API exposes a coarser, app-facing analog:
  `EXTRA_BIASING_STRINGS` (API 33+) lets a calling app pass an array of
  hint strings to the system recognizer to bias results — the app-facing
  surface of the same idea, though Android's own reference page content
  could not be fully retrieved during this research pass (indirectly
  confirmed via [Microsoft Learn's Xamarin/MAUI binding
  docs](https://learn.microsoft.com/en-us/dotnet/api/android.speech.recognizerintent.extraaudiosource?view=net-android-34.0)
  and secondary sources); this is a public-API convenience layer over
  Google's internal FST-biasing machinery, not a description of the
  on-device model internals.

**Open Android alternatives.** None of the surveyed open keyboards implement
their own streaming STT model with custom-vocabulary biasing:

- **FUTO Voice Input** ([futo-org/voice-input](https://github.com/futo-org/voice-input),
  mirror of `gitlab.futo.org/keyboard/voiceinput`) is Whisper-based, not
  RNN-T — it runs **whisper.cpp** on-device (confirmed by the
  [lrq3000/futo-voiceinput-whisper mirror](https://github.com/lrq3000/futo-voiceinput-whisper)
  description: "Voice Input... transcribing using Whisper... supporting
  large multilanguage models"). It is architecturally the mobile analog of
  Voxi's own approach — encoder-decoder Whisper, not a transducer — and no
  documented first-class custom-vocabulary/biasing feature (prompt or FST)
  was found in its public materials during this pass.
- **OpenBoard** (AOSP-derived, FOSS, no Google binaries) has **no voice-input
  intent support at all** and has been unmaintained since December 2022.
- **AnySoftKeyboard** does not ship its own recognizer; it delegates to
  whatever `SpeechRecognizer`/`RecognizerIntent` service is installed
  (historically Google's), with open GitHub issues
  ([#3230](https://github.com/AnySoftKeyboard/AnySoftKeyboard/issues/3230),
  [#927](https://github.com/AnySoftKeyboard/AnySoftKeyboard/issues/927),
  [#1672](https://github.com/AnySoftKeyboard/AnySoftKeyboard/issues/1672))
  describing breakage when Google's voice-search package is absent — i.e. it
  has never had its own on-device model or biasing mechanism to survey.

**Takeaway for Voxi.** The RNN-T + FST-biasing architecture is the right
choice for a phone (streaming-native, tiny quantized model, hard latency/
battery budget), but it is not a transferable target for Voxi: building or
adopting a competitive from-scratch RNN-T model is out of scope for a hobby
project, and no open Android keyboard has assembled a shippable open
RNN-T+FST stack Voxi could borrow directly. The nearest desktop-available
open implementations of the *same class of technique* (transducer model +
shallow-fusion biasing, not a from-scratch phone stack) are NeMo/Parakeet and
k2/icefall/sherpa-onnx, both covered in the comparison table above.

### 5.3 Recommendation

**No model/runtime change now; canary sherpa-onnx hotwords as the next
concrete experiment if issue-032's measurement gate shows the initial-prompt
technique under-delivering on exact keyterm recall.**

Rationale:

- Voxi's issue-032 `--initial-prompt` mechanism is decoder *conditioning*,
  not a hard constraint — same category (a) as faster-whisper's
  `hotwords`, NeMo's word-boosting, sherpa-onnx's hotwords, and Coqui's
  hot-word API. It is weaker than Kaldi/Vosk's or a GBNF grammar's hard FST
  constraint (b) for guaranteeing an exact technical term appears verbatim,
  but it is also the only option in this survey that requires **zero new
  runtime dependency** — whisper.cpp is already Voxi's shipped backend.
- whisper.cpp's own **GBNF grammar** feature
  (`grammars/*.gbnf`, [ggml-org/whisper.cpp](https://github.com/ggml-org/whisper.cpp/tree/master/grammars))
  is a lower-risk upgrade path than swapping engines: it stays inside the
  existing whisper.cpp binary Voxi already invokes, and moves Voxi from
  category (a) prompt-biasing toward category (b) constrained decoding for
  a bounded set of exact terms (CLI flags, package names) without touching
  the model or adding a dependency. This is a stronger "next canary"
  candidate than any competing engine, and should be tried **before**
  a runtime swap.
- If GBNF grammar biasing is tried and still insufficient, the best
  justified engine swap is **sherpa-onnx** (Apache-2.0, open weights,
  portable C++/ONNX runtime with Go bindings, comparable footprint to
  whisper.cpp) using its documented `--hotwords-file`/`--hotwords-score`
  API with `modified_beam_search` decoding. Concrete integration point:
  `internal/eager/eager.go`'s command-construction path that currently
  builds the `voxtype`/whisper.cpp invocation (the same place issue-032's
  `--initial-prompt`/`--speech-context` flag is threaded through) would gain
  a parallel sherpa-onnx-backed transcription command behind its own opt-in
  flag, reusing the existing speech-context vocabulary builder as the
  hotwords-file source instead of prompt text.
- NeMo/Parakeet is the clear accuracy leader among alternatives but was
  ruled out as a near-term canary: it pulls in a PyTorch/NeMo (or a
  from-scratch ONNX export + custom runtime) dependency far heavier than
  Voxi's current single static whisper.cpp binary, which conflicts with the
  project's local-first, low-dependency posture.
- Picovoice and Meta MMS/SeamlessM4T were excluded from canary
  consideration: Picovoice's core models are license-gated/account-gated
  (not fully source-available), and MMS/SeamlessM4T carry a non-commercial
  license clause — both conflict with Voxi's local-first, no-account,
  freely-redistributable design goals.

### 5.5 Follow-up tickets

- [040 GBNF grammar-constrained vocabulary](040-whisper-cpp-grammar-constrained-vocabulary.md) —
  pursue the first recommendation (whisper.cpp's own GBNF grammar feature)
  before any engine change.
- [041 sherpa-onnx hotwords canary](041-sherpa-onnx-hotwords-canary.md) —
  pursue the contingent second recommendation only if 032/040's measurement
  gates show in-engine biasing is still insufficient.
