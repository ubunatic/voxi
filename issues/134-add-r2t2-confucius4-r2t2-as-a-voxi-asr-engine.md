# 134 — Add R2T2 (Confucius4-R2T2) as a voxi ASR engine

**Status**: Closed — Done; see docs/Roadmap.md close/park section
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

## 5. M1 Progress & Resource Trap (2026-09-22)

M1 is **not finished**. Two canary attempts were interrupted; nothing landed in
the repo.

### Carried-over finding
- The `language English<asr_text>` prefix seen from the CLI (131 §6) **also
  appears over HTTP** (reported, from an interrupted run; re-verify and quote
  the body). So M2 needs an explicit strip step wherever the response is read.

### Resource trap — read before starting llama-server here
On this T14 (AMD Cezanne), GPU VRAM **is** system RAM. `llama-server` fits its
context to the free VRAM it sees (about 27 GiB), so
`llama-server ... -ngl 99` with no context cap inflated to **26.5 GiB VRAM and
99% RAM** and made the machine unusable (observed 2026-09-22). The earlier M2
run (131 §7) happened to stay near 8 GiB, so this does not show up every time.

Always start it with an explicit cap, e.g.:

```
llama-server -m <gguf> --mmproj <mmproj> -ngl 99 -c 4096 --no-warmup --port 18131
```

and check `free -m` plus `/sys/class/drm/card*/device/mem_info_vram_used`
**after** startup, not only before. Kill the server as soon as the measurement
is done. Two earlier heavy builds also pushed desktop apps into swap; do not
run parallel builds above `-j 6` on this machine.

### M1 remaining
Steps 3-5 of M1 above: POST the bench clip to `/v1/audio/transcriptions` exactly
as `transcribeOpenAIWAV` does, record status, verbatim body, and wall time, then
decide (a) spec-only or (b) Go changes.

## 6. M1 Result (2026-09-22): DECIDED — (b) Go changes needed

Measured against a resident `llama-server` with the capped command from §5.
Resources stayed safe: VRAM peaked at 2.78 GiB, available RAM never dropped
below 18.8 GB, and `n_ctx_slot = 4096` confirms the `-c` cap held. So the
`-c 4096` cap is what makes this safe — without it, see §5.

- `response_format=text` → **HTTP 400** (verified):
  `Only 'json' response_format is supported for transcription`
- `response_format=json` → **HTTP 200 in 1.99 s** (verified), body verbatim:
  `{"type":"transcript.text.done","text":"language English<asr_text>And so, my fellow Americans, ask not what your country can do for you; ask what you can do for your country.","usage":{...}}`
- Transcript content is correct; the `language English<asr_text>` prefix leaks
  into the HTTP response, as suspected.
- GPU use was **not** logged at this verbosity and is only inferred from timing
  and VRAM (*unverified*). 131 §7 verified `offloaded 29/29 layers to GPU` for
  the same binary and flags, so this is not a new risk — but don't cite M1 as
  GPU evidence.

### Why spec-only is impossible
`internal/eager/openai_transcribe.go` hardcodes `response_format=text` (`:100`)
and returns the **raw body** with no JSON parsing at all (verified: zero
`json.Unmarshal` in that file). Against llama-server that yields a 400, and
even on success the body would be JSON, not a transcript.

### M2 scope (replaces the M2 sketch in §3)
1. Per-backend `response_format`: this engine must send `json`. Drive it from
   `spec/models.yaml`, not a hardcoded branch on the model name.
2. Parse the JSON envelope: read `.text` from
   `{"type":"transcript.text.done","text":...}`. Keep the existing raw-text
   path working for whisper-server/agy (issue 126), which returns plain text.
3. Strip the `language English<asr_text>` prefix. Make it an explicit,
   spec-driven cleanup rule with a test, not a silent regex buried in the
   engine.
4. Process ownership: llama-server must be started with the §5 capped command.
   Decide documented-manual-start vs. supervision, and write the choice down.
   An uncapped start is a machine-hanging bug, so if voxi ever starts it, the
   cap is not optional.

## 7. M3 Result (2026-09-22): live corpus run, engine works, accuracy is the problem

Ran `VOXI_LIVE_ASR=1 go test -count=1 ./internal/eager/ -run TestLiveASR_R2T2Corpus -v`
against a capped llama-server (§5 command), over the 24 labelled clips in
`~/.config/voxi/samples/corpus.tsv`. Resources stayed safe: VRAM 2.63 GiB,
available RAM never below 18.9 GB.

**The plumbing works.** Per-model `base_url` routes to :18131, the JSON envelope
is parsed, and the `<asr_text>` marker never leaked in 24 live responses. That
is M2's contract verified end to end, not by mock.

