# 040: Grammar-Constrained Decoding (GBNF) for Exact Technical Terms

**Status**: Blocked — Grammar Flag Not Available
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Feature
**Related**: [032 small.en vocabulary biasing](032-small-en-project-vocabulary-biasing.md), [039 OSS STT landscape research](039-oss-stt-landscape-and-custom-vocabulary-research.md)

---

## 1. Problem & Motivation

Issue 032 gave Voxi's eager `small.en` pipeline a soft biasing signal — an
`--initial-prompt` built from project vocabulary — which nudges the decoder
but does not guarantee exact recognition of technical terms (CLI flags,
package names, identifiers). Issue 039's research found that the installed
`whisper.cpp`/`voxtype` binary already ships a stronger, still-local, still
zero-new-dependency mechanism: **GBNF grammar-constrained decoding**
(`grammars/*.gbnf`), which penalizes/forbids tokens outside a defined grammar
rather than merely conditioning on a prompt. This is issue 039's first
recommendation and the lowest-risk upgrade path.

## 2. Desired Design

Add an opt-in **grammar mode** to Voxi's eager speech-context feature that
generates a permissive GBNF grammar from the same vocabulary source issue 032
already builds (explicit user/project vocabulary, static technical terms,
safe repository metadata), rather than replacing prompt biasing outright.

Constraints:

- The grammar must remain permissive enough to allow ordinary dictation
  (free-form English), using the vocabulary list only to bias/constrain
  matching of the specific known terms — not to restrict the transcript to a
  closed command grammar. Study `whisper.cpp`'s example grammars
  (`grammars/*.gbnf`, e.g. the JSON/chess examples) to confirm the permissive
  pattern is achievable before committing to production plumbing.
- Reuse the existing sanitized, capped, deduplicated term list and its
  priority ordering from issue 032's builder; do not duplicate vocabulary
  logic.
- Off by default, behind an opt-in flag distinct from (or layered on top of)
  `--speech-context`, until benchmarked.
- No change to `small.en` as the default model, no cloud dependency, no new
  runtime/binary — this only changes what is passed to the already-installed
  `voxtype`/`whisper.cpp` invocation.

## 3. Implementation Plan

1. Canary-first: hand-write one minimal permissive GBNF grammar containing a
   handful of Voxi terms (`Voxi`, `voxtype`, `dotool`, `PipeWire`) and confirm
   via `--grammar-file` (or the actual installed flag name — verify it) that
   `voxtype` accepts it and materially improves exact-term recognition on the
   same synthesized/JFK fixtures issue 032 already used, without breaking
   recognition of an unrelated ordinary sentence.
2. If the canary confirms real grammar-constrained decoding (not silently
   ignored), add a pure Go GBNF generator that renders the existing
   speech-context term list into a permissive grammar, with unit tests for
   escaping, empty/disabled output, and cap behavior.
3. Wire the generated grammar file path into the resolved `small.en` eager
   command alongside (or instead of) the initial-prompt, behind the opt-in
   flag.
4. Reuse issue 032's paired corpus/runner
   (`scripts/speech_context_bench/`, `testdata/speech-context/`) to compare
   three modes on the same fixtures: unprompted, prompt-only (032's current
   default), and grammar-constrained. Record WER, exact keyterm recall,
   latency, and hallucination/repetition count for all three.

## 4. Acceptance Criteria

- [x] A canary determined whether the installed `voxtype` binary honors a
  grammar file. **Result: it does not — see Section 6.**
- [ ] Grammar mode remains opt-in until the three-way benchmark shows a
  meaningful keyterm-recall improvement over prompt-only with no material
  general-WER, latency, or false-constraint regression. **Not reached — no
  grammar mechanism exists to benchmark.**
- [ ] No duplication of issue 032's vocabulary-building logic; the grammar
  generator consumes the same builder output. **Not reached — no generator
  was implemented per the ticket's stop condition.**
