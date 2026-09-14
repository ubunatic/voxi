# Voxi Roadmap

Reconciled from the active issue backlog on 2026-09-11 (previous passes:
2026-09-10, 2026-09-05). This is a communication artifact, not a scheduling
tool; `issues/README.md` remains the authoritative tracker.

## Value axis

Voxi earns its place as a daily driver by making local Wayland dictation feel
immediate and **trustworthy**: the microphone starts and stops predictably,
speech becomes accurate text quickly, every word the user actually spoke gets
typed exactly once, and nothing else ever reaches the keyboard. Trust is the
binding constraint — a single runaway injection, a silently dropped utterance,
or a silent typing failure costs more user confidence than a dozen milliseconds
of latency ever buys back.

Three properties define that axis, in order:

1. **Injection safety**, which this pass splits into two distinct halves now
   that they have diverged in maturity:
   - *Exactly-once delivery* — no stale or duplicated text, physical modifier
     gating actually active, stop means stop without eating the last utterance.
     Largely **closed** by 083 §9/§10 (durable ledger, bounded final drain).
   - *Only the user's speech* — nothing the user did not say is ever accepted.
     Still **open**, and now the leading edge of the axis (100, 096).
2. **Failure visibility** — when the pipeline breaks, the user finds out where
   they are looking, not in a private JSONL file. Closed by 080.
3. **Responsiveness and accuracy under real load** — measured, not felt.

Presentation, distribution, and internal-hygiene work rank below all three,
with one exception: documentation that *misstates what the product does* is a
trust problem, not a presentation problem, and is sequenced accordingly.

The center of gravity moved again this pass. The 2026-09-10 roadmap was about
*closing the delivery contract*; three slices (`a65a623`, `df6af2b`, plus the
review-gap and drain fixes) have now done most of that. What remains on the
trust axis is almost entirely **false acceptance**: audio that was never the
user speaking becoming confident, well-formed, correctly-delivered text.

## Now — stop accepting speech the user did not produce

- **[100 background-voice false acceptance](../issues/100-background-distant-voice-hallucinated-into-accepted-transcripts-bypassing-silence-gate.md)**
  (In Progress, P1). Promoted to the head of `Now` this pass, ahead of 083.
  Rationale: 083's remaining work is residual hardening of a contract that now
  demonstrably holds, while 100 is an *open hole* — chunk #1533's
  `"Aye, you did that."` is neither repetitive nor duplicated, so neither the
  repetition limits, the delivery ledger, nor the new `repeated_sentence_pair`
  check from §7 can touch it. Half of it landed (`26fc3cf`: adjacent duplicated
  complete sentence pairs rejected before typing, covering the hospital
  hallucination shape). The §7 canary also produced a hard negative result that
  should be respected, not re-litigated: the fixtures' energy overlaps ordinary
  background recordings, so a bare RMS or voiced-ratio threshold would reject
  legitimate quiet speech. That makes this ticket **dependent on 096**, which
  is why 096 moves into `Now` with it.
- **[096 acoustic classification research](../issues/096-spectral-centroid-keyboard-clack-vs-speech-classification-research.md)**
  (Research In Progress, P2). Moved up from `Next`. It is no longer a
  supporting track for 056's noise-rejection gate — it is now the blocking
  research for 100's accept/reject signal. Three single-feature hypotheses are
  falsified (spectral centroid, ZCR, and transient-shape's inapplicability to
  sustained noise), and §6 names harmonicity / pitch salience as the most
  promising untested direction because it targets a property that should hold
  across percussive *and* sustained noise: voiced speech has a periodic
  fundamental, keyboard/mouse/motor/kitchen noise does not. §6 also flags that
  the bar for "hand-crafted features are good enough" keeps rising at 28
  samples with three falsifications — that trade-off decision belongs in this
  bucket, taken deliberately, not deferred a fourth time.
