# Voxi Roadmap

Synthesized from the open issue backlog on 2026-09-05. This is a communication
artifact, not a scheduling tool — see `issues/README.md` (via `harnez index`)
for the authoritative ticket list and statuses.

## Value axis

Per `README.md`, voxi's reason to exist is: **fast, local, privacy-first
dictation that never leaves the machine, feels responsive (sub-second), and
never corrupts the focused window with hallucinated or delayed text.** Every
bucket placement below is judged against that — not ticket age or priority
field alone. For a single-user daily-driver tool, "doesn't silently misbehave"
outranks "has more features."

---

## Now — protect the daily-driver path

The pipeline is in active daily use and just had two real bugs fixed this
session (unbounded transcribe-subprocess timeout; asymmetric punctuation
matching in silence-artifact filtering, see 034 §7). The two items below are
what's most likely to erode trust in the tool if left alone.

- **[057 recording start/stop race: delayed hallucinated typing after stop,
  cannot restart recording](../issues/057-recording-start-stop-race-delayed-hallucinated-typing-after-stop-cannot-restart-recording.md)**
  (Open, P1 — filed 2026-09-05). This session traced a suspicious
  `Recording started`/`stopped`/`started`/`stopped` sequence all within the
  same second in `journalctl`, plus a user report of "random hallucinations in
  chat long after record is off, and then I can't turn recording back on."
  This is the single highest-value item outstanding — it directly attacks the
  "the tool must not misbehave after you think you've stopped it" property.
  Root-cause `runEagerDaemon`'s `stopCurrent`/`startRecording`/
  `toggleRecording` mutex-guarded state in `internal/eager/eager.go` before
  anything else in this list.
- **[052 CPU/GPU scheduling priority under load](../issues/052-cpu-gpu-priority-under-load.md)**
  (Open — Research, P2). Directly serves "feels responsive": today
  `voxi-agent.service` and its `voxtype` children run at default `nice`/no
  cgroup weight, so a build, a local LLM, or a browser tab can make dictation
  visibly lag exactly when the user is mid-thought. Research-only step is
  cheap and unblocks a well-scoped follow-up implementation ticket.

---

## Next — close the gate on warm-model, then harden

- **[050 optional warm-model/daemon-mode transcription](../issues/050-optional-warm-model-daemon-transcription.md)**
  / **[051 warm-model prior-art research](../issues/051-warm-model-prior-art-research.md)**
  (Reopened / Research Complete, P2). 051's research found a real third
  option (a `whisper.cpp`-family resident HTTP server) that 050's original
  closure didn't consider, and 051 §3.2 captured fresh live evidence
  (RTF up to 19x on trivial utterances) that the underlying per-utterance
  reload cost is real and current, not historical. Gate items 1-4 in 051 §4
  are cheap canary checks (confirm the binary exists, confirm model-file
  reuse, measure latency, confirm lifecycle scoping) — worth clearing before
  any implementation commitment. This is `Next` and not `Now` because it's a
  latency/GPU-load *quality* improvement, not a correctness bug — the two
  `Now` items outrank it.
- **[040 GBNF grammar-constrained decoding](../issues/040-whisper-cpp-grammar-constrained-vocabulary.md)**
  (Blocked — Grammar Flag Not Available, P2). Directly serves dictation
  accuracy for technical terms, which 032's softer prompt-biasing only
  partially solves. Currently blocked on the installed `voxtype` build not
  exposing the grammar flag — re-check after any `voxtype` upgrade; no voxi
  work is possible until then.
- **[056 end-to-end stress-session integration testing](../issues/056-end-to-end-stress-session-testing-with-noise-and-load.md)**
  (Open, P3). Natural pairing with 052 — once CPU/GPU priority work lands,
  this is the regression harness that proves it (and catches the class of
  bug the `Now` toggle-race item represents) before it ships silently broken
  again.
- **[048 injection fallback chain (wtype/clipboard-paste)](../issues/048-typing-injection-fallback-chain.md)**
  (Proposed, P3). Low-risk robustness addition — `dotool` remains the
  primary, intentional dependency; this only prevents a hard failure when
  it's missing. Small, self-contained, doesn't compete for priority against
  the correctness-first items above.

---

## Later — polish and expansion

- **[045 editable pre-filled transcript + keyterm prompt for sample recorder](../issues/045-sample-recorder-editable-transcript-and-keyterms.md)**
  (Proposed, P2). Real workflow friction for whoever curates the dev corpus
  (today: retype-from-scratch, keyterms field silently empty), but it's a
  contributor/maintainer tool, not the dictation path itself — lower value
  than anything above.
- **[037 code quality / test coverage / modularization](../issues/037-code-quality-and-test-coverage-roadmap.md)**
  (Open, P3). `internal/eager/eager.go` is already the file every bug this
  session touched — decomposing it (Work Item 1) would make the `Now` bug
  above easier to fix and would have made the two bugs already fixed this
  session easier to review. Worth doing once the current live-bug backlog is
  cleared, not before — don't refactor the file mid-firefight.
- **[001 website integration and public docs](../issues/001-website-integration.md)**
  (In Progress). Public-facing polish; doesn't affect the tool's own
  reliability or accuracy for its actual (currently solo) user.
- **[035 SSH-managed remote transcription server research](../issues/035-ssh-remote-transcription-server-research.md)**
  (Proposed / Research, P2). Legitimate architecture idea (offload inference
  to a beefier machine) but it's speculative multi-machine infrastructure for
  a tool whose stated identity is "zero cloud dependencies" / local-first;
  research it after the *local* latency levers (050/051, 052) are actually
  exhausted, not before.
- **[021 fluent streaming typing / enter-to-stop](../issues/021-fluent-streaming-typing.md)**
  (Research spike complete, opt-in, not shipped). Real UX upgrade (word-by-word
  typing instead of batch), but the spike itself found it needs a
  streaming-capable backend voxi doesn't currently default to — re-scope this
  before treating it as shovel-ready; not competing for attention against the
  `Now`/`Next` items.
- **[031 Claude Code / agent CLI voice pipeline research](../issues/031-claude-code-and-agent-cli-voice-pipeline-research.md)**
  (Proposed / Research). Informational research with no committed
  implementation target; low urgency.

---

## Close / Park

- **[024 GNOME typing-feedback icon](../issues/024-gnome-typing-feedback-icon.md)** and
  **[025 GNOME volume/VU-meter animation](../issues/025-voice-input-volume-animation.md)**
  — both explicitly P4, both tickets state the user does not currently use
  the GNOME Shell extension and the built-in OS indicator is sufficient.
  Park until extension usage actually resumes; don't let these accumulate
  priority just by sitting in the backlog.
- **037 Work Item 3 (GNOME extension modularization)** — same reasoning,
  already marked "Deprioritized — P4" inside the ticket itself.

---

## Gaps Reviewed

Independently reviewed for genuine, concrete product gaps beyond what the
open backlog already covers. Kept conservative — only flagging things that
are clearly missing and clearly justified, not speculative nice-to-haves.

- **Recording start/stop state-machine race (see `Now` above)** — was an
  unticketed gap as of the first pass of this roadmap; now filed as
  [057](../issues/057-recording-start-stop-race-delayed-hallucinated-typing-after-stop-cannot-restart-recording.md).

No other new gaps met the bar for "clearly missing, clearly justified" — the
rest of what might otherwise look like a gap (multi-language model support,
packaging, sandboxing/hardening, non-English decoding) is either explicitly
out of scope for a single-user local tool per its own README framing, or
already tracked under an existing ticket above.
