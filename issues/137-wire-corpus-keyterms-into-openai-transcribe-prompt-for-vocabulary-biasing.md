# 137 — Wire corpus keyterms into openai-transcribe prompt for vocabulary biasing

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Feature
**Related**: 134 §8 (probe evidence), 074 §5 (crispasr --prompt is a no-op for Cohere), `@docs/ASREngines.md`

---

## 1. Problem

R2T2 mangles project vocabulary in live dictation: Voxi -> "Foxy"/"voxey",
voxtype -> "box type", PipeWire -> "pipe wire", harnez -> "harness",
uman -> "human". Whisper avoids this via voxtype's `initial_prompt`; the
`openai-transcribe` engine sends no biasing hint at all.

Verified in 134 §8: `/v1/audio/transcriptions` accepts a `prompt` field which
llama.cpp uses as the literal ASR instruction
(`~/.cache/voxi/llama.cpp/tools/server/server-chat.cpp:653`). Passing
`Voxi|voxtype|dotool|PipeWire|Wayland` on `kt-core.wav` corrected Voxi, voxtype
and PipeWire in one request. "dotool" -> "two tool" survived, so biasing is
partial and the prompt shape matters.

## 2. /goal

The `openai-transcribe` engine sends a vocabulary prompt, so voxi's own jargon
transcribes correctly in live dictation. The term list is spec- or
config-driven, not hardcoded in Go, and an engine that ignores `prompt` is
unaffected. A test asserts the prompt reaches the request body.

## 3. Notes / Uncertainties

- Re-verify against current code first (`internal/eager/openai_transcribe.go`).
- **Source of terms is open**: `~/.config/voxi/samples/corpus.tsv` has a
  `keyterms` column, but that is a *test* corpus, not a runtime config. There is
  also `~/.config/voxi/vocabulary.txt`. Decide which is the runtime source —
  vocabulary.txt looks right; corpus keyterms belong to accuracy measurement.
- **Prompt shape is open**: bare pipe-separated terms left "dotool" wrong. Try a
  wrapping instruction ("Use these exact terms if heard: …") and measure. Do not
  guess — the corpus makes this measurable.
- Watch the token cost: the prompt precedes every request and counts as input
  tokens on every utterance.
