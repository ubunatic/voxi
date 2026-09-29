# Voxi Roadmap

Reconciled from the active issue backlog on 2026-09-29 (previous passes:
2026-09-27, 2026-09-11, 2026-09-10, 2026-09-05). This is a communication
artifact, not a scheduling tool; `issues/README.md` remains the authoritative
tracker.

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

*2026-09-29:* the layout and settings items above have shipped. A new trust
dimension joins line 1: **the user's own voice recordings go only where the
user allowed** (sample store, 169-175): development samples never feed
cloning or training by accident, and only noise is ever published.

## Now — finish the sample store, then close the dictation-trust gates

> **Update 2026-09-29:** a new theme took over the working set. Issue 169
> decided on one sample concept and store (`docs/SampleStore.md`); 170 (store
> package) and 172 (`voxi sample` commands) shipped, 099 closed as superseded.
> 171 (legacy migration and cleanup) shipped the same day, so 173 now heads
> `Now`.

- **[173 sample purpose guards and consent](../issues/173-sample-purpose-guards-voice-only-training-and-cloning-consent-noise-only-publish.md)**
  (Open, P1). Head of `Now`, because it depends on 170/172 (done) and the
  store's invariants only become real when enforced: development samples
  must never reach cloning or training, and only noise may be published.
  Privacy of the user's own voice is a trust issue on par with delivery.
- **[115 modifier-buffer flush drops](../issues/115-prevent-modifier-buffer-flush-drops-from-expired-stopdraintimeout.md)**
  (In Progress, P1). Unchanged: only the human live-dictation gate is open.
  Minutes of the user's time close a P1 that dropped transcribed speech.
- **[125 pause media during recording](../issues/125-pause-media-playback-during-recording-resume-if-voxi-paused-it.md)**
  (Open, P3). Promoted from `Later`, as proposed on 2026-09-27: a cheap way
  to remove one real source of background voices while 100 stays stalled.
- **[100 background-voice false acceptance](../issues/100-background-distant-voice-hallucinated-into-accepted-transcripts-bypassing-silence-gate.md)**
  (In Progress, P1) with **[096](../issues/096-spectral-centroid-keyboard-clack-vs-speech-classification-research.md)**
  research. Still `Now`, still stalled; the decision (harmonicity canary, or
  accept hand-crafted features are not enough) is still owed. The finished
  sample store (noise vs dictation purposes) gives this work a clean corpus.

## Next — engine accuracy, cleanup latency, sample follow-ups, TTS

Dictation accuracy and latency:

- **[137 keyterm prompt biasing](../issues/137-wire-corpus-keyterms-into-openai-transcribe-prompt-for-vocabulary-biasing.md)**
  (Open, P2). Unchanged; highest-value accuracy item. Now reads its keyterms
  from the new store (171 shipped).
- **[133 bench Cohere via crispasr](../issues/133-bench-crispasr-cohere-transcribe-alongside-whisper-models-in-voxi-bench.md)**
  (Open, P2). Unchanged; makes engine choice measurable.
- **[158 call lmcoder proxy by model name](../issues/158-call-lmcoder-proxy-on-8735-by-model-name-instead-of-raw-8734.md)**
  (Open, P2). New. Cleanup today talks to whatever model sits on port 8734,
  which `lmcoder load` or idle eviction can swap or stop. Small fix; it goes
  with the cleanup track because 123's measurements are only meaningful if
  the model answering is the one named.
- **[121](../issues/121-llm-cleanup-timeout-adds-1-5s-dead-latency-per-chunk-under-cpu-load.md)
  / [123](../issues/123-retest-llm-cleanup-models-against-current-yaml-request-format-and-2-5s-timeout.md)
  / [112](../issues/112-preserve-multiline-transcript-fidelity-through-llm-cleanup.md)
  cleanup track** — unchanged: 158 first, then 123 measures, 121 and 112
  follow from the numbers.
- **[083 injection-safety residue](../issues/083-prevent-runaway-repeated-dotool-desktop-injection.md)**,
  **[056 stress phases](../issues/056-end-to-end-stress-session-testing-with-noise-and-load.md)**,
  **[088 scheduling priority](../issues/088-elevate-os-scheduling-priority-for-the-transcription-critical-path.md)**
  — unchanged bounded hardening.

Sample store follow-ups (from 169):

- **[174 review public noise samples](../issues/174-review-public-noise-samples-and-move-them-to-testdata-samples-noise.md)**
  (Open, P2). After 173's noise-only publish guard, so the move into
  `testdata/samples/noise` runs through the guarded path.
- **[175 sample glossary and wording](../issues/175-sample-glossary-help-text-man-pages-and-website-wording.md)**
  (Open, P3). Cheap, and users cannot tell chunk, sample, corpus and clone
  apart today; do it once the command set stops moving (after 171/173).

Reading aloud (TTS):

