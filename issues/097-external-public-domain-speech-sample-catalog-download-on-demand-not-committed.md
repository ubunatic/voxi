# 097 — External public-domain speech sample catalog (download-on-demand, not committed)

**Status**: Open
**Priority**: P3 (Low)
**Severity**: Enhancement
**Category**: ASR Quality / Dev Tooling
**Related**: [042 private dev sample recorder](042-private-dev-sample-recorder.md) (`~/.config/voxi/samples`,
private/local-only, never committed), [096 spectral-centroid research](096-spectral-centroid-keyboard-clack-vs-speech-classification-research.md)
(introduced `voxi feedback sample promote` and `testdata/noise-samples`, the public git-lfs corpus
this ticket must stay distinct from), [internal/devsample/sample.go](../internal/devsample/sample.go)
(`SamplesDir`, `PublicSamplesDir`, manifest helpers), [internal/devsample/flow.go](../internal/devsample/flow.go)
(`Promote`), [testdata/speech-context/corpus.tsv](../testdata/speech-context/corpus.tsv) (existing
corpus.tsv format/convention), `scripts/speech_context_bench` (existing `-corpus` flag pointing at a
directory of `corpus.tsv` + WAVs)

---

## 1. Problem & Motivation

Voxi's dev-sample system now has two corpora, each with a clear, deliberate boundary:

- **Private** (`~/.config/voxi/samples`, `internal/devsample.SamplesDir`): the developer's own real
  speech, never committed, explicitly documented as sensitive.
- **Public/promoted** (`testdata/noise-samples/`, `internal/devsample.PublicSamplesDir`): recordings
  confirmed to contain *no* real speech (keyboard clacks, mouse noise, ambient noise), promoted via
  `voxi feedback sample promote` and committed to the repo via git-lfs, per issue 096.

Both corpora are bounded by one developer's own voice, environment, and recording sessions. Every
accuracy canary that needs genuine *speech* (as opposed to noise) — small.en vocabulary biasing
(032), grammar-constrained decoding (040), sherpa-onnx hotwords (041), keyterm-dense recording
(044) — has so far relied on either synthesized eSpeak NG audio or this one person's real
recordings. There is no broader, more diverse, real-human-speech corpus available to Voxi's canary
tooling.

The user wants a **third** corpus category: speech samples sourced from public-domain audio
(e.g. LibriVox, Mozilla Common Voice, other public-domain speech datasets), downloaded on demand
rather than committed to the repo, and kept structurally separate from both the private personal
corpus and the promoted public-repo corpus so the three are never mixed.

This ticket is a proposal/research ticket — the design is not settled. It records the shape of the
problem, the existing conventions this should build on, and the open questions to resolve before
implementation, rather than prescribing a full design up front.

## 2. Constraints Established by Existing Conventions

