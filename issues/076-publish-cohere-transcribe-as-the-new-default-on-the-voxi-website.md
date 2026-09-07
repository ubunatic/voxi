# 076 — Publish Cohere Transcribe as the New Default on the Voxi Website

**Status**: Implemented — synced to publishing repo; public deployment awaits publishing-repo commit/push
**Priority**: P1 (High)
**Severity**: Minor
**Category**: Documentation
**Related**: [001 Website integration](001-website-integration.md), [075 documentation and product messaging](075-align-documentation-and-product-messaging-with-cohere-transcribe-as-the-default-asr.md), commit `4159109`

---

## 1. Problem & Motivation

Voxi now uses CPU-local Cohere Transcribe 03-2026 as its default ASR, but the
public website still presents the product primarily as local Whisper and shows
`small.en` as the default. That stale messaging hides an important product
improvement and gives visitors inaccurate expectations about installation,
model selection, downloads, and privacy.

Issue 075 covers repository-wide documentation consistency and allows the
website source to be corrected, but deliberately does not authorize publishing.
This dedicated follow-up owns the website-focused implementation and explicitly
includes publication and deployed-page verification.

## 2. Scope

Update `website/` so the landing page positively and accurately advertises
Cohere Transcribe 03-2026 as Voxi's new default ASR:

- Revise the hero copy and badge so Cohere is prominent and Whisper is not
  presented as the current default.
- Update terminal demos and model examples to use the default naturally;
  retain Whisper models as selectable alternatives where useful.
- Correct hallucination-filtering and backend-specific copy so features are
  attributed accurately and Whisper-only behavior is not implied for Cohere.
- Add `crispasr` to installation/dependency guidance for the default backend.
- Update model and `--model` help shown on the site so `small.en` is no longer
  labeled as the default.
- Explain that first use lazily downloads an approximately 1.66 GiB Q5_0 GGUF
  from the ungated `cstr/cohere-transcribe-03-2026-GGUF` Hugging Face community
  mirror into `~/.cache/voxi/models/`, after which inference runs locally on
  the CPU.
- Phrase privacy precisely: audio is not sent to a cloud transcription
  service, but uncached model weights require a first-run network download.
  Avoid an unqualified "zero cloud dependencies" claim.
- Search all website source for related stale `Whisper`, `small.en`,
  dependency, default-model, download, and privacy wording rather than treating
  the known locations as exhaustive.

Keep the page static and consistent with the established Voxi visual language.
Follow `docs/Website.md` if that project guide exists at implementation time;
it was not present when this ticket was filed.

## 3. Acceptance Criteria

- The page visibly identifies Cohere Transcribe 03-2026 as Voxi's new default
  and describes its quality advantage without unsupported benchmark or
  universal-accuracy claims.
- Hero text, badges, terminal examples, hallucination-filtering copy,
  dependency information, and model help agree with the actual default in
  `spec/models.yaml`.
- `crispasr`, lazy download size/source/cache location, CPU-local inference,
  offline-after-cache behavior, and the network requirement for an uncached
  model are explained accurately.
- Whisper remains documented as a selectable alternative, not as the default.
- Privacy wording distinguishes local transcription from model distribution.
- The page remains responsive and keyboard-accessible; semantic structure,
  focus states, contrast, reduced-motion behavior, and internal/external links
  are checked on narrow and wide viewports.
- No CDN or dynamic-service dependency is introduced; the static-site
  constraints and existing asset/link conventions remain intact.
- The updated website is published with `uman website sync voxi`, and the
  deployed Voxi page is opened and verified for content, assets, links, and
  responsive presentation.

## 4. Implementation & Verification Plan

1. Confirm current behavior and values against `spec/models.yaml`, backend
   code, and issue 075 before editing marketing copy.
2. Update the website source and inspect repository-local diffs for accidental
   changes.
3. Run the project's applicable static-site, markup, link, and asset checks;
   manually exercise narrow and wide layouts plus keyboard navigation.
4. Run `make install` as required by the repository after changes. No service
   restart is expected for website-only source edits.
5. Publish with `uman website sync voxi`.
6. Verify the deployed page rather than treating a successful sync command as
   sufficient evidence, recording the public URL and verification result when
   closing this issue.

This ticket authorizes the website sync only after the source update and local
verification are complete. It does not broaden scope to unrelated website
redesign or implementation of issue 001's interactive demos and packaging
goals.

## 5. Implementation Notes (2026-09-07)

- Updated `website/index.html` so the hero, terminal example, monitor example,
  backend filtering copy, prerequisites, and CLI model help consistently name
  Cohere Transcribe 03-2026 as the default. Whisper remains documented as an
  alternative and for legacy modes.
- Added a "Why Cohere Transcribe?" section grounded in issue 066's local canary
  results. Added precise first-run behavior: the approximately 1.66 GiB Q5_0
  GGUF comes from the ungated `cstr/cohere-transcribe-03-2026-GGUF` community
  mirror, is cached in `~/.cache/voxi/models/`, and then runs CPU-local and
  offline without uploading audio for transcription.
- Added `crispasr` to prerequisites and linked the actual CrispASR and Voxtype
  upstream projects. `voxtype` is not called optional yet because issue 077
  tracks removing the current eager-startup dependency.
- Updated `website/index.css` with explicit `:focus-visible` outlines and a
  `prefers-reduced-motion` fallback. No media or dynamic/CDN dependency was
  introduced.
- Verification: custom HTML audit (unique IDs, fragments, local assets, and
  button labels), `node --check website/index.js`, `git diff --check`, stale
  default-message search, HTTP 200 checks for every external destination,
  and Chromium renders at 1440x1200 and 390x844. `make install` succeeded.
- Ran `uman website sync voxi`; all post-sync package, asset, REUSE, LFS, and
  shared-navigation checks passed. The synced output is present in
  `/home/uwe/projects/ubunatic.com/voxi/`.
- Deployment boundary: `https://ubunatic.com/voxi/` returns HTTP 200, but still
  serves the prior Whisper-default page because the publishing repository now
  has uncommitted `voxi/index.html` and `voxi/index.css` changes. This ticket
  explicitly authorized sync, not a cross-repository commit or push, so those
  actions were not taken. Close the ticket after that publication step and a
  second live-page content/render check.
