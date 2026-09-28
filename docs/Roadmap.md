# Voxi Roadmap

Reconciled from the active issue backlog on 2026-09-27 (previous passes:
2026-09-11, 2026-09-10, 2026-09-05). This is a communication artifact, not a
scheduling tool; `issues/README.md` remains the authoritative tracker.

## Value axis

Voxi earns its place as a daily driver by making local Wayland dictation feel
immediate and **trustworthy**: the microphone starts and stops predictably,
speech becomes accurate text quickly, every word the user actually spoke gets
typed exactly once and *as the right characters*, and nothing else ever
reaches the keyboard. Trust remains the binding constraint.

Since 2026-09-11 the product grew a second, genuinely used surface: **voxi
reads text aloud** (141 TTS MVP, 148 daemon-owned queue, 152 Super+X stops
TTS). It also grew a second ASR engine in daily use (R2T2 via `llama-server`,
134). The axis therefore has two lines, ranked:

1. **Dictation trust** (unchanged, still first):
   - *Correct characters at the keyboard* — new leading edge this pass. The
     layout bug (129) types `y`/`z` swapped on a non-install-time layout, and
     the attempted fix produced a path that types nothing (130). Wrong or
     missing text is exactly the failure the axis ranks worst.
   - *Exactly-once delivery* — largely closed (083 §9/§10, 115 implemented,
     awaiting a human live check).
   - *Only the user's speech* — still open (100, 096), but stalled with no
     progress since the last pass.
   - *Settings mean what they say* — new: saved settings that silently do not
     apply (113, 128) are a trust bug, not a UX nicety.
2. **Accuracy and responsiveness under real engines and load** — now concrete
   because there are two engines and an LLM cleanup step to measure.
3. **Reading aloud (TTS)** — real daily value, but a secondary product line;
   its work ranks below any dictation-trust defect.

Presentation, distribution, and hygiene rank below all three.

## Now — correct text at the keyboard, and settings that apply

> **Update 2026-09-27:** shipped and closed: 129 + 130 (layout follows the
> active input source, prewarmed on switch, live-verified), 113 (history and
> modifier-gating settings applied at runtime) and 128 (restart on save).
> The regression that stopped the eager child starting (`004a9f4`, `e5e46af`)
> is fixed. Proposed next: 125 (pause media during recording) ahead of 100, as a
> cheap removal of one real background-voice source.

- **[129 dotoold layout must follow the active input source](../issues/129-dotoold-keyboard-layout-must-follow-the-active-input-source-not-the-install-time-layout.md)**
  (Open, P1) together with **[130 standalone dotool types nothing on GNOME](../issues/130-standalone-dotool-injection-exits-successfully-but-produces-no-visible-typing-on-gnome.md)**
  (Open, P1). Head of `Now`. Dictation on the user's own second keyboard
  profile produces wrong characters; the per-injection standalone `dotool`
  fix was reverted because it exits successfully and types nothing — a
  *silent* delivery failure, the worst kind. The two tickets are one problem:
  find a layout-following mechanism that keeps the working persistent
  `dotoold` FIFO path. Canary-first (`docs/Canary.md`): probe how `dotoold`
  can switch keymap before rebuilding anything.
- **[115 modifier-buffer flush drops](../issues/115-prevent-modifier-buffer-flush-drops-from-expired-stopdraintimeout.md)**
  (In Progress, P1). Implementation and review are done; only the §6.3 human
  live-dictation gate is open. That is minutes of the user's time and closes
  a P1 that dropped fully transcribed speech. Ask for it, don't let it age.
- **[113 wire saved ASR/history/modifier settings into runtime](../issues/113-wire-saved-asr-history-and-modifier-settings-into-runtime.md)**
  (Open, P1) and **[128 settings save should restart or reload the daemon](../issues/128-settings-save-should-restart-the-daemon-or-auto-reload-changed-config.md)**
  (Open, P2). New to the roadmap. "History off" still writes history, and the
  modifier toggle is ignored; the ASR-model half was fixed during 126 and 128
  now offers a restart prompt. What remains is small and trust-relevant: honour
  or remove each toggle, and do 128's live check plus its delivery-ledger
  criterion.
