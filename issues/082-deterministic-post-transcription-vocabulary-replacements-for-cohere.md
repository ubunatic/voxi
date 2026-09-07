# 082 — Deterministic Post-Transcription Vocabulary Replacements for Cohere

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Feature
**Related**: [032 small.en project vocabulary biasing](032-small-en-project-vocabulary-biasing.md), [038 vocabulary feedback command](038-vocabulary-feedback-command.md), [062 automatic vocabulary training research](062-fluidvoice-automatic-vocabulary-training-from-user-corrections-research.md), [074 Cohere backend integration](074-wire-cohere-transcribe-in-as-an-additional-selectable-asr-backend.md)

---

## 1. Problem & Motivation

Cohere Transcribe is accurate enough to be Voxi's default engine, but its
CrispASR backend does not support the preamble/initial-prompt vocabulary
biasing used by Whisper. It can therefore repeatedly miss uncommon product or
proper names even when the rest of the transcript is correct—for example,
emitting `Voxy` when the intended project name is `voxi`.

Voxi needs a deterministic, user-configurable correction layer that can map a
known recognition form to the intended spelling after transcription. This is
different from decoder biasing: it cannot help recognition itself, but it can
make recurring, narrowly defined corrections useful with Cohere today.

The behavior must be conservative. A naive substring replacement could corrupt
ordinary words, change unintended capitalization, or produce inconsistent
results across eager chunks.

## 2. Scope and Open Decisions

Explore and implement a post-transcription replacement dictionary, including a
clear decision on each of these boundaries:

- **Matching semantics**: define word- or phrase-boundary behavior so a rule
  such as `Voxy -> voxi` does not replace text inside a longer word. Specify
  punctuation, Unicode, whitespace, and multi-word handling.
- **Case semantics**: decide whether source matching is exact-case,
  case-insensitive, or configurable, and whether the replacement is always
  literal. Preserve intentional target spelling such as lowercase `voxi`.
- **Safety and precedence**: reject empty or overly broad rules, define
  duplicate/conflicting rule behavior, apply rules deterministically, and
  avoid recursive/cascading replacements unless explicitly designed.
- **Provider boundary**: decide whether corrections apply only to Cohere output
  or form a shared post-processing stage for all engines. Do not silently alter
  Whisper behavior merely because its existing vocabulary file is reused.
- **Configuration ownership**: decide whether mappings belong in a new
  user-owned config file, a spec-backed format, or an extension of `voxi
  feedback vocabulary`. Keep decoder vocabulary terms distinct from explicit
  heard-form-to-written-form mappings.
- **Streaming behavior**: identify the single point between completed ASR text
  and history/chunk storage, eager aggregation, and desktop typing where a
  correction must run. All downstream consumers should observe the same
  corrected text without retroactively editing text already committed from an
  earlier chunk.
- **Management UX**: provide discoverable add/list/remove operations and useful
  validation/errors, for example a way to add the mapping `Voxy -> voxi`.

Relevant implementation surfaces include `internal/eager`'s engine dispatch
and transcription worker, `internal/speechcontext`'s current prompt vocabulary,
`internal/feedback`'s persistent feedback commands, and `spec/models.yaml` if
provider policy is spec-owned.

## 3. Acceptance Criteria

- A user can persist, list, and remove an explicit replacement mapping such as
  `Voxy -> voxi` without manually editing implementation-owned data.
- A Cohere transcript containing the standalone recognition form is corrected
  before it is typed, aggregated, or stored in chunk/history metadata.
- Matching is word/phrase aware and does not replace an occurrence embedded in
  an unrelated longer word.
- Case handling, punctuation adjacency, whitespace, multiple rules, conflicts,
  and non-cascading behavior are specified and covered by table-driven tests.
- The provider scope is explicit and tested: either non-Cohere engines remain
  unchanged or shared behavior is intentionally documented and verified.
- Invalid, empty, duplicate, and ambiguous mappings fail safely with actionable
  errors; persistence is deterministic, atomic, and private to the user.
- Eager multi-chunk tests prove every downstream representation uses the same
  corrected text and that replacements do not introduce duplicate typing or
  cross-chunk corruption.
- User and developer documentation distinguish post-transcription correction
  from Whisper speech-context prompting and give a `Voxy -> voxi` example.
- `go test ./...`, `make check`, `make install`, and `git diff --check` pass.

## 4. Verification Guidance

- Unit-test the matcher independently with exact-word, embedded-word,
  punctuation, Unicode/case, phrase, overlap, and cascade cases.
- Test configuration parsing, normalization, file permissions, atomic writes,
  and command add/list/remove behavior using an isolated home directory.
- Exercise the Cohere worker with a fake CrispASR transcript containing `Voxy`
  and assert corrected output in the transcription event, chunk metadata,
  aggregate transcript, and typed-output seam.
- Add a negative engine-scope test and a multi-chunk eager test.
- When implementation is complete, run the repository-native checks and a
  safe manual canary that transcribes or injects controlled text without
  overwriting the user's existing mapping file.

## 5. Non-Goals

- Adding unsupported preamble, prompt, or hotword flags to Cohere/CrispASR.
- Fuzzy, phonetic, semantic, or model-based rewriting of arbitrary transcript
  text.
- Automatically learning mappings from system-wide edits in third-party
  Wayland applications; issue 062 covers the observability limitations.
- Silently correcting text without an explicit user-authored or accepted rule.
- Replacing the existing decoder-level speech-context vocabulary for engines
  that support it.
