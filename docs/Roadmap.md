# Voxi Roadmap

Reconciled from the active issue backlog on 2026-09-09 (previous pass:
2026-09-05). This is a communication artifact, not a scheduling tool;
`issues/README.md` remains the authoritative tracker.

## Value axis

Voxi earns its place as a daily driver by making local Wayland dictation feel
immediate and **trustworthy**: the microphone starts and stops predictably,
speech becomes accurate text quickly, every word the user actually spoke gets
typed exactly once, and nothing else ever reaches the keyboard. Trust is the
binding constraint — a single runaway injection, a silently dropped utterance,
or a silent typing failure costs more user confidence than a dozen milliseconds
of latency ever buys back.

Three properties define that axis, in order:

1. **Injection safety** — no pathological, stale, or duplicated text is ever
   injected; physical modifier gating is actually active; stopping means stopped.
2. **Failure visibility** — when the pipeline breaks, the user finds out where
   they are looking, not in a private JSONL file.
3. **Responsiveness and accuracy under real load** — measured, not felt.

Presentation, distribution, and internal-hygiene work rank below all three,
with one exception: documentation that *misstates what the product does* is a
trust problem, not a presentation problem, and is sequenced accordingly.

The measurement substrate the previous roadmap was chasing now exists (060 and
061 shipped), so this pass shifts the center of gravity from *building
instrumentation* to *closing the trust contract* it revealed.

## Now — close the injection-safety and failure-visibility contract

- **[083 reject pathological repetitive ASR output before injection](../issues/083-prevent-runaway-repeated-dotool-desktop-injection.md)**
  (In Progress, P1/Critical). The single highest-value open item: it is the
  ticket standing directly on top of the trust axis. The §7 slice shipped the
  immediate boundary (spec-owned output/token/repetition limits, the retained
  `Ubuntuktuktuktuk` regression with zero captured injector calls, session-derived
  cancellation), but §8 records that the first cut of "no flush after stop" was
  too broad and silently dropped the trailing utterance on *every* normal stop —
  fixed in `8795182`, and a standing warning that the remaining work is delicate.
  What is left is the part that makes the guarantee durable rather than
  incidental: a cross-session delivery ledger giving each accepted transcript an
  at-most-once identity, injector attempt/process telemetry, and FIFO/standalone
  process-lifecycle canaries. Sequenced first because the design must distinguish
  "the last thing the user said, right up to stop" from "a stale or pathological
  late result" — collapsing the two reintroduces the §8 regression.
- **[080 surface eager typing/transcription failures](../issues/080-surface-eager-typing-transcription-failures-beyond-private-telemetry.md)**
  (Open, P0/Critical). Small scope, disproportionate value. A live session
  transcribed correctly and typed nothing — with `dotool` missing — and produced
  no signal in the console, in `journalctl --user -u voxi-agent.service`, or
  anywhere else a user would look. For a dictation tool this is close to the
  worst failure mode: apparent success, no output, no clue. The fix is bounded
  (establish the daemon-mode logging convention `internal/eager` currently
  lacks, then surface `typeErr`), and it makes every other failure in this
  bucket diagnosable by the user instead of by an agent running telemetry
  queries. Take it alongside 083 — both touch the same acceptance/typing seam.
- **[089 `voxi-modifierd` not installed (modifier gating inactive)](../issues/089-voxi-modifierd-not-installed-on-this-dev-machine-modifier-gating-currently-inactive.md)**
  (Open, P2, operational). Cheapest trust win on the board: one `sudo make
  install-modifierd` plus a live re-verify. The headline safety mechanism the
  README and website advertise is currently *not running* on the development
  machine, and `mods: off` is plainly visible in the published
  `website/voxi-demo.mp4`. Do it now, then act on its second half — decide
  whether the quickstart's "(Optional)" framing undersells a step that is the
  actual safety guarantee behind a headline feature.
- **[092 `eagerSessionManager.Toggle` check-then-act race](../issues/092-eagersessionmanager-toggle-has-a-check-then-act-race-under-concurrent-sigusr1-socket-invocation.md)**
  (Open, P2). Included in Now because it is small, well-localized, and sits in
  exactly the session-lifecycle code 083 is already rewriting — fixing it as
  part of that pass is much cheaper than fixing it later against a changed
  file. User-visible symptom (two near-simultaneous toggles silently no-op the
  dictation start, while both report "Recording started") is a lifecycle
  predictability bug, i.e. the same axis as 083.

## Next — make responsiveness and the product story evidence-based