- **[100 background-voice false acceptance](../issues/100-background-distant-voice-hallucinated-into-accepted-transcripts-bypassing-silence-gate.md)**
  (In Progress, P1) with **[096 acoustic classification research](../issues/096-spectral-centroid-keyboard-clack-vs-speech-classification-research.md)**
  (Research In Progress, P2). Demoted from the head of `Now`, still `Now`.
  Nothing moved on either since 2026-09-11, and the ASR landscape changed
  under them (R2T2 in daily use, see 138's no-speech sentinel). The prior
  pass's decision point stands: take the harmonicity / pitch-salience canary,
  or decide deliberately that hand-crafted features are not enough. Note that
  **125 (pause media playback)** removes one real source of background voices
  cheaply and is a partial mitigation worth weighing here.

## Next — measure the engines, fix cleanup latency, finish the TTS WIP

Dictation accuracy and latency:

- **[137 keyterm prompt biasing for openai-transcribe](../issues/137-wire-corpus-keyterms-into-openai-transcribe-prompt-for-vocabulary-biasing.md)**
  (Open, P2). R2T2 is in daily use and mangles project vocabulary (Voxi ->
  "Foxy"); the API field exists and the corpus makes the prompt shape
  measurable. Highest-value accuracy item in the backlog.
- **[133 bench Cohere via crispasr in voxi bench](../issues/133-bench-crispasr-cohere-transcribe-alongside-whisper-models-in-voxi-bench.md)**
  (Open, P2). Without it, engine choice (Cohere vs R2T2 vs Whisper) is
  anecdotal; it also absorbs the last open item of 131.
- **[121 cleanup timeout dead latency](../issues/121-llm-cleanup-timeout-adds-1-5s-dead-latency-per-chunk-under-cpu-load.md)**
  (Open, P2), **[123 retest cleanup models](../issues/123-retest-llm-cleanup-models-against-current-yaml-request-format-and-2-5s-timeout.md)**
  (Open, P3), **[112 multiline fidelity through cleanup](../issues/112-preserve-multiline-transcript-fidelity-through-llm-cleanup.md)**
  (Open, P2). One track: 123's loaded/cold re-measurement decides whether 121
  closes by headroom or needs adaptive skip; 112's policy follows from the
  same numbers. Every fallback chunk costs up to 2.5 s of dead latency.
- **[083 injection-safety residue](../issues/083-prevent-runaway-repeated-dotool-desktop-injection.md)**
  (In Progress, P1). Moved from `Now` to `Next`: the contract holds in daily
  use; remaining are emergency stop, injector lifecycle canaries, and the
  claim-to-submit crash window. Bounded hardening, not an open hole.
- **[056 stress-session phases](../issues/056-end-to-end-stress-session-testing-with-noise-and-load.md)**
  (In Progress, P3) and **[088 scheduling priority](../issues/088-elevate-os-scheduling-priority-for-the-transcription-critical-path.md)**
  (Open, P3). Unchanged: one live Phase 2 run, then the idle-vs-loaded canary
  that 088 needs. 121's load findings feed directly into 088.

Reading aloud (TTS):

- **[144 handoff: resume 143 and 142 follow-ups](../issues/144-handoff-resume-143-super-y-reader-wip-and-142-layout-follow-ups.md)**,
  **[143 read selection/clipboard via hotkey](../issues/143-read-the-primary-selection-aloud-via-hotkey-shift-hotkey-reads-the-clipboard.md)**,
  **[142 unified log feed follow-ups](../issues/142-monitor-merge-tts-into-a-unified-voxi-log-feed-tts-box-on-and-hardware-box-off-by-default.md)**
  (all Open, P2). Untested WIP exists; finishing it is cheaper than letting it
  rot. 143 is the feature that makes TTS usable on arbitrary text.
- **[145 Markdown normalization for TTS](../issues/145-improve-markdown-text-normalization-for-tts-reading.md)**
  (Open, P2). Small, unit-testable, and applies to every read-aloud path.
- **[162 remove Chatterbox and keep the cloned-voice seam](../issues/162-remove-chatterbox-tts-serve-keep-cloned-voice-engine-placeholder.md)**
  (Open, P2). Removes the slow, memory-heavy Chatterbox/tts-serve implementation
  while preserving the reference-WAV profile and engine dispatch. The `voxcpm`
  selection is a clear-error placeholder for **[160 VoxCPM](../issues/160-adopt-voxcpm-gguf-vulkan-as-cloned-voice-tts-engine-incl-voxi-install.md)**.
- **[140 harden the harness against hazardous local inference](../issues/140-harden-the-agent-harness-against-resource-hazardous-local-inference-runs.md)**
  (Open, P2). Promoted from unranked into `Next` as 155's prerequisite: a
  prior uncapped `llama-server` made the machine unusable.

## Later — distribution, config hygiene, polish

- **[075 Cohere-default messaging](../issues/075-align-documentation-and-product-messaging-with-cohere-transcribe-as-the-default-asr.md)**
  (Open, P1) — moved from `Next`: with R2T2 and cloud engines selectable,
  "Cohere is *the* default" is less of the story; fold the remaining audit
  and website publish into a broader engine-choice message. Still gates
  **[001 website](../issues/001-website-integration.md)** (In Progress).
- **[116 installer portability matrix](../issues/116-test-installer-portability-across-distributions-and-real-systemd.md)**
  (Open, P2) — adoption value, not daily-driver value.
- **[106 LLM cleanup service defaults](../issues/106-configure-llm-transcription-cleanup-service-defaults-for-lmcoder-integration.md)**
  (Open, P2) and **[126 lmcoder transcription endpoint](../issues/126-use-lmcoder-openai-compatible-transcription-api-as-asr-backend-target.md)**
  (In Progress, P2) — both waiting on the lmcoder side; the voxi engine
  already exists.
- **[125 pause media during recording](../issues/125-pause-media-playback-during-recording-resume-if-voxi-paused-it.md)**
  (Open, P3) — see the note under 100; promote if background audio proves a
  frequent false-acceptance source.
- **[138 R2T2 no-speech sentinel](../issues/138-treat-r2t2-language-none-scaffold-as-an-explicit-no-speech-result.md)**
  (Open, P3) — small; effect today is a dropped "Uh".
- **[119 consolidate config formats](../issues/119-consolidate-voxi-local-config-storage-formats-stop-words-replacements-vocabulary-samples-config-yaml.md)**
  and **[099 corpus manifest](../issues/099-replace-corpus-tsv-sample-manifest-with-a-more-robust-storage-format.md)**
  (Open, P3) — design together; 119 explicitly references 099.
- **[095 spoken numbers](../issues/095-normalize-spoken-number-words-to-digits-in-dictated-transcripts-library-vs-build-our-own.md)**,
  **[090 monitor alias spec drift](../issues/090-spec-drift-monitor-w-section-flag-aliases-hardcoded-separately-from-spec-actions-yaml.md)**,
  **[037 code quality](../issues/037-code-quality-and-test-coverage-roadmap.md)**,
  **[102 notification language packs](../issues/102-multi-language-audio-packs-for-the-modifier-release-notification-clip.md)**
  — unchanged from the prior pass.
- **[024](../issues/024-gnome-typing-feedback-icon.md)** and
  **[025](../issues/025-voice-input-volume-animation.md)** GNOME indicators
  (P4) — unchanged; 025 partially overtaken by the monitor's live meter.

## Close / Park

- **134 (R2T2 engine)** — close. The ticket says itself nothing remains; 139
  (systemd unit) is closed, and 137/138 carry the follow-ups.
- **131 (R2T2 research)** — close; its only open item (WER vs Cohere) is 133.
- **154 (Pocket TTS canary)** — close as a completed canary with a negative
  result: it works end to end but drops words on longer texts.
- **157 (TalkWithMe / tts-serve research)** — research history only; its
  implementation recommendation was retired by 162. Issue 160 owns the next
  cloned-voice engine evaluation.
- **156 (Piper fine-tuning bugs)** — park and decide whether to remove
  `voxi voice train`; the prior cloned-voice implementation was retired, and
  issue 160 is evaluating a replacement engine.
- **149 (personalized voice umbrella)** and **147 (TTS engine research)** —
  close or fold into 155: Piper stock voices shipped, and the my-voice goal
  now has a concrete path.
- **132 (Qwen3.8 live STT)** — park; blocked on an external release.
- **122 (Gemini Flash cleanup via `agy`)** — park; every tested design misses
  the 2.5 s bound, and 123 decides the cleanup backend question.
- **091 (unvalidated JSON Schemas)** — unchanged: decide, don't schedule
  (rewording `docs/Spec.md` closes it for free).
- **097 (public speech sample catalog)** — unchanged: parked.

## Shipped since the 2026-09-11 pass

- **TTS line:** 141 (TTS MVP: `voxi say`, Festival/Piper, media keys),
  146 (`voxi say --llm` plan), 148 (daemon owns the TTS queue, recording
  mutes playback), 150 (compact monitor mode), 151 (voice prepare/train
  pipeline), 152 (Super+X permanently stops TTS), 153 (clone-sample
  allowlist).
- **ASR engines:** 134's R2T2 integration (ticket not yet closed, see above),
  135/136 (monitor shows the selected model and backend state), 139 (voxi
  install owns all units incl. R2T2).
- **Settings, config and install:** 107 (settings TUI), 114 (out-of-checkout
  modifierd build), 117/118 (sample and config import).
- **Diagnostics and cleanup evaluation:** 108 (chunk trace inspection),
  109/110 (audiolevel sparkline and example), 111 (cleanup fidelity
  evaluation; multiline failure became 112).
- **Delivery:** 120 closed as a duplicate of 115.
- **Research closed:** 124 (VoxCtrl comparison), 127 (hooking agent audio
  capture — not recommended).

## Reconciliation notes

- **New leading edge: 129/130 above 100.** A live bug that types wrong or no
  characters outranks a stalled research-dependent acceptance problem.
- **Promoted: 113/128 and 115's live gate into `Now`** — small, trust-level,
  mostly waiting on a check.
- **Demoted: 083 to `Next`, 075 to `Later`** — with reasons given in place.
- **New theme: reading aloud (TTS).** Placed in `Next` below dictation
  accuracy; issue 162 leaves a cloned-voice placeholder for the VoxCPM work in
  issue 160.
- **New Close/Park entries:** 131, 134, 147, 149, 154, 157 (done or
  superseded), 132 and 122 (externally blocked), 156 (decide to remove).
