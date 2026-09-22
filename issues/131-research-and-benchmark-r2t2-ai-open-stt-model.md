# 131 — Research and benchmark R2T2.ai open STT model

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Performance
**Related**: Current Cohere transcription integration and repository speech samples

---

## 1. Problem & Motivation

Investigate the open speech-to-text model/service associated with R2T2.ai and determine whether it can improve voxi's current Cohere transcription pipeline. The project needs an evidence-based comparison before adding another provider or changing the default model.

## 2. Technical Specification / Findings

Research the available R2T2.ai model, license, distribution/API, runtime requirements, supported languages, and integration surface. Resolve uncertainties about the exact model/repository and whether it is practical to run locally on the supported Linux/Wayland setup.

Use the existing voxi transcription samples and current Cohere path as the comparison baseline. Record transcription quality, latency, throughput/resource use, failure behavior, and operational constraints for equivalent runs. Do not assume R2T2.ai is compatible with the existing provider API until verified.

## 3. Implementation & Verification Plan

**/goal**: Produce a reproducible benchmark and, if R2T2.ai is viable, integrate it behind the existing transcription abstraction or add a narrowly scoped provider path, with tests and documented configuration. The work is done when the comparison against Cohere is recorded on the repository samples and the recommendation is clear: adopt, retain as an optional backend, or reject with reasons.

- Probe the authoritative R2T2.ai model/repository and establish a canary transcription path.
- Run both backends against the same available samples with equivalent settings; preserve benchmark methodology and results.
- Add the smallest maintainable integration only if the model passes the canary and licensing/runtime constraints.
- Verify with focused tests and the applicable project checks; document setup, limitations, and the decision.

**Status**: Draft

---

## 4. Advisory Findings (2026-09-22, quick research)

Unverified web claims are marked *(reported)*; code refs were spot-checked.

- **Identity**: the model is **R2T2** by NetEase Youdao, repo
  <https://github.com/netease-youdao/Confucius4-R2T2>, site <https://r2t2.ai/>.
  "R2T2.ai" is the marketing domain, not the vendor. "S2T2" does not exist.
- **License** *(reported)*: code Apache-2.0; weights under NetEase's own
  Model Use License Agreement. Review the terms before bundling or downloading
  the weights by default.
- **Model** *(reported)*: Qwen3-ASR based, true streaming and append-only
  (committed text is never revised), chunks of 80 ms to 2 s, about 200-600 ms
  latency. Optimized for Chinese and English, also supports DE, FR, ES, IT,
  JA, KO, PT, RU, AR. Local weights only; no hosted API.
- **Runtimes** *(reported)*: vLLM (CUDA, primary), Transformers, and a
  llama.cpp/GGUF backend. Target hardware is the T14's AMD Cezanne iGPU,
  which already runs Cohere and Qwen models on GPU with plenty of VRAM. vLLM
  is CUDA-first, so the GGUF path on the same GPU backend that `crispasr`
  uses is the likely route.
  **Unverified**: the GGUF instructions, the HF weight id, and the file size.
- **Documented canary** (vLLM path):
  `./run_example.sh audio.wav --model_path <Confucius4-R2T2> --infer_mode stream_vllm --language English --chunk_size_ms 160`

### Integration surface in voxi

- `internal/eager/cohere.go:17`: `cohereTranscribeEngine`, which wraps the
  external `crispasr` binary around GGUF weights; `crispASRTranscribeArgs` is at `:54`.
- `internal/eager/eager.go:327,337`: engine switch (`cohere-transcribe`,
  OpenAI); more per-engine branches at `:570-596` and `:702`.
- `spec/models.yaml:94`: the `engine:` registry. A new `r2t2` engine is added
  here first (Spec.md: no duplicated values in Go).

### Next steps (pre-work for M1)

1. Fetch the repo README's GGUF/llama.cpp section and the HF card to confirm
   the weight id, the size, and a GGUF run command. Check whether `crispasr` or
   upstream llama.cpp can already load it.
2. Run a canary on one repository sample on the Cezanne GPU, on the same
   backend as Cohere. Record wall time and VRAM/RSS.
3. Only if (2) passes: run the Cohere-vs-R2T2 benchmark on the same samples.
   Then add an `r2t2` engine modeled on `cohere.go`.

## 5. GGUF / GPU Probe (2026-09-22)

- **Weights** *(reported)*: the official GGUF is `netease-youdao/Confucius4-R2T2-GGUF`,
  in Q4_K_M 1.0 GiB, Q8_0 1.7 GiB, and f16 3.2 GiB. It also needs an audio
  **mmproj** file (f16 0.6 GiB, Q8_0 0.3 GiB) because it is a multimodal
  Qwen3-ASR model. The original weights are `netease-youdao/Confucius4-R2T2`.
- **Documented command** *(reported)*: `llama-cli -hf netease-youdao/Confucius4-R2T2-GGUF:Q4_K_M`.
  The model card warns that standard llama.cpp text workflows may not apply.
  **Unknown**: whether mainline llama.cpp's mtmd supports this audio mmproj.
- **Local crispasr** (verified): `~/.local/bin/crispasr` 0.8.32 reports
  `ggml backends : cpu`. This build has no Vulkan or HIP, which contradicts
  the assumption that Cohere runs on the GPU; that needs resolving
  separately. Its help lists no Qwen3-ASR or R2T2 model family.
- **Verdict**: crispasr cannot run R2T2 today. The most plausible route on the
  Cezanne GPU is upstream llama.cpp built with `GGML_VULKAN`.