- **[075 align documentation and messaging with Cohere as the default ASR](../issues/075-align-documentation-and-product-messaging-with-cohere-transcribe-as-the-default-asr.md)**
  (Open, P1). Promoted above the performance work this pass. `spec/models.yaml`
  now defaults to `cohere-transcribe-03-2026`, but the docs still largely tell
  a Whisper/`small.en` story, and "zero cloud dependencies" is now imprecise
  given the ~1.66 GiB first-use weight download. Misstating the runtime
  requirements, privacy boundary, and the fact that vocabulary prompting is a
  no-op for the default backend sets false expectations at first contact — a
  trust cost, not a polish item. It is also the natural gate on 001: the
  website should not be expanded until it is telling the right story. Mostly
  writing, low risk, no dependency on the Now bucket.
- ~~087 `voxi monitor` lights the GNOME mic-in-use indicator while idle~~ —
  **closed**, re-verified resolved. `82b0ee6` added GNOME suppression tags to
  `PwRecordCommand`, closing the gap this ticket reported; see "Shipped"
  below.
- **[088 elevate OS scheduling priority for the transcription critical path](../issues/088-elevate-os-scheduling-priority-for-the-transcription-critical-path.md)**
  (Open, P3). 052's research questions and canary probes were folded into
  088 §7 and 052 closed as a duplicate track (same launch path, same
  question — neither the agent service nor the `exec.CommandContext` ASR
  launch applies any `Nice=`, `CPUWeight=`, `SCHED_*`, or cgroup weight
  today). The motivating report is *felt* slowness — 088 explicitly records a
  subjective single-session report with no timing numbers, under load partly
  self-inflicted by screen recording plus the monitor's own meter. With
  060/061 shipped there is now no excuse for tuning by feel: reproduce the
  "screencast + `voxi monitor -w` + dictation" scenario, get idle-versus-loaded
  stage latencies out of `voxi telemetry query`, and only then choose a
  mechanism from §4. Ordered after 075 because it is open-ended measurement
  work, and starting it without the numbers is how it becomes a permanent
  research ticket.
- **[056 remaining stress-session phases](../issues/056-end-to-end-stress-session-testing-with-noise-and-load.md)**
  (In Progress, P3). Two distinct pieces of leftover work. First, make the
  Phase 2 noise-rejection assertion deterministic without moving production
  defaults — a known-flaky gate is worse than no gate. Second, the deferred
  CPU/GPU contention phases, which should be the validation vehicle for
  whatever 052/088 recommends rather than a parallel performance project with
  its own measurements. The harness has already earned its keep (it caught a
  genuine, previously-unknown Whisper outro-hallucination variant on its first
  independent run), which is why it stays active rather than being parked.
- **[037 code quality, coverage, and modularization](../issues/037-code-quality-and-test-coverage-roadmap.md)**
  (Open, P3), scoped initially to the Eager decomposition and focused CLI/audio/
  ASR tests. Unchanged reasoning from the last pass, reinforced by this one:
  083, 080, and 092 all land in `internal/eager`, and 083's §8 regression is
  precisely the kind of defect that unclear lifecycle boundaries produce.
  Extract those boundaries *after* the Now bucket has pinned the observable
  behavior down with tests, not before. GNOME modularization stays parked.

## Later — distribution, spec hygiene, optional UI polish

- **[001 website integration and public documentation](../issues/001-website-integration.md)**
  (In Progress). Interactive demos, packaging recipes (RPM/deb/PKGBUILD), and
  GNOME/PipeWire setup guides help adoption but change nothing about the
  reliability of the current user's daily dictation path. Resume after 075, so
  the expanded site tells the Cohere-default story rather than propagating the
  Whisper one further. Note the demo asset itself is entangled with 089 — the
  current clip shows `mods: off`.
- **[090 spec drift: `monitor -w` section aliases hardcoded](../issues/090-spec-drift-monitor-w-section-flag-aliases-hardcoded-separately-from-spec-actions-yaml.md)**
  (Open, P3) and **[091 JSON Schemas in `spec/schemas/` are never validated against](../issues/091-spec-system-json-schemas-in-spec-schemas-are-never-actually-validated-against-validate-spec-only-runs-go-test.md)**
  (Open, P2). Both are real gaps between what `docs/Spec.md` promises and what
  the code enforces — `ParseSections` shadows `spec/actions.yaml` with a
  hand-written alias table, and `make validate-spec` is just `go test
  ./spec/...` with no schema validator anywhere in the module. Neither has
  produced user-facing breakage, so they rank below trust and responsiveness
  work; both are small and make good filler alongside larger tickets. 091
  additionally admits a legitimate cheaper resolution: if a schema-validation
  dependency is unwanted, correct `docs/Spec.md` instead of adding the
  validator. Decide that before implementing.
