# 077 — Make voxtype Optional and Remove Whisper-Only Engine Assumptions

**Status**: Open
**Priority**: P1 (High)
**Severity**: Major
**Category**: Bug
**Related**: [074 Cohere backend integration](074-wire-cohere-transcribe-in-as-an-additional-selectable-asr-backend.md), [075 Cohere default documentation](075-align-documentation-and-product-messaging-with-cohere-transcribe-as-the-default-asr.md), [076 Cohere website publication](076-publish-cohere-transcribe-as-the-new-default-on-the-voxi-website.md), commits `0581620`, `9789a68`

---

## 1. Problem & Motivation

Cohere Transcribe via `crispasr` is now Voxi's default eager ASR backend, but
the eager startup paths still require `voxtype` before resolving and
dispatching the selected model engine. A fresh default installation therefore
fails when `voxtype` is absent even though the Cohere path never invokes it.
This makes an obsolete Whisper runtime dependency block the primary product
path and contradicts the new default's installation story.

The same Whisper-only assumption appears in adjacent developer tools. They can
load the Cohere default or enumerate the full model registry while always
invoking `voxtype`, producing misleading failures or invalid benchmark/debug
behavior. Dependency checks and subprocess dispatch must follow the resolved
engine everywhere they coexist.

`voxtype` remains supported and genuinely required for explicit Whisper
models, legacy batch and Parakeet streaming modes, and their related tooling.
This issue makes it optional for the default Cohere eager path; it does not
remove Whisper or legacy modes.

## 2. Findings and Required Changes

### Eager startup

- `internal/eager/eager.go`'s direct `RunEagerDictation` path currently calls
  `LookPath("voxtype")` unconditionally around lines 179–182, before model and
  engine dispatch around lines 270–346.
- The daemon startup path repeats the unconditional lookup around lines
  848–851. Consequently a fresh-default `voxi-agent` also fails without
  `voxtype`, despite Cohere dispatch later selecting `crispasr`.
- Resolve the model and its engine first in both paths. Require `crispasr` and
  Cohere weights only for `engine: cohere-transcribe`; require `voxtype` only
  for `engine: whisper`. Prefer one shared resolver/check so direct and daemon
  behavior cannot drift.
- Preserve precise, actionable errors: explicit Whisper selection without
  `voxtype` must name the missing executable and selected backend; Cohere
  failures must identify `crispasr`, missing weights, or download failure as
  appropriate.

### Adjacent engine assumptions

- `internal/devsample/transcribe.go` loads the default model but always invokes
  `voxtype`.
- The debug VAD probe likewise loads the default while hard-coding `voxtype`.
- `internal/bench` defaults to all registry models but always invokes
  `voxtype`, so the Cohere registry entry is handled incorrectly.
- Make these surfaces engine-aware where that matches their purpose, or
  explicitly constrain and label them Whisper-only. They must never silently
  pass a Cohere model name to `voxtype`.

### Dependency and product messaging

Update installation, help, and dependency documentation so `crispasr` is the
runtime required by default eager dictation and `voxtype` is an optional
dependency for explicit Whisper models and legacy batch/streaming workflows.
Coordinate wording with issues 075 and 076 rather than duplicating or
contradicting their broader Cohere-default messaging.

Document the important transitions: persisted configurations using batch or
streaming still need the relevant `voxtype` services; explicitly choosing a
Whisper model still needs the binary; and first-time Cohere use may download
approximately 1.66 GiB of weights before becoming offline-capable.

## 3. Acceptance Criteria

- Default Cohere `voxi eager` reaches readiness when `voxtype` lookup fails but
  `crispasr` and valid cached weights are available.
- Default Cohere `voxi-agent` reaches readiness under the same conditions.
- Direct, daemon, and agent-level automated tests cover the missing-`voxtype`
  default path; tests must prove readiness rather than merely testing an
  argument builder.
- Explicit selection of every supported Whisper model still dispatches through
  `voxtype`, and a missing binary produces a precise backend-specific error.
- Cohere dispatch still checks for `crispasr` and weights, retaining clear
  first-download/offline failure behavior.
- Model/engine resolution and dependency validation are shared where practical
  so the direct and daemon entry points apply equivalent rules.
- `internal/devsample`, the debug VAD probe, and `internal/bench` either
  dispatch according to the model engine or reject/omit unsupported engines
  explicitly with accurate user-facing labels and tests.
- Legacy batch and Parakeet streaming behavior is unchanged and their
  `voxtype` requirement remains explicit.
- README, operational docs, CLI help, and website dependency copy touched by
  issues 075/076 consistently describe `voxtype` as optional for
  Whisper/batch/streaming and `crispasr` as required for default eager mode.

## 4. Verification and Delivery

1. Add dependency-lookup test seams or fakes as needed; do not make tests rely
   on whichever executables happen to be installed on the developer machine.
2. Run focused tests for `internal/eager`, `internal/bench`,
   `internal/devsample`, the debug probe, and agent/command startup, followed
   by `go test ./...` and `make check`.
3. Exercise the installed default Cohere eager path with `voxtype` deliberately
   unavailable, and separately verify the explicit-Whisper missing-binary
   diagnostic.
4. Run `make restart-service`, not only `make install`, because the live
   `voxi-agent.service` executes the eager/agent code changed by this issue.
   Confirm the restarted service is active and perform one live default-model
   dictation.
5. Recheck the installation and dependency claims coordinated through issues
   075 and 076. Website publication remains owned by issue 076.

## 5. Non-Goals

- Removing Whisper, batch mode, Parakeet streaming, or `voxtype` support.
- Changing the default away from Cohere Transcribe.
- Redesigning Cohere weight distribution, the approximately 1.66 GiB initial
  download, or vocabulary prompting beyond documenting their current behavior.
- Publishing the website outside issue 076's authorized workflow.