### Revised next step (M1 canary)

Build llama.cpp with Vulkan in a scratch directory, then run Q4_K_M and the
mmproj on one repository WAV. Pass means an English transcript on the GPU;
record the latency and the VRAM used. Fail means mtmd rejects the audio mmproj.
In that case, stop and decide between the vLLM/ROCm route and rejecting R2T2.

## 6. M1 Canary Result (2026-09-22): WORKS on Vulkan

- **Setup** (verified): mainline llama.cpp `f95b0d9`, built with `-DGGML_VULKAN=ON`,
  target `llama-mtmd-cli`. Build deps were already installed. The Vulkan shader
  compile takes several minutes.
- **Command** (verified):
  `llama-mtmd-cli -m Confucius4-R2T2-Q4_K_M.gguf --mmproj mmproj-Confucius4-R2T2-Q8_0.gguf --audio <wav> -p "Transcribe the audio." -ngl 99 --temp 0`
- **GPU** (verified with `-v`): `using device Vulkan0 (AMD Radeon Graphics (RADV RENOIR))`,
  and all layers were assigned to Vulkan0. Mainline mtmd accepts the audio mmproj,
  with an "audio input is experimental" warning.
- **Result** on `~/.cache/voxi/bench/jfk-reference.wav` (11.0 s), verified:
  - Transcript: exact, "And so, my fellow Americans, ask not what your country
    can do for you; ask what you can do for your country." The output is prefixed
    with `language English<asr_text>`, which must be stripped.
  - Wall time: 6.0 s, i.e. RTF 0.55 cold, including about 3.3 s of model and
    mmproj load. Warm inference is roughly 2.7 s (RTF ≈ 0.25, *estimated* from
    log timestamps).
  - Peak RSS: 161 MB, because the weights live in VRAM.
- **Comparison** with the same clip in `voxi bench` on Vulkan: small.en 0.14,
  large-v3-turbo 0.45. Cohere/crispasr is not benched yet (issue 133).
- **Caveats**: this was one run of one clip, done batch-style through the CLI.
  Streaming (80 ms–2 s chunks) is not exercised, and a CLI run reloads the model
  every time, so real use needs a resident server (`llama-server` with audio) or a
  library binding.

### Next
M2: keep the model resident and measure warm RTF and latency on the bench clips
(llama-server with audio input, if supported), then decide the integration path
(a server process like crispasr vs. in-process). Also check whether streaming
chunk input is reachable through mainline llama.cpp at all.

### Local artifacts (reusable)
- Models: `~/.cache/voxi/models/Confucius4-R2T2-Q4_K_M.gguf`,
  `~/.cache/voxi/models/mmproj-Confucius4-R2T2-Q8_0.gguf`
- llama.cpp `f95b0d9` with a Vulkan build of `llama-mtmd-cli` and `llama-server`:
  `~/.cache/voxi/llama.cpp/build/bin/`. It was rebuilt in place after the move,
  because the CMake RUNPATH is absolute and a moved build dir can't find its
  libraries.

## 7. M2 Result (2026-09-22): resident llama-server, warm

- **Server** (verified): `llama-server -m Confucius4-R2T2-Q4_K_M.gguf --mmproj mmproj-Confucius4-R2T2-Q8_0.gguf -ngl 99`.
  The log shows `offloaded 29/29 layers to GPU` on Vulkan0. The server takes about
  24 s to become ready: model load plus the first Vulkan shader compile.
- **Warm** `jfk-reference.wav` (11.0 s), 3 runs after one warm-up (verified):
  median 2.02 s, **RTF 0.18**. The transcript is exact.
  - Server timings: prompt 113 ms and decode 1.79 s at 16 tok/s, so text decode
    dominates.
  - Caveat: `cache_n=170` shows the identical request hit the prompt cache, so
    audio encoding may have been skipped. M1 measured the audio encode at about
    0.5 s, which puts a realistic warm RTF on new audio at about 0.2–0.25
    (*estimated*).
- **Comparison**, same clip, Vulkan, `voxi bench`: small.en 0.14, R2T2 ~0.2,
  large-v3-turbo 0.45.
- **VRAM**: about 1.4 GiB of model files, plus about 0.9 GiB reserved for the
  mmproj (reported). The system-wide 8 GiB reading isn't specific to this
  process and is ignored.
- **Streaming** (verified): the server accepts one whole `input_audio` blob
  per request (`tools/server/server-common.cpp:1251-1263`). Neither
  `tools/server` nor `tools/mtmd` has an incremental or partial-audio input
  path (reported, grep only). R2T2's native 80 ms–2 s chunk streaming is **not
  reachable** through mainline llama.cpp. It is whole-utterance only, like
  crispasr/Cohere today.
- **Only one bench clip** exists locally (`~/.cache/voxi/bench/`), so there is
  no spread across clips.

### Verdict and next options
R2T2 via llama-server works as a GPU utterance engine. It is faster than
large-v3-turbo and close to small.en, and should be more accurate than
small.en (unverified). It does not deliver its headline feature, low-latency
streaming, without upstream work. Options:
1. Integrate it as an engine alongside `cohere-transcribe`: a resident
   llama-server with an OpenAI `input_audio` request. Bench it via issue 133's
   engine dispatch.
2. Accuracy check first: run a WER comparison against Cohere and whisper on
   more than one clip, which needs a small labelled clip set.
3. Native streaming needs NetEase's own runtime (vLLM/transformers, likely
   ROCm) and is out of scope unless streaming latency becomes the goal.