- **[083 injection-safety residue](../issues/083-prevent-runaway-repeated-dotool-desktop-injection.md)**
  (In Progress, P1). Demoted within `Now` — still `Now`, no longer first. §9
  shipped the durable at-most-once ledger (file-locked, fsynced, survives
  daemon restarts, `delivery_duplicate` telemetry) and the injector attempt
  boundary; §10 shipped the final-Super-X drain that makes "stop capture and
  flush" mean what the daily workflow expects, with generation eligibility
  checked both before the durable claim and before injection. Its original
  five-second wall-clock lease was replaced in issue 115 by an explicit
  generation-superseded signal, after the lease was confirmed to drop fully
  transcribed speech twice in production; see
  [EagerDeliverySafety.md](EagerDeliverySafety.md). The §8 warning still stands for anyone touching this code: the
  design must keep distinguishing "the last thing the user said, right up to
  stop" from "a stale or pathological late result." What is explicitly *not*
  claimed and remains open: emergency stop, exhaustive FIFO/standalone injector
  process-lifecycle canaries, and the documented crash window between the
  durable claim and injector submission. All three are bounded, and the crash
  window is the only one with a real user-visible failure mode.

## Next — make responsiveness and the product story evidence-based

- **[075 align documentation and messaging with Cohere as the default ASR](../issues/075-align-documentation-and-product-messaging-with-cohere-transcribe-as-the-default-asr.md)**
  (Open, P1). Scope has shrunk substantially: §5 records that README,
  `docs/VoiceInput.md`, telemetry guidance, root CLI help, the GNOME extension
  label, model-registry comments/schema descriptions, and website copy now
  describe Cohere Transcribe as the default, distinguish the one-time ~1.66 GiB
  local weight download from cloud transcription, and state that vocabulary
  prompting applies to Whisper alternatives rather than the default path. What
  is left is a repository-wide wording audit for stale legacy-architecture and
  historical notes, plus publishing the updated website source (it was changed
  but never synced). Keeping it at the head of `Next` rather than closing it:
  an unpublished website update means the public-facing surface still tells the
  old story, and that is the half of this ticket that actually reaches users.
  It remains the gate on 001.
- **[088 elevate OS scheduling priority for the transcription critical path](../issues/088-elevate-os-scheduling-priority-for-the-transcription-critical-path.md)**
  (Open, P3). §8 closed the question honestly: **no scheduler change is
  justified yet.** The live service runs at `Nice=0`/`LimitNICE=0`, and
  existing telemetry has latency values but no CPU/GPU-load marker or priority
  correlation to tune against. The named next step is a controlled
  idle-versus-loaded canary with fixed utterances and telemetry/process
  snapshots, testing `CPUWeight` before attempting negative nice values. This
  is the right shape and the right order; it stays in `Next` precisely because
  the measurement, not the mechanism, is the work. Starting it from the
  original *felt-slowness* report instead of numbers is how it becomes a
  permanent research ticket.
- **[056 remaining stress-session phases](../issues/056-end-to-end-stress-session-testing-with-noise-and-load.md)**
  (In Progress, P3). §9 narrowed this to one concrete blocker. The de-flaking
  work landed at the shared safety boundary — `asr.IsSafeToType` now rejects
  punctuation-only decodes, and the Phase 2 matcher requires distinct accepted
  chunks so one chunk cannot satisfy both WER checks. But the gated real
  pipeline test was never run: it needs `VOXI_E2E=1`, private WAV fixtures, and
  the Whisper runtime. **One live hardware run is all that stands between
  Phase 2 and done** — that is a small, well-defined action, not open-ended
  work, and it should be taken before the deferred CPU/GPU contention phases.
  Those contention phases remain the validation vehicle for whatever 088
  recommends, not a parallel performance project with its own measurements.
- **[037 code quality, coverage, and modularization](../issues/037-code-quality-and-test-coverage-roadmap.md)**
  (Open, P3), scoped to the Eager decomposition and focused CLI/audio/ASR
  tests. The argument for waiting has now partly expired in Voxi's favour:
  `internal/eager` has absorbed the ledger, generation eligibility, the drain
  lease, and the stop-boundary timestamp, all covered by tests that pin the
  observable behavior down. Decompose *after* 083's residue lands, so the
  refactor is not chasing a file that is still changing under it — but the
  window is opening, and `docs/EagerDeliverySafety.md` now gives the extraction
  a written contract to preserve. GNOME modularization stays parked.