- **[024 GNOME typing-feedback icon](../issues/024-gnome-typing-feedback-icon.md)**
  and **[025 GNOME volume/VU-meter animation](../issues/025-voice-input-volume-animation.md)**
  (Open, P4). Unchanged: low value while the GNOME Shell extension is not in
  daily use and the OS recording indicator suffices. 025 is also partially
  overtaken — 084's live loudness meter delivered the equivalent capability in
  `voxi monitor`, so re-scope 025 against what already exists before starting it.

## Close / Park

- ~~040 grammar-constrained decoding (GBNF)~~ — **closed**, doubly dead:
  parked, because it targets the Whisper path while Cohere is the default
  backend and does not accept prompt/hotword biasing at all (see 075). Recheck
  only on an upstream binary change; there is no productive Voxi work here.
- ~~052 as a standalone ticket~~ — **closed**, folded into 088 §7. Same
  question, same launch path, same systemd unit as 088 — kept as one
  workstream instead of two duplicated benchmarking efforts.
- **037 Work Item 3 (GNOME extension modularization)** — noted parked
  directly in the ticket (037 stays otherwise open: Items 1/2/4 — eager.go
  decomposition, test coverage, doc archiving — are real, untouched work).
  Parked alongside 024 and 025 until extension usage resumes.
- ~~087~~ — **closed**, re-verified resolved: `82b0ee6` (landed after filing)
  tagged `PwRecordCommand` for GNOME suppression, fixing the stated root
  cause.

## Shipped since the 2026-09-05 pass

- **061 telemetry analytics and query commands** closed (`3c60c6f`, `662d316`).
  The previous roadmap's top `Now` item; `voxi telemetry query` now makes the
  060 timeline usable, which is what allows this pass to demand measurements
  before scheduling-priority work.
- **083 safety slice and its regression fix** (`2d438ff`, `6f9cd90`): runaway
  transcript rejection landed, then the over-broad stop semantics it introduced
  were caught and fixed by the trailing-utterance flush. Ticket stays open for
  the remaining at-most-once contract.
- **084 live mic-loudness meter** closed, with follow-on ballistics/perf and
  spec work (`cadf312`, `89e2294`, `92c8dc4`, `82b0ee6`, `85bca67`, `ba33ff8`,
  `7b76138`, `a6cf50a`) plus `docs/LiveMicMeter.md`. It also *created* 087
  (closed the same session, see Close/Park).
- **087 GNOME mic-in-use indicator while idle** and **040 GBNF grammar-
  constrained decoding** and **052 CPU/GPU priority research** all closed
  during this roadmap pass (087 re-verified fixed by `82b0ee6`; 040 doubly
  blocked with no productive path on either ASR backend; 052 folded into 088).
- **086 always-show live loudness**, **085 release onboarding** (v0.1.1),
  **081 persistent `dotoold` user service**, and **082 Cohere transcript
  replacements** all closed.
- **Website and demo asset work** (`01139ce`, `87c3889`, `262f4d2`, `7dd0dff`,
  `e2eb29f`): real screenshots replaced mockups, and a compressed demo video
  plus a Go compression script landed — partial, unclosed progress on 001.

## Reconciliation notes

- **Value axis sharpened** rather than replaced: the prior "immediate and
  trustworthy" framing was right, but with telemetry shipped the operative
  question moved from *can we see what happens* to *is the injection contract
  actually safe*. Trust properties are now ranked explicitly.
- **Out of the roadmap entirely**: 057, 060, and 061 (all closed) — 061 was the
  prior top `Now` item and is recorded under Shipped rather than dropped.
- **New to the roadmap**: 080, 083, 087, 088, 089, 090, 091, 092, all filed
  since the last pass.
- **Promoted**: 075 from unlisted to the head of `Next` — Cohere is the default
  in the spec but not in the docs, and that gates 001. 089 promoted into `Now`
  on cost-versus-trust grounds despite a P2 field, since it is a single install
  command standing between the project and its own headline safety claim.
- **Demoted / restructured**: 052 moved from `Next` in its own right to a merge
  candidate under 088, and both now sit behind a hard requirement to produce
  telemetry numbers first. 056 stayed in `Next` but split explicitly into
  "de-flake the existing gate" and "the deferred contention phases".
- **Deliberately not force-fit into a bucket**: 040 (external blocker), and the
  052/088 duplication, which is a tracker decision rather than a sequencing one.