- **corpus.tsv format**: `testdata/speech-context/corpus.tsv` and `internal/devsample`'s
  `manifestFile` both use a shared TSV shape (`id`, `wav file`, `expected transcript`, `keyterms`).
  Any new external-corpus manifest should stay compatible with this so `scripts/speech_context_bench`'s
  existing `-corpus <dir>` flag (currently pointed at `testdata/speech-context` or, per
  `internal/devsample`'s doc comment, potentially `SamplesDir` unmodified) can also point at an
  external-corpus directory without new bench code.
- **Three-way separation, not two**: `SamplesDir` (private) and `PublicSamplesDir` (public/promoted,
  git-lfs-tracked) are both real, committed-code concepts today. A third directory/helper (e.g.
  `ExternalSamplesDir`) should follow the same doc-comment-driven, explicit style — clearly stating
  in code comments that this directory holds *downloaded* third-party audio, is gitignored (not
  git-lfs-tracked like `PublicSamplesDir`), and must never be populated by `promote` or mixed with
  either of the other two.
- **Nothing gets committed here**: unlike `testdata/noise-samples/` (committed via git-lfs) and
  unlike `testdata/speech-context/corpus.tsv` (text manifest committed, WAVs gitignored, per issue
  042 §1), external samples' *audio* should not be committed at all — only a manifest/catalog
  (source URLs, licenses, checksums) belongs in the repo, with a download step fetching audio into
  a gitignored local directory on demand.
- **CLI integration point**: `voxi feedback sample` already has `record/list/play/remove/save-chunk/
  promote`. A new subcommand family (e.g. `voxi feedback sample fetch-external` /
  `external list/download`) is the natural extension point per `internal/feedback/command.go`'s
  existing wiring, but the exact command shape is an open question (§4).

## 3. Candidate Public-Domain Speech Sources (unvetted)

Not yet evaluated for license terms, audio quality, per-utterance transcript alignment, or download
mechanics — evaluate before committing to any:

- **LibriVox** (public-domain audiobook recordings, volunteer-read). Long-form audio; would need
  chunking/alignment to short utterances, and per-reader accent/quality varies widely.
- **Mozilla Common Voice** (CC0-licensed short utterance clips with transcripts, purpose-built for
  ASR training/eval — likely the closest fit to the existing corpus.tsv utterance-level shape).
  Requires checking current download/dataset access mechanics (may require a dataset export/API
  rather than a stable direct-download URL).
- Other public-domain or permissively-licensed speech datasets (e.g. LibriSpeech, which is derived
  from LibriVox and already utterance-segmented with transcripts — potentially a better fit than raw
  LibriVox for this exact use case; VoxForge; other CC0/public-domain corpora) — worth a broader
  survey pass before picking one.

## 4. Open Questions (unresolved — do not invent answers, resolve during design)

1. **Which source(s)** best match Voxi's actual need (short, transcript-aligned utterances, ideally
   with some diversity of accent/mic quality) — Common Voice and/or LibriSpeech look like stronger
   candidates than raw LibriVox on first look, but this needs a real evaluation pass, not a guess.
2. **License/attribution tracking**: how to record per-sample source, license (CC0 vs. other
   permissive terms), and attribution requirements in the catalog manifest, and whether any
   candidate source actually requires attribution (CC0 does not; others might).
3. **Download mechanism**: a `voxi feedback sample` subcommand vs. a standalone `scripts/` tool
   (matching `scripts/clack_features`' and `scripts/speech_context_bench`'s existing precedent of
   standalone `go run ./scripts/...` tools) vs. a plain shell/Makefile target — evaluate against how
   `scripts/speech_context_bench` is already invoked before picking.
4. **Directory/manifest shape**: a new `internal/devsample.ExternalSamplesDir` (gitignored, sibling
   to `SamplesDir`/`PublicSamplesDir`) holding downloaded audio, with a *committed* catalog manifest
   (source metadata, not audio) living in `testdata/` or similar — needs a concrete proposal, not
   just this ticket's sketch.
5. **Bench integration**: whether `scripts/speech_context_bench -corpus <external-dir>` should work
   unmodified once samples are downloaded and a corpus.tsv-compatible manifest is generated, or
   whether external samples need their own bench entry point given they may arrive pre-segmented
   with their own transcript format that needs converting to corpus.tsv shape.
6. **Cache/refresh policy**: whether downloaded samples are cached indefinitely once fetched, how a
   developer clears/refreshes them, and whether a fixed subset (vs. a large bulk download) is the
   right default to keep this a fast, low-friction dev convenience rather than a heavyweight dataset
   pull.

## 5. Non-Goals (for this ticket)

- Not a replacement for the private (`SamplesDir`) or promoted-public (`PublicSamplesDir`) corpora —
  additive, for broader speech diversity than one developer's own voice provides.
- Not committing any downloaded third-party audio to the repo, under any circumstance — only a
  catalog/manifest of sources, licenses, and download instructions.
- Not designing the full implementation here — this ticket exists to scope the problem and record
  open questions; a follow-up ticket (or a reopened/expanded version of this one) should carry the
  concrete design once §4's questions are answered.