**Latency** (verified): short clips 0.5-1.0 s, ordinary sentences 1.4-2.0 s,
the two multi-sentence clips 3.8 s and 4.6 s. Whole corpus in 35 s.

**Accuracy** (verified, word-level similarity against the corrected transcripts):
- 11 of 24 clips exact or near-exact (100%), including both multi-sentence clips
  and the noise/silence robustness clips.
- **2 clips returned an EMPTY transcript**: `bug-d-etc` (a full sentence,
  "So we either extend the classification system") and `short-uh`. The test
  flagged both. An empty return on a real utterance is a correctness bug, not a
  wording difference — in eager mode it would silently drop speech.
- **Domain vocabulary fails badly**: "Voxi" -> "Foxy"/"Voxie"/"Boxey",
  "voxtype" -> "box type", "PipeWire" -> "pipe wire", "harnez" -> "harness",
  "uman" -> "human", "Golang" -> "GoLand", "systemd" -> "system d".
  Similarity 38-54% on the voxi-jargon clips.

### Verdict
R2T2 is usable as an engine but is **not ready to be a default**. The vocabulary
misses are exactly what `keyterms` in corpus.tsv exist for, and issue 074 §5
records that crispasr's `--prompt`/`--hotwords` are no-ops for Cohere too — so
voxi currently has no working vocabulary-biasing hook on either non-whisper
engine. Whisper's `initial_prompt` does work (`voxtype`), which is why small.en
handles this jargon better.

### Open follow-ups (not in this ticket)
1. **Empty-transcript bug**: reproduce `bug-d-etc` and `short-uh` directly
   against llama-server. Is it VAD, clip length, or the ASR prompt? Needs its
   own ticket.
2. **Vocabulary biasing** for openai-transcribe: llama-server takes a `prompt`
   field; check whether R2T2 honours it, and wire corpus keyterms if so.
3. **Accuracy comparison** against Cohere and small.en on this same corpus
   (131 option 2). `scripts/speech_context_bench` already reads corpus.tsv but
   is voxtype-only (`main.go:61`), the same gap as issue 133.

## 8. Probe of the §7 follow-ups (2026-09-22)

### A. The "empty transcripts" are a no-speech sentinel, not an empty string
Both clips return a filled envelope whose text is the bare scaffold (verified):
`{"type":"transcript.text.done","text":"language None<asr_text>",...}` — note
`language None`, and exactly 4 output tokens. Our `<asr_text>` marker strip then
yields `""`, which is why the live test reported "empty".

**Correction to the §7 wording and to the probe's own theory**: this is not a
duration effect. `short-yes` (0.92 s) transcribes fine, while `short-uh`
(1.06 s) does not. And `bug-d-etc.wav` is **0.64 s long while its corpus row
claims a 7-word sentence** — that row is mislabelled or its audio is truncated,
so it is not evidence of a model bug at all. What is left: R2T2 emits
`language None` for audio it judges to contain no lexical speech ("Uh!", a
truncated fragment). A `prompt` does not change it (byte-identical responses).

So the actionable part is on voxi's side: `language None<asr_text>` with no
content is a **distinct no-speech signal** and must not be conflated with a
successful empty transcript. In eager mode the difference decides whether to
type nothing quietly or to surface a failure.

### B. Keyterm biasing works, partially (verified)
`/v1/audio/transcriptions` does accept a `prompt` field, and llama.cpp uses it
as the literal ASR instruction (`tools/server/server-chat.cpp:653`, falling back
to `common_chat_get_asr_prompt` at :659-666).

Same clip, `kt-core.wav`, verbatim:
- no prompt: `...<asr_text>Box C uses box type with two tool on pipe wire and wayland.`
- prompt `Voxi|voxtype|dotool|PipeWire|Wayland`:
  `...<asr_text>Voxi uses voxtype with two tool on PipeWire and Wayland.`

Voxi, voxtype and PipeWire are all fixed; "dotool" -> "two tool" survives even
though it is in the keyterm list. So biasing is real but partial, and raw
pipe-separated terms may not be the best prompt shape.

This also closes a longer-standing gap: 074 §5 recorded that crispasr's
`--prompt` is a no-op for Cohere, so voxi had no vocabulary hook on any
non-whisper engine. The openai-transcribe engine now demonstrably has one.

## 9. Status (2026-09-22): M1-M3 done, engine is live

Landed: `89675af` (response_format + marker strip), `be78170` (schema test),
`1c736af` (per-model `base_url` + live corpus test). R2T2 is selectable and in
daily use by the user. Durable knowledge moved to `@docs/ASREngines.md`.

Follow-ups split out so this ticket can close: **137** (keyterm prompt biasing),
**138** (no-speech sentinel), **139** (systemd unit). Remaining here: nothing —
close once 139 makes the backend survive a reboot, or close now and let 139
stand alone.
