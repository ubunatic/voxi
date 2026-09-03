# 046: Make Speech-Context Vocabulary Prompting the Default Behavior

**Status**: Implemented
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Enhancement
**Related**: [032 small.en vocabulary biasing](032-small-en-project-vocabulary-biasing.md) (Section 7.2 measurement gate, now closed), [044 keyterm-dense sample recording follow-up](044-keyterm-dense-sample-recording-followup.md)

---

## 1. Problem & Motivation

Issue 032 introduced Whisper initial-prompt vocabulary biasing behind an
opt-in flag (`voxi eager --speech-context`), deliberately withheld from
default-on until a real measurement gate was met. That gate closed in
Section 7.2 (2026-09-02): across a 13-fixture real-microphone corpus,
prompting raised keyterm recall from 0.35 to 0.90 and cut mean WER from
0.259 to 0.095, with only a ~2% median latency cost and zero
hallucination/repetition regressions in either mode.

Right now a new user setting up Voxi for the first time gets the weaker,
unprompted behavior unless they already know the `--speech-context` flag
exists — the opposite of what the measured data supports. The feature that
actually helps technical dictation accuracy should be the default a
first-time user experiences, not an opt-in they have to discover.

## 2. Desired Design

Flip the default for the eager transcription path: speech-context prompting
is **on by default**, with an explicit opt-out (`--speech-context=false` or
equivalent) for anyone who wants the old unprompted behavior back. No change
to the underlying mechanism (initial-prompt construction, term sources,
caps, sanitization) — only the default value of the flag/config changes.

Constraints carried over unchanged from issue 032:

- `small.en` remains the default model, entirely local.
- The prompt still obeys the term/character caps and sanitization rules.
- No behavior change for non-eager commands, other models, typing, or
  history.

## 3. Implementation Plan

1. Flip the default in the Cobra flag/config definition for
   `--speech-context` (and any `voxi config`/spec default it reads from).
2. Update any first-run/onboarding messaging or `voxi config` summary output
   so a first-time user setting up Voxi sees that speech-context prompting
   is active by default and how to disable or extend it
   (`voxi feedback vocabulary add/list/remove`,
   `~/.config/voxi/vocabulary.txt`).
3. Update `README`/docs and issue 032's Section 4 acceptance criteria to
   reflect the new default.
4. Regression-test that explicit `--speech-context=false` still fully
   disables prompting (existing tests from issue 032 should already cover
   the disabled path — verify, don't rewrite, unless the flag's polarity
   changes required test updates).

## 4. Acceptance Criteria

- A first-time `voxi` setup transcribes with speech-context prompting active
  without the user having to pass any flag.
- `--speech-context=false` (or equivalent) still fully restores the
  unprompted path, unit-tested.
- Docs/help text reflect the new default and explain how to disable or
  extend the vocabulary.
- `go test ./...`, `make check`, `make install` pass.

## 5. Non-goals

- No change to the prompt-construction algorithm, term sources, caps, or
  sanitization from issue 032.
- No new UI/onboarding wizard beyond updating existing help/docs text.
- No model or backend change.

## 6. Implemented Behavior (2026-09-02)

Only the default value flipped; the prompt-construction algorithm, term
sources, caps, and sanitization from issue 032 are untouched.

- `internal/eager/eager.go`: `DefaultEagerOptions()` now sets
  `SpeechContext: true` (was the implicit zero value `false`). This is the
  single source of the default — `cmd/voxi/main.go`'s
  `eagerCmd.Flags().BoolVar(&eagerOpts.SpeechContext, "speech-context",
  eagerOpts.SpeechContext, ...)` binds the Cobra flag's default to whatever
  `DefaultEagerOptions()` returns, so the flag now defaults to `true`. A
  fresh `voxi eager` (default model `small.en`, per `spec/models.yaml`'s
  `default_model`) runs with speech-context prompting active; explicit
  `--speech-context=false` still fully restores the unprompted path via the
  unchanged `shouldUseSpeechContext(modelName, opts.SpeechContext)` gate in
  `internal/eager/eager.go`.
- `cmd/voxi/main.go`: `--speech-context` flag help text changed from
  "opt in to bounded local vocabulary hints for small.en" to "bounded local
  vocabulary hints for small.en (default: on; use --speech-context=false to
  disable)".
- `internal/feedback/command.go`: `voxi feedback status` now calls
  `BuildSummary(..., true)` instead of a hardcoded `false`, with an updated
  comment explaining this reports the built-in default (there is no
  persisted override for the flag). `voxi feedback vocabulary add`'s
  confirmation message now says the term is active by default on the next
  `voxi eager` run and how to disable prompting, instead of telling the user
  to opt in with `--speech-context`.
- `internal/feedback/summary.go`: the `voxi feedback status` output line and
  `VocabularySources` doc comment now describe speech-context as on by
  default for `small.en`, disableable per invocation with
  `--speech-context=false`, instead of "opt-in per eager invocation".
- `spec/models.go` / `spec/models.yaml`: doc comments on `SpeechContextSpec`
  and the `speech_context` YAML block updated from "opt-in" to "active by
  default (disable per invocation with --speech-context=false)". No schema
  or data field changed.
- `issues/032-small-en-project-vocabulary-biasing.md`: Status line updated to
  "Implemented / Default-on — Measurement Gate Closed, default flipped in
  046"; Section 4's "off by default until..." criterion rewritten to state
  the on-by-default outcome and cite the Section 7.2 numbers (recall
  0.35->0.90, mean WER 0.259->0.095, ~2% median latency cost). Section 6's
  historical canary narrative (which still correctly describes the
  now-superseded "off by default" canary-era decision) was left untouched
  per the ticket's instruction not to rewrite history sections.
- `internal/eager/eager_test.go`: renamed
  `TestSpeechContextIsOptInAndSmallEnOnly` to
  `TestSpeechContextIsSmallEnOnlyAndDisableable` (same table, same
  assertions — `shouldUseSpeechContext` itself is unaffected by the default
  flip since it only gates on the caller-supplied `enabled` bool) and added
  `TestDefaultEagerOptionsEnableSpeechContext`, which asserts
  `DefaultEagerOptions().SpeechContext == true` and that the default model
  passes `shouldUseSpeechContext`. This is new coverage: no prior test
  asserted on `DefaultEagerOptions()`'s `SpeechContext` value, so the
  default-value plumbing itself was previously untested.
- Not touched, per the ticket's constraints: `internal/devsample` /
  `voxi feedback sample` (issue 045's recorder keeps speech-context
  prompting off by design, independent of this default), and the
  prompt-construction algorithm/term sources/caps/sanitization in
  `internal/speechcontext`.
- `README.md` was checked; it does not currently mention `--speech-context`
  or `voxi eager`'s flag list, so there was no stale "opt-in" claim there to
  fix.

### Verification

- `go build ./...` — pass, no output.
- `go vet ./...` — pass, no output.
- `go test ./...` — all packages pass, including the new/renamed
  `internal/eager` tests and unchanged `internal/feedback` summary tests
  (which pass their `SpeechContextDefaultOn` bool explicitly in every case,
  so were unaffected by the default flip).
- `make check` (`go vet ./...` + `go test ./...` + `go test ./spec/...`) —
  pass.
- `make install` — rebuilt and installed `voxi` successfully.
- `make restart-service` — rebuilt, installed, and ran
  `systemctl --user restart voxi-agent.service`; `systemctl --user status
  voxi-agent.service` confirmed `Active: active (running)` immediately
  after, so the live daemon is running the new default-on binary.
