# 032: Small.en Project Vocabulary Biasing for Technical Dictation

**Status**: Implemented / Opt-in Canary  
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

## 6. Implemented Canary Slice (2026-09-01)

The installed `voxtype 0.7.5` exposes the global Whisper option
`--initial-prompt <PROMPT>`. A one-utterance synthesized canary established
that this is real decoder conditioning, not post-correction:

| Run | Transcript |
|---|---|
| Unprompted | `Moxie uses voice type with doo-to-lon pipe, wire, and wayland.` |
| Prompted | `Voxi uses voice type with dotool and PipeWire and Wayland.` |

The source phrase was generated locally with eSpeak NG 1.52.0 and converted to
16 kHz mono PCM. The prompt was `Technical dictation. Terms: Voxi, voxtype,
dotool, PipeWire, Wayland.` A public 11-second JFK reference clip produced the
same transcript in both modes; one smoke run measured 1.46 seconds unprompted
and 1.47 seconds prompted. These two probes establish backend support and a
promising keyterm effect, but are not a representative accuracy benchmark.

Canary environment:

- AMD Ryzen 5 PRO 5650U (12 logical CPUs), Radeon/RADV Renoir Vulkan backend.
- Linux 7.1.9 x86_64, `voxtype 0.7.5`, Whisper `small.en`.
- Model SHA-256:
  `c6138d6d58ecc8322097e0f987c32f1be8bb0a18532a3f88f734d1bbf9c41e5d`.

The production slice is deliberately off by default. Enable it with
`voxi eager --speech-context`; additional highest-priority terms can be passed
with `--vocabulary Voxi,dotool` or placed one per line in
`~/.config/voxi/vocabulary.txt`. Only the resolved `small.en` eager command
receives the prompt. Other models, disabled/empty context, transcript cleaning,
typing, and history retain their existing behavior.

The builder enforces the spec-owned 50-term, 400-character, and 64-character
per-term limits; sanitizes controls and paths to basenames; deduplicates without
case; and prioritizes explicit, shipped, then repository-derived terms. Local
repository discovery reads only the repository basename and modification times
and basenames of tracked files. It does not read file contents, remotes, Git
history, environment variables, or untracked files. A daemon whose working
directory is not inside a repository simply receives no repository-derived
terms.

The text-only corpus and paired runner live under
`testdata/speech-context/` and `scripts/speech_context_bench/`. Private WAVs are
Git-ignored. The runner reports paired transcript, WER, exact keyterm recall,
median latency, and adjacent repetition count.

## 7. Remaining Measurement Gate

Before considering opt-out or default-on behavior, record the three local WAV
fixtures with a representative speaker/microphone and run:

```sh
go run ./scripts/speech_context_bench -corpus testdata/speech-context
```

Record the paired aggregate output here. Keep the feature opt-in unless the
full corpus shows meaningful keyterm-recall gain without material general-WER,
latency, hallucination, or repetition regression.
