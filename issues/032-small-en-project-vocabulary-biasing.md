# 032: Small.en Project Vocabulary Biasing for Technical Dictation

**Status**: Open  
**Priority**: P2 (Medium)  
**Severity**: Moderate  
**Category**: Feature  
**Related**: [031 Claude Code voice-pipeline research](031-claude-code-and-agent-cli-voice-pipeline-research.md), [models specification](../spec/models.yaml)

---

## 1. Problem & Motivation

Voxi's default eager model is the local English Whisper model `small.en`.
It is fast enough for live dictation but has no knowledge of the active
repository's vocabulary, so it can misrecognise product names, CLI commands,
file names, programming-language terms, and people/project names.

Claude Code's shipped voice client demonstrates a useful non-LLM technique:
send a small, bounded set of project-derived keyterms *before* transcription.
Voxi should investigate the corresponding local Whisper mechanism—an initial
prompt—without replacing text after decoding, sending audio to a cloud service,
or changing the default `small.en` model.

## 2. Desired Design

Create a Voxi-owned **speech context** for each eager transcription session.
It is an ordered, deduplicated list of terms, rendered as a short natural
language Whisper initial prompt, for example:

> Technical dictation. Terms: Voxi, voxtype, dotool, PipeWire, Wayland, Cobra,
> systemd, `models.yaml`.

Sources, in descending priority:

1. Explicit user/project vocabulary, configured in a Voxi-owned file or flag.
2. A small static technical vocabulary maintained in a YAML specification.
3. Safe repository metadata: repository basename, recognized programming
   languages/tools, and basenames of recently edited tracked files.

Constraints:

- Default cap: 50 terms and a strict character/token budget suitable for
  Whisper's prompt context. Prefer explicit terms when the cap is reached.
- Do not read or transmit file contents, git remotes, environment variables,
  untracked paths, credentials, or history entries.
- Terms must be plain display names/basenames only; strip paths, control
  characters, duplicates, and overlong items.
- The prompt changes decoder context only. Voxi must type the ASR result
  verbatim after its existing hallucination filtering—no local LLM rewrite,
  fuzzy replacement, or command execution.
- Preserve the current behaviour when speech context is disabled or empty.

## 3. Implementation Plan

1. Canary-first: verify the exact `voxtype`/Whisper initial-prompt flag and
   behaviour with a one-utterance fixture before adding production plumbing.
2. Add a spec-backed context vocabulary and a pure Go builder with unit tests
   for priority, cap, deduplication, sanitisation, and disabled/empty output.
3. Pass the rendered prompt only to the `small.en` eager transcription command;
   retain the flag behind an opt-in configuration until accuracy is measured.
4. Add a fixture corpus with matched ordinary dictation and technical phrases:
   Voxi/voxtype/dotool, filenames, shell commands, Go identifiers, and user
   names. Store text expectations only; do not commit private recordings.
5. Compare prompted and unprompted `small.en` on the same recorded corpus and
   representative hardware. Record word error rate, exact keyterm recall,
   median final-transcript latency, and hallucination/repetition count.

## 4. Acceptance Criteria

- `small.en` is still the default model and remains entirely local.
- Vocabulary context is off by default until the canary proves support and the
  benchmark shows a meaningful keyterm-recall gain with no material general-WER
  or latency regression.
- The generated prompt obeys the term and size limits and contains no file
  contents or secrets.
- Tests cover deterministic construction and sanitisation; a manual canary
  validates that a recognized term is improved rather than post-corrected.
- Findings, hardware, model/backend, and both benchmark outputs are recorded
  in this ticket before changing the default.

## 5. Non-goals

- No LLM-based post-processing or semantic rewriting (tracked separately in
  issue 028).
- No cloud transcription or network dependency.
- No model switch to `large-v3-turbo`, Parakeet, or another ASR engine.