## Later — distribution, spec hygiene, optional UI polish

- **[001 website integration and public documentation](../issues/001-website-integration.md)**
  (In Progress). Interactive demos, packaging recipes (RPM/deb/PKGBUILD), and
  GNOME/PipeWire setup guides help adoption but change nothing about the
  reliability of the current user's daily dictation path. Resume after 075's
  website publish, so the expanded site tells the Cohere-default story rather
  than propagating the Whisper one further.
- **[090 spec drift: `monitor -w` section aliases hardcoded](../issues/090-spec-drift-monitor-w-section-flag-aliases-hardcoded-separately-from-spec-actions-yaml.md)**
  (Open, P3). `ParseSections` shadows `spec/actions.yaml` with a hand-written
  `switch`; the fix is to resolve against `spec.LoadActions()`'s `Action.Short`
  and add a test asserting every accepted alias is derivable from the loaded
  spec. Small, unambiguous, no open questions — good filler alongside a larger
  ticket, and the accompanying test is what actually prevents recurrence.
- **[095 spoken number normalization](../issues/095-normalize-spoken-number-words-to-digits-in-dictated-transcripts-library-vs-build-our-own.md)**
  (Open, P3). Real dictation-quality value, but the ticket's own framing is a
  library-versus-hand-rolled evaluation with three unvetted Go candidates, and
  `docs/Go.md` says avoid unneeded deps. Cardinal number conversion is a
  well-bounded algorithm; the honest expectation is that the hand-rolled
  version wins and the evaluation is short. Ranked here rather than `Next`
  only because it is an accuracy *improvement*, not an accuracy *defect*.
- **[099 corpus manifest replacement](../issues/099-replace-corpus-tsv-sample-manifest-with-a-more-robust-storage-format.md)**
  (Open, P3) and **[102 notification language packs](../issues/102-multi-language-audio-packs-for-the-modifier-release-notification-clip.md)**
  (Open, P3). Developer-experience and polish respectively; neither should
  displace false-acceptance work. 099 gains urgency only if 096/100's growing
  sample set makes the TSV manifest actively painful — watch for that signal.
- **[024 GNOME typing-feedback icon](../issues/024-gnome-typing-feedback-icon.md)**
  and **[025 GNOME volume/VU-meter animation](../issues/025-voice-input-volume-animation.md)**
  (Open, P4). Unchanged: low value while the GNOME Shell extension is not in
  daily use and the OS recording indicator suffices. 025 is also partially
  overtaken — 084's live loudness meter delivered the equivalent capability in
  `voxi monitor`, so re-scope 025 against what already exists before starting.

## Close / Park

- **[091 JSON Schemas are never validated against](../issues/091-spec-system-json-schemas-in-spec-schemas-are-never-actually-validated-against-validate-spec-only-runs-go-test.md)**
  (Open, P2) — **decide, don't schedule.** Moved out of `Later` into this
  section because it is not really an implementation ticket: §3 offers two
  resolutions, and one of them is *free*. Either add a pure-Go JSON Schema
  validator to `validate-spec`, or — if a new dependency is unwanted, which
  `docs/Go.md` suggests it is — reword `docs/Spec.md` to state plainly that the
  schemas are IDE-only hints rather than CI-enforced invariants. The second
  option closes the ticket in one edit and removes a doc that over-promises.
  Make that call rather than carrying it as backlog.
- **[097 external public-domain speech sample catalog](../issues/097-external-public-domain-speech-sample-catalog-download-on-demand-not-committed.md)**
  (Open, P3) — **park, subordinate to 056/096.** §6 is explicit: no download
  mechanism was added, and source licensing, checksums, cache policy, and
  transcript-format alignment must all be resolved before implementation. Those
  are unanswered external/design questions, not work. Revive it only when
  096's classifier work actually runs out of locally-recorded samples — right
  now the corpus is growing fine from real recordings, which are better
  fixtures anyway because they match this user's actual acoustic environment.
- **037 Work Item 3 (GNOME extension modularization)** — noted parked directly
  in the ticket (037 stays otherwise open: Items 1/2/4 are real, untouched
  work). Parked alongside 024 and 025 until extension usage resumes.