- **[144 handoff](../issues/144-handoff-resume-143-super-y-reader-wip-and-142-layout-follow-ups.md)**
  and **[143 read selection/clipboard via hotkey](../issues/143-read-the-primary-selection-aloud-via-hotkey-shift-hotkey-reads-the-clipboard.md)**
  (Open, P2). 142 closed, so 144 reduces to finishing the 143 reader WIP.
- **[145 Markdown normalization for TTS](../issues/145-improve-markdown-text-normalization-for-tts-reading.md)**
  (Open, P2). Unchanged.
- **[140 harden against hazardous local inference](../issues/140-harden-the-agent-harness-against-resource-hazardous-local-inference-runs.md)**
  (Open, P2). Still relevant: VoxCPM (160) now runs local GGUF inference.

## Later — distribution, config hygiene, polish

- **[161 VoxCPM 2 as optional model](../issues/161-voxcpm-2-multilingual-48-khz-as-optional-voxcpm-model.md)**
  (Open, P3). New. Multilingual (German) cloned voice at 48 kHz, but a 2B
  model; 160 already delivers a working cloned voice, so this is an upgrade,
  gated on 140 and on 173's cloning guards.
- **[168 merge chunks/samples into longer voice material](../issues/168-merge-chunks-samples-into-longer-clean-voice-material.md)**
  (Open, P3). New. Serves cloning/training input quality; lands on the new
  store and behind 173's guards, so it waits for both.
- **[119 consolidate config formats](../issues/119-consolidate-voxi-local-config-storage-formats-stop-words-replacements-vocabulary-samples-config-yaml.md)**
  (Open, P3). Its samples half is now answered by the store (169/170); what
  remains is stop-words, replacements, vocabulary and config.yaml.
- **[075 engine messaging](../issues/075-align-documentation-and-product-messaging-with-cohere-transcribe-as-the-default-asr.md)**
  gating **[001 website](../issues/001-website-integration.md)**,
  **[116 installer portability](../issues/116-test-installer-portability-across-distributions-and-real-systemd.md)**,
  **[106](../issues/106-configure-llm-transcription-cleanup-service-defaults-for-lmcoder-integration.md)
  / [126](../issues/126-use-lmcoder-openai-compatible-transcription-api-as-asr-backend-target.md)
  lmcoder-side waits**, **[138 R2T2 no-speech sentinel](../issues/138-treat-r2t2-language-none-scaffold-as-an-explicit-no-speech-result.md)**,
  **[095 spoken numbers](../issues/095-normalize-spoken-number-words-to-digits-in-dictated-transcripts-library-vs-build-our-own.md)**,
  **[090 alias spec drift](../issues/090-spec-drift-monitor-w-section-flag-aliases-hardcoded-separately-from-spec-actions-yaml.md)**,
  **[037 code quality](../issues/037-code-quality-and-test-coverage-roadmap.md)**,
  **[102 notification language packs](../issues/102-multi-language-audio-packs-for-the-modifier-release-notification-clip.md)**,
  **[024](../issues/024-gnome-typing-feedback-icon.md)/[025](../issues/025-voice-input-volume-animation.md)
  GNOME indicators** — unchanged from the prior pass.

## Close / Park

- **132 (Qwen3.8 live STT)** — park; blocked on an external release.
- **122 (Gemini Flash cleanup)** — park; misses the 2.5 s bound, 123 decides
  the cleanup backend.
- **091 (unvalidated JSON Schemas)** — decide, don't schedule (rewording
  `docs/Spec.md` closes it for free).
- **097 (public speech sample catalog)** — park; revisit only after 174
  settles what public samples look like.
- **144 (handoff)** — close once 143 ships; it carries no scope of its own.

## Shipped since the 2026-09-27 pass

- **Sample store:** 169 (decision record, `docs/SampleStore.md`), 170 (store
  package), 172 (`voxi sample` list/show/play/add/record/edit/move/delete),
  171 (live migration and legacy cleanup); 099 closed as superseded by 170.
- **Chunks and samples tooling:** 163-167 (incl. editor prompt for sample
  transcripts, chunk delete filters).
- **TTS / cloned voice:** 160 (VoxCPM adopted as cloned-voice engine), 162
  (Chatterbox removed), 155, 159; 142 (unified log feed) closed.
- **Closed as proposed last pass:** 131, 134, 147, 149, 154, 156, 157.
- **Dictation trust (already noted 09-27):** 129/130 (layout follows input
  source), 113/128 (settings apply at runtime).

## Reconciliation notes

- **171 shipped** (2026-09-29): it headed `Now` only to fix the broken build; 173 moves up.
- **New in `Now`: 173 (P1)** — voice-sample privacy guards, ranked with trust.
- **Promoted: 125 `Later` -> `Now`**, as the 09-27 pass proposed.
- **New in `Next`: 158, 174, 175**; **new in `Later`: 161, 168** (both wait
  on 173's guards).
- **Removed from roadmap:** 129, 130, 113, 128, 142, 162 (shipped), all
  prior Close/Park entries that were closed (131, 134, 147, 149, 154, 156, 157).
- **Narrowed: 119** (samples half answered by the store), **144** (142 half done).
