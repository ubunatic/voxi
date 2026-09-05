# Voxi Roadmap

Reconciled from the active issue backlog on 2026-09-05. This is a
communication artifact, not a scheduling tool; `issues/README.md` remains the
authoritative tracker.

## Value axis

Voxi earns its place as a daily-driver by making local Wayland dictation feel
immediate and trustworthy: the microphone must start and stop predictably,
speech must become accurate text quickly, and no stale or hallucinated text may
appear after the user believes a session is over. Work that makes those
properties measurable and then improves them outranks presentation and
distribution work.

## Now — make today's telemetry actionable

- **[061 telemetry analytics, statistics, and query commands](../issues/061-telemetry-analytics-statistics-and-query-commands.md)**
  (Open, P2). This is the immediate next slice. Issue 060 now records the exact
  mic, chunk, transcription, and typing timeline, but the JSONL still requires
  manual joins. A small read-only CLI turns today's instrumentation into an
  operational tool: identify post-deactivation drains, distinguish queue delay
  from Whisper time and typing delay, and compare latency with silence/audio
  characteristics. Implement raw filtering and correlated per-session/per-chunk
  views first, then the bounded aggregate report; stable JSON output and
  deterministic fixtures keep it useful for later stress work.
- **[056 end-to-end stress-session integration testing](../issues/056-end-to-end-stress-session-testing-with-noise-and-load.md)**
  (In Progress, P3). Finish the smallest correctness gap after 061: make the
  Phase 2 noise-rejection assertion deterministic without changing production
  defaults, then use 061's queries against replayed sessions. This joins the
  test harness and telemetry into one evidence loop before adding synthetic
  contention or a wider fixture sweep.

## Next — improve responsiveness with evidence

- **[052 CPU/GPU scheduling priority under load](../issues/052-cpu-gpu-priority-under-load.md)**
  (Open — Research, P2). Run the canary probes and idle-versus-loaded benchmark
  after the telemetry query surface exists. That ordering replaces subjective
  reports of sluggishness with stage-level measurements and makes it possible
  to tell CPU starvation, GPU queueing, capture loss, and ordinary transcription
  cost apart before changing service priorities.
- **056 remaining phases** — add controlled CPU/GPU contention, compare RTF and
  stage latency under load, sweep pause lengths, and expand corpus coverage.
  This should validate whatever 052 recommends rather than becoming a separate
  performance project with different measurements.
- **[037 code quality, coverage, and modularization](../issues/037-code-quality-and-test-coverage-roadmap.md)**
  (Open, P3), limited initially to the Eager decomposition and focused CLI/audio/
  ASR tests. The live-race fix and telemetry both touched the central Eager path;
  extracting clearer lifecycle and emission boundaries is valuable once the
  observable behavior is protected by 056 and 061. GNOME modularization remains
  parked.

## Later — distribution and optional UI polish

- **[001 website integration and public documentation](../issues/001-website-integration.md)**
  (In Progress). Interactive demos, packaging recipes, and setup guides improve
  adoption, but do not improve the reliability or responsiveness of the current
  user's daily dictation path. Resume after the measurement-and-hardening loop.
- **[024 GNOME typing-feedback icon](../issues/024-gnome-typing-feedback-icon.md)**
  and **[025 GNOME volume/VU-meter animation](../issues/025-voice-input-volume-animation.md)**
  (Open, P4). Both remain low-value while the user does not use the GNOME Shell
  extension and the operating-system recording indicator is sufficient.

## Close / Park

- **[040 grammar-constrained decoding](../issues/040-whisper-cpp-grammar-constrained-vocabulary.md)**
  remains blocked because the installed `voxtype` exposes no grammar flag.
  Recheck only after an upstream binary change; there is no productive Voxi
  implementation work to schedule today.
- **037 Work Item 3 (GNOME extension modularization)** remains parked alongside
  024 and 025 until extension usage resumes.

## Shipped today

- **[057 start/stop race](../issues/057-recording-start-stop-race-delayed-hallucinated-typing-after-stop-cannot-restart-recording.md)**
  shipped in `0d416f4` and passed a real Super+X retest. Recording lifecycle is
  decoupled from old transcription drains, eliminating the observed delayed
  typing/unresponsive-restart failure.
- **[060 correlated Eager telemetry](../issues/060-correlated-eager-pipeline-telemetry-for-mic-to-type-latency.md)**
  shipped in `323ab5e`. The private append-only event timeline now correlates
  microphone activation/deactivation, chunk audio metrics, Whisper work, and
  typing, including work that drains after the mic closes.
- **056 Phase 1 and most of Phase 2** shipped in `bd192c5`: the real pipeline
  replay harness exercises speech plus interleaved noise and caught a genuine
  stop-word gap. Independent reruns exposed one flaky bare-period rejection
  assertion, so 056 correctly remains active rather than being declared done.

## Reconciliation notes

- 057 moved out of `Now` to `Shipped today`; 060 was added there after landing.
- 061 is the new top `Now` item because it unlocks practical use of 060 and gives
  052/056 a shared measurement surface.
- 052 moved behind 061, and 056 moved up beside it: performance tuning should be
  measured with the new correlated timeline rather than evaluated by feel.
- Tickets no longer returned by the active-backlog query were removed from the
  prior `Next`/`Later` lists rather than carried forward as stale roadmap work.