- ~~040 grammar-constrained decoding (GBNF)~~ and ~~052 CPU/GPU priority
  research~~ — both closed in prior passes (040 doubly blocked upstream; 052
  folded into 088 §7). Retained here only so the merge is not rediscovered.

## Shipped since the 2026-09-10 pass

- **080 surface eager typing/transcription failures** closed (`023e080`). The
  prior pass's second `Now` item: hard pipeline failures — including the live
  "transcribed correctly, typed nothing, `dotool` missing, no signal anywhere"
  session — are now surfaced where a user actually looks.
- **092 `eagerSessionManager.Toggle` check-then-act race** closed (`8a9ec18`),
  serialized concurrent toggles. Fixed inside the 083 pass exactly as the prior
  roadmap predicted would be cheapest.
- **103 queued utterance lost on stop** closed (`b9ea358`, `70c8607`,
  `d9a093d`, `df6af2b`). The queued-job half of the stop/delivery contract,
  resolved together with 083 §10's bounded drain rather than separately.
- **083 §9 durable at-most-once delivery** (`a65a623`) and **§10 bounded
  final-Super-X drain** (`df6af2b`, after the chunk `#52`/`#53` reproduction),
  with the contract written up in
  [`EagerDeliverySafety.md`](EagerDeliverySafety.md) and a sprint retrospective
  recorded. Ticket stays In Progress for the residue listed in `Now`.
- **100 §7 partial safety slice** (`26fc3cf`): adjacent duplicated complete
  sentences are rejected before typing with an explicit
  `repeated_sentence_pair` reason, deliberately requiring three-plus words per
  sentence so short legitimate answers are not eaten.
- **Feedback replacement dictionary overhaul** (`da7c132`, `b61b7a9`) — not
  tracked by any ticket, recorded here so it is not invisible. Replacements now
  match case-insensitively and re-case their target to how the phrase was
  actually heard (lower/UPPER/Title), so one rule replaces the whole family of
  case-variant entries; `--fixed-case` opts out for things like domain names;
  and `voxi feedback replacement cleanup [--dry-run]` merges the now-redundant
  duplicates while flagging genuine target conflicts instead of silently
  discarding them. This is direct dictation-accuracy value on the main axis.
- **Install-path consolidation** (`59115ba`, `7a860fe`): a single binary in
  `~/.local/bin` with a `~/go/bin` symlink, fixing the systemd user service's
  view of the installed binary.
- **075 §5 documentation alignment** — most product surfaces now tell the
  Cohere-default story; see `Next` for the remaining audit and website publish.
- Everything recorded as shipped in the 2026-09-10 pass (061, 084, 087, 089,
  093, 094, 098, 101, 104, and the earlier 081/082/085/086 group) remains
  shipped and is not repeated here.

## Reconciliation notes

- **Value axis split, not replaced.** "Injection safety" was one property; it
  is now two, because exactly-once delivery and only-the-user's-speech have
  diverged sharply in maturity. The first is close to closed by mechanism; the
  second has no working mechanism yet and is now the leading edge.
- **Reprioritized: 100 above 083.** A contract that demonstrably holds and
  needs hardening ranks below an acceptance hole with a known, reproducible
  failing fixture. This is the main sequencing change this pass.
- **Reprioritized: 096 from `Next` to `Now`.** Its role changed from supporting
  056's noise gate to blocking 100's accept/reject signal — and 100's §7 canary
  independently proved the cheap alternatives (RMS, voiced ratio) unsafe,
  which is what elevates the research from optional to on the critical path.
- **Reprioritized: 037 softened.** Not moved buckets, but the "wait until
  behavior is pinned by tests" condition it was gated on is now nearly met.
- **Moved to Close/Park: 091** — a decision, not a build, with a free closing
  option — **and 097**, blocked on unresolved external licensing/format
  questions and not currently needed by the work it was meant to serve.
- **Out of the roadmap entirely**: 080, 092, and 103, all closed and recorded
  under Shipped rather than dropped. They were the prior pass's `Now` bucket.
- **Deliberately not force-fit into a bucket**: 091 and 097, per above.
