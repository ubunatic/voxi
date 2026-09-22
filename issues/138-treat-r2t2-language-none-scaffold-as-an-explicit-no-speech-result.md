# 138 — Treat R2T2 "language None" scaffold as an explicit no-speech result

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Bug
**Related**: 134 §8, `@docs/ASREngines.md`, `@docs/ChunkDiagnostics.md`

---

## 1. Problem

For audio it judges to contain no lexical speech, R2T2 returns a well-formed
envelope whose text is the bare scaffold (verified, 134 §8):

```
{"type":"transcript.text.done","text":"language None<asr_text>", ...}
```

Note `language None` and exactly 4 output tokens. After `strip_before_marker`
this becomes `""`, which voxi cannot distinguish from a failed or empty
transcription. In eager mode the two deserve different handling: a no-speech
chunk should be dropped quietly, a failure should surface.

Not a duration effect: `short-yes` (0.92 s) transcribes fine, `short-uh`
(1.06 s) returns the scaffold. A `prompt` does not change it.

## 2. /goal

The engine reports no-speech distinctly from an empty or failed transcript, and
`voxi chunks list` shows a no-speech reason rather than a `transcribe_error` or a
silent empty result. The rule is spec-driven (like `strip_before_marker`), not a
hardcoded R2T2 string in the engine, and a test covers scaffold-only input.

## 3. Notes

- Re-verify against current code; `strip_before_marker` handling lives in
  `internal/eager/openai_transcribe.go`.
- Check how the existing rejection reasons are modelled
  (`low_energy_transient` already exists in the chunk pipeline) and reuse that
  vocabulary instead of inventing a parallel one.
- Low priority: the audible effect today is a dropped "Uh", not lost sentences.