- [x] Canary environment and exact flag-name finding are recorded in this
  ticket before any implementation. **Recorded in Section 6; blocks further
  work until upstream `voxtype`/whisper.cpp exposes grammar support.**

## 5. Non-goals

- No closed/restrictive command grammar that would block free-form dictation.
- No engine swap — this stays within the existing whisper.cpp/voxtype binary.
- No LLM-based post-processing (tracked separately in issue 028).

## 6. Canary Results (2026-09-02) — Grammar Flag Not Honored/Not Present

The Section 3 plan called for confirming the exact grammar flag name via
`--help`/docs before writing any grammar generator or plumbing. That canary
was run first, per the ticket's stop condition, before any Go code was
written.

**Finding: `voxtype 0.7.5` exposes no grammar-constrained-decoding mechanism
at all — not `--grammar-file`, not any other name.** The ticket's guessed
flag name does not exist.

Evidence gathered:

1. `voxtype --help` (top-level and all ten subcommands: `daemon`, `setup`,
   `config`, `info`, `configure`, `status`, `record`, `meeting`,
   `check-update`, `transcribe`) lists no grammar-related option anywhere.
   The `Whisper:` section of the top-level help only offers
   `--initial-prompt <PROMPT>` (the mechanism issue 032 already uses),
   `--no-whisper-context-optimization`, `--flash-attention`,
   `--whisper-mode`, and remote-endpoint options.
2. `strings $(which voxtype) | grep -i grammar` and `grep -i gbnf` turn up
   **zero** occurrences of `--grammar-file` or `gbnf` anywhere in the binary.
   The only unrelated "grammar" hits are TOML-parser-internal error strings
   and one default-config comment about an *LLM-based* post-processing
   profile ("Fix grammar, remove filler words") — unrelated to whisper.cpp
   GBNF decoding.
3. `voxtype config` (resolved configuration dump) has no `grammar` key under
   `[whisper]` or anywhere else.
4. `voxtype` supports a `--whisper-mode cli` backend that shells out to a
   separate `whisper-cli` binary (real upstream whisper.cpp, which *does*
   support `-gr/--grammar` natively). However:
   - No `whisper-cli` binary is installed on this machine (`which whisper-cli`
     fails; none found under `~/.local/bin`, `/usr/local/bin`, `/usr/bin`, or
     `./build/bin/whisper-cli`).
   - Even if it were installed, `strings` shows voxtype's CLI-subprocess
     wrapper (`src/transcribe/cli.rs`) only ever forwards four flags to the
     `whisper-cli` child process: `--model`, `--language`, `--translate`,
     `--threads`. Grammar/GBNF is not in that forwarded set, so a grammar
     file could not reach whisper.cpp through voxtype even via the CLI
     backend.
   - The default and actually-used backend is `mode = "local"` (embedded
     `whisper-rs` bindings, not a subprocess), which has no code path to a
     grammar file at all.

Canary environment:

- `voxtype 0.7.5`, source install, `gpu-vulkan` feature, at
  `/home/uwe/.local/bin/voxtype`.
- AMD Ryzen 5 PRO 5650U (12 logical CPUs), Radeon/RADV Renoir Vulkan backend,
  Linux 7.1.9 x86_64 (same machine/binary as issue 032's canary).
- `[engine] engine = Whisper`, `[whisper] model = "small.en"`,
  `[whisper] mode` unset (defaults to local/embedded, not CLI subprocess).

No synthesized-audio transcription pass was needed to reach a conclusion:
there is no flag or config key to pass a grammar file to in the first place,
so no transcription runs could exercise grammar-constrained decoding. Per
the ticket's stop condition, this ticket stops here. **No Go GBNF generator,
no CLI wiring, and no three-way benchmark were implemented** — steps 2–4 of
the Implementation Plan are not applicable until an installed `voxtype`
version (or a documented `whisper-cli` passthrough) actually exposes grammar
support upstream. Revisit this ticket if a future `voxtype` release adds a
`--grammar-file`/`--grammar` option or forwards it through the CLI backend.
