# 075 — Align Documentation and Product Messaging with Cohere Transcribe as the Default ASR

**Status**: Open
**Priority**: P1 (High)
**Severity**: Minor
**Category**: Documentation
**Related**: [066 Cohere/Nemotron canary](066-canary-cohere-transcribe-and-nemotron-3-5-streaming-as-alternative-asr-backends.md), [074 Cohere backend integration](074-wire-cohere-transcribe-in-as-an-additional-selectable-asr-backend.md), commits `0581620`, `9789a68`, `46a0c68`

---

## 1. Problem & Motivation

Voxi has adopted CPU-based Cohere Transcribe 03-2026 as its default ASR
model. Commit `0581620` added it as a selectable backend, `9789a68` changed
`spec/models.yaml`'s `default_model` to `cohere-transcribe-03-2026`, and
`46a0c68` stabilized live use by forcing English after observed language
drift. Live dictation now demonstrates strong transcription quality,
including technical terms and German words embedded in English speech.

The implementation and canonical model spec reflect that product decision,
but Voxi's public and operational documentation still largely presents
Whisper/`small.en` as the default. This creates misleading installation,
privacy, resource, feature, and product expectations at exactly the point
where Cohere should become a visible reason to try Voxi.

This is high priority despite being low-severity documentation drift: the
README, website, and CLI are Voxi's primary product surfaces. They must both
advertise the improved new default and state its operational tradeoffs
accurately.

## 2. Required Documentation and Messaging Changes

Establish one consistent product story across repository documentation,
runtime help, comments, and the website:

- **Default and positioning**: prominently identify Cohere Transcribe
  03-2026 as Voxi's new default ASR model and describe the quality motivation
  without making unsupported benchmark or universal accuracy claims.
- **Local inference and privacy**: retain the promise that transcription is
  local and audio is not sent to a cloud transcription service, while
  clarifying that first use requires a network download of model weights.
  Avoid the unqualified phrase "zero cloud dependencies" where it would imply
  that no network access is ever needed.
- **Runtime and resource expectations**: document `crispasr` as a required
  runtime for the default backend; first use lazily downloads an approximately
  1.66 GiB Q5_0 GGUF from the ungated `cstr/cohere-transcribe-03-2026-GGUF`
  community mirror on Hugging Face into `~/.cache/voxi/models/`; subsequent
  inference runs locally and is currently CPU-based.
- **Choice remains**: explain that existing Whisper models remain selectable
  alternatives/fallbacks through the existing model option rather than
  implying that Whisper support was removed.
- **Feature boundary**: make clear that `small.en` speech-context vocabulary
  prompting does not apply to Cohere. CrispASR's prompt/hotword options are
  no-ops for this backend, so docs must not promise project-vocabulary biasing
  for the default model.
- **Language behavior**: reflect the current English-forced Cohere invocation
  where language behavior is documented; do not imply that recognition of
  occasional embedded non-English words means general multilingual mode is
  enabled.

Update at least the following known stale surfaces, and search the repository
for equivalent Whisper-only/default-model wording so this list is not treated
as exhaustive:

1. `README.md`: product promise/hero copy, eager-mode description,
   dependencies and setup, example model selection, model/default guidance,
   download/cache behavior, and privacy wording (known stale areas near lines
   5, 12, 40, 134, and 157 at filing time).
2. `docs/VoiceInput.md`: current status, architecture/backend description,
   eager pipeline, model registry/selection, download behavior, defaults,
   speech-context limitations, and troubleshooting/setup.
3. `website/index.html`: hero and marketing copy, demo/example commands,
   dependency table, model help/default claims, privacy/download language,
   and any Whisper-only presentation. Update website source as part of this
   issue, but do **not** run `uman website sync voxi` unless separately asked
   to publish.
4. `cmd/voxi/main.go`: eager command long help and `--model` flag help so
   generated/runtime CLI documentation no longer calls the pipeline or model
   set Whisper-only.
5. `docs/TelemetryGuide.md`: replace unnecessarily engine-specific Whisper
   event wording with accurate ASR/transcription terminology, retaining event
   compatibility unless a code change is genuinely required.
6. `contrib/gnome-shell-extension/extension.js`: replace "Continuous
   Whisper" or equivalent stale eager-mode UI descriptions.
7. `internal/eager/eager.go`: correct the stale comment that identifies
   Whisper as today's default.
8. `spec/models.go` and `spec/schemas/models.schema.json`: correct comments
   and descriptions that frame the model registry as Whisper-only. Preserve
   `spec/models.yaml` as the single source of truth; do not duplicate model
   values into Go code or prose-derived logic.
9. Issue 074's implementation note: correct the historical note that falsely
   says the default remains `small.en`, while preserving the fact that default
   promotion was outside 074's original implementation scope.

## 3. Acceptance Criteria

- README and website visibly promote Cohere Transcribe 03-2026 as Voxi's new
  default; examples use the default naturally and show Whisper only where an
  explicit alternative is useful.
- Installation/setup instructions make the `crispasr` requirement actionable
  and accurately explain the lazy ~1.66 GiB download, source, cache location,
  CPU execution, offline behavior after caching, and failure expectations when
  weights are absent and the network is unavailable.
- Privacy claims consistently distinguish local transcription from the
  one-time network model download.
- User-facing and developer documentation consistently distinguishes Cohere's
  behavior from Whisper-only speech-context prompting.
- CLI help, GNOME extension copy, spec/schema descriptions, implementation
  comments, telemetry documentation, and issue 074 no longer assert or imply
  that `small.en`/Whisper is the current default.
- A repository-wide search for `small.en`, `Whisper`, `whisper`, `voxtype`,
  `default`, and cloud/local claims has been reviewed; remaining occurrences
  are either accurate historical references or explicit selectable-alternative
  documentation.
- Website source is updated but not published/synced as part of this work.

## 4. Verification and Delivery

1. Validate links, command examples, cache paths, binary names, model names,
   download source/size, and claims against the implementation and
   `spec/models.yaml` rather than copying stale prose.
2. Run documentation/spec/CLI-help tests and generation checks that apply to
   the files changed, plus `go test ./...` and `make check` if Go or schema
   descriptions are modified.
3. Exercise `voxi eager --help` (and any generated completion/help assertions)
   to confirm the installed wording is coherent and does not regress flag
   behavior.
4. Per repository convention, run `make install` after changes. If work goes
   beyond documentation/help/comments and changes code used by the live eager
   daemon, run `make restart-service` instead so the active service loads it.
5. Do not publish the website during implementation; leave
   `uman website sync voxi` for an explicitly authorized release step.

## 5. Sprint Progress (2026-09-10)

The main product surfaces now describe Cohere Transcribe as the default:
README, `docs/VoiceInput.md`, telemetry guidance, root CLI help, the GNOME
extension label, model-registry comments/schema descriptions, and website copy
were aligned. They also distinguish the one-time local weight download from
cloud transcription and state that vocabulary prompting applies to Whisper
alternatives, not the default Cohere path.

The issue remains open for a final repository-wide wording audit and any stale
legacy-architecture or historical notes not covered by this pass. Website
source was updated; it was not published or synced.
