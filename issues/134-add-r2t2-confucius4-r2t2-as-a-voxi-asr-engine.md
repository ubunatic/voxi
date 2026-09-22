# 134 — Add R2T2 (Confucius4-R2T2) as a voxi ASR engine

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Feature
**Related**: 131 (R2T2 evaluation, M1/M2 canaries), 133 (bench crispasr/Cohere), 126 (openai-transcribe)

---

## 1. Problem & Motivation

Issue 131 established (verified, 2026-09-22) that R2T2 runs on this machine's
Vulkan iGPU through mainline llama.cpp, transcribes the bench clip exactly, and
reaches RTF ~0.18-0.25 warm behind a resident `llama-server` — between
small.en (0.14) and large-v3-turbo (0.45). It is not selectable in voxi.

The likely cheap path: voxi already has an HTTP engine, `openai-transcribe`
(`internal/eager/openai_transcribe.go:24`), which POSTs to
`<base_url>/audio/transcriptions`, and llama.cpp's server registers exactly
that route (`~/.cache/voxi/llama.cpp/tools/server/server.cpp:266`, verified by
grep). If that route accepts R2T2 audio and returns clean text, this ticket may
need a `spec/models.yaml` entry and no new Go engine.

Known constraint from 131 §7: mainline llama.cpp is whole-utterance only. R2T2's
native 80 ms-2 s chunk streaming is out of scope here.

## 2. /goal

An R2T2 model is selectable in voxi and transcribes through a resident
llama-server on the GPU, reusing the existing engine machinery wherever
possible. `spec/models.yaml` carries the model, engine, and any cleanup flags;
Go does not duplicate spec values. The ticket states plainly who starts and owns
the llama-server process, and what happens when it is not running (a clear
error, not a hang). Tests cover the new path at the same level as the existing
engines.

## 3. Milestones

### M1 — Canary: does /v1/audio/transcriptions work for R2T2? (read-only + curl)
Start `llama-server -m ~/.cache/voxi/models/Confucius4-R2T2-Q4_K_M.gguf
--mmproj ~/.cache/voxi/models/mmproj-Confucius4-R2T2-Q8_0.gguf -ngl 99 --port
18131`, then POST `~/.cache/voxi/bench/jfk-reference.wav` to
`/v1/audio/transcriptions` with `response_format=text`, exactly as
`transcribeOpenAIWAV` does. Record: HTTP status, the raw body, whether the
`language English<asr_text>` prefix seen in 131 §6 leaks into the response, and
the wall time. Kill the server afterwards.
Decide from the result: (a) reuse `openai-transcribe` with a spec entry only, or
(b) a new engine is needed, and say exactly why.

### M2 — Wire it up
Implement the path M1 chose. Include process ownership: `openai-transcribe`
assumes an endpoint that is already running (unlike `cohere-transcribe`, which
spawns `crispasr` per utterance, `internal/eager/eager.go:327`). Either document
that the user starts llama-server, or add supervision — don't leave it implicit.
Weights: 131 recorded them in `~/.cache/voxi/models/`; decide whether to reuse
`ensureCohereWeights`-style auto-download (`internal/eager/cohere.go`) or require
a manual fetch, and write the choice down.

### M3 — Verify
Tests for engine dispatch and any response cleanup. Then a live check on the
bench clip, with RTF recorded next to the numbers in 131 §7.

## 4. Notes / Uncertainties

- Re-verify against current code before starting; this ticket may lag it.
- Unknown: whether llama.cpp's transcription route applies the chat template and
  the R2T2 ASR prompt correctly, or whether it needs the chat/completions
  `input_audio` form used in 131 §7. M1 settles this.
- Unknown: output hygiene. 131 §6 saw a `language English<asr_text>` prefix from
  the CLI. If it appears over HTTP, it must be stripped somewhere explicit.
- Accuracy is unmeasured. 131's option 2 (a WER comparison against Cohere and
  whisper) is not part of this ticket, but do not claim R2T2 is better than
  small.en until it exists.
