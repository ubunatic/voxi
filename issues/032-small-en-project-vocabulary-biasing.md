# 032: Small.en Project Vocabulary Biasing for Technical Dictation

**Status**: Implemented / Default-on — Measurement Gate Closed, default flipped in [046](046-speech-context-default-on.md)  
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
- Vocabulary context is on by default for `small.en`, now that Section 7.2's
  benchmark showed a meaningful keyterm-recall gain (0.35 -> 0.90) with no
  material general-WER (0.259 -> 0.095) or latency regression (~2% median);
  it stayed off by default until that measurement gate closed. Explicit
  `--speech-context=false` restores the unprompted path (see issue 046).
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

Persistent terms can also be managed without editing the file directly:

```sh
voxi feedback vocabulary add TLDR
voxi feedback vocabulary list
voxi feedback vocabulary remove TLDR
```

This feedback vocabulary supplies initial-prompt hints only. It does not store
error variants, fuzzy-match transcripts, or post-correct recognized text.

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

### 7.1 First real-microphone data point (2026-09-02, via issue 042)

Issue 042 shipped `voxi feedback sample record` for building a private,
real-microphone corpus at `~/.config/voxi/samples/`, corpus.tsv-compatible
with this gate's runner. The user recorded three ad hoc samples
(`hello-voxi-thinkpad`, `hello-voxi-webcam`, `my-toolchain`) and ran:

```sh
go run ./scripts/speech_context_bench -corpus ~/.config/voxi/samples
```

Result: mean WER 0.40 unprompted vs. 0.10 prompted across the three fixtures;
keyterm recall read 0/0 because none of the three phrases happened to contain
an exact static-vocabulary keyterm (see issue 042 Section 7 for the full
per-fixture table). This is real signal that `--speech-context` prompting
helps on genuine microphone audio, not just synthesized eSpeak NG fixtures —
but it is not yet the gate this section calls for: three ad hoc phrases are a
small, keyterm-sparse sample, not the deliberately keyterm-dense fixture set
this gate was written for.

**Still open**: record a corpus specifically designed to exercise the
technical vocabulary (Voxi, voxtype, dotool, PipeWire, Wayland, file/package
names, etc., matching `testdata/speech-context/corpus.tsv`'s phrasing style)
via `voxi feedback sample record`, then re-run the bench and record the
aggregate here before considering opt-out or default-on behavior.

### 7.2 Keyterm-dense corpus result (2026-09-02, via issue 044) — Gate Closed

The user recorded the 10 phrases proposed in issue 044 (8 technical, 2
ordinary control) with `voxi feedback sample record`. Two rows in the
generated `~/.config/voxi/samples/corpus.tsv` were missing their trailing
tab for the (empty) keyterms field — a data-entry artifact, not a code bug —
and every row's keyterms column was empty (the recorder does not infer
keyterms, only text), so both were fixed by hand before benching, populating
keyterms per fixture from the static vocabulary terms actually present in
each phrase (e.g. `Voxi|voxtype|dotool|PipeWire|Wayland` for `kt-core`).

```sh
go run ./scripts/speech_context_bench -corpus ~/.config/voxi/samples
```

Aggregate over all 13 fixtures (10 from issue 044 plus the 3 from issue 042's
first pass):

| | mean WER | keyterm recall | median latency | adjacent repeats |
|---|---|---|---|---|
| Unprompted | 0.259 | 0.35 (7/20) | 1703 ms | 0 |
| Prompted | 0.095 | 0.90 (18/20) | 1733 ms | 0 |

Prompting more than doubles keyterm recall (0.35 → 0.90) and roughly halves
mean WER (0.26 → 0.09), with a ~30 ms (1.8%) median latency delta and zero
hallucination/repetition regressions in either mode. Per-fixture detail:
unprompted runs consistently mangled brand/technical terms into
phonetically-similar ordinary words (`Voxi` → `Foxy`/`Voxy`, `voxtype` →
`FoxType`, `PipeWire` → `Pyfire`, `Wayland` → `Waydend`, `systemd` → `system
D`), which the prompted runs corrected in every case except `kt-config`
(`models.yaml`/`context_test.go` stayed partially wrong in both modes — the
weakest fixture, likely because file-name-shaped terms don't tokenize the
same way as single words) and `kt-mixed-2` (prompted recovered `Voxi` but not
`voxtype`, transcribed as `VoxType`).

**Gate closed**: this meets the acceptance criteria in Section 4 — meaningful
keyterm-recall gain, no material general-WER or latency regression. `voxi
eager --speech-context` remains a documented opt-in flag rather than
switching to default-on in this ticket; a separate follow-up would be needed
to change the default given this is now a P2, not urgent, decision.
