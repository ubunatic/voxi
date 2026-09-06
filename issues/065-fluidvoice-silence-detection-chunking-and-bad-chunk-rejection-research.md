# 065: FluidVoice Silence Detection, Chunking, and Bad-Chunk Rejection (Research)

**Status**: In Progress — starting research per user request
**Priority**: P3 (Low)
**Severity**: Informational
**Category**: Research
**Related**: [054 Short-pause acoustic gating and context priming](054-short-pause-acoustic-gating-and-context-priming.md), [034 Isolated silence-artifact feedback](034-isolated-silence-artifact-feedback.md), [033 User stop-word feedback](033-user-stop-word-feedback.md), [057 Recording start/stop race](057-recording-start-stop-race-delayed-hallucinated-typing-after-stop-cannot-restart-recording.md), [056 End-to-end stress session testing with noise](056-end-to-end-stress-session-testing-with-noise-and-load.md), [021 Fluent streaming typing](021-fluent-streaming-typing.md), [062 FluidVoice vocabulary training research](062-fluidvoice-automatic-vocabulary-training-from-user-corrections-research.md), [063 FluidVoice punctuation/formatting research](063-fluidvoice-spoken-punctuation-and-dictation-literal-post-processing-rules-research.md), [064 FluidVoice model landscape research](064-fluidvoice-model-landscape-fluid-intelligence-licensing-and-underlying-stt-model-portability-research.md)

---

## 1. Problem & Motivation

`altic-dev/FluidVoice` (macOS, Swift, GPLv3, ~11k stars — a "Wispr Flow
alternative") ships an audio capture/streaming pipeline with several
services whose names suggest a dedicated silence-detection, chunk-boundary,
and bad-chunk-rejection design:
`Sources/Fluid/Services/AudioCaptureIdlePolicy.swift`,
`AudioCaptureReadinessGate.swift`, `AudioStartupGate.swift`,
`AudioEngineRetirementDrain.swift`, `ThreadSafeAudioBuffer.swift`,
`DirectCoreAudioInput.swift`, `AudioBufferConverter.swift`,
`ParakeetRealtimeProvider.swift`, and `ASRService.swift`.

Voxi has invested heavily in exactly this problem class already: acoustic
gating and context priming to suppress short-pause hallucinations (issue
054, Implemented), isolated silence-artifact feedback (034, Complete),
user stop-word feedback for hallucinations (033, Complete), a recording
start/stop race that caused delayed hallucinated typing (057, Fixed
2026-09-05), and stress testing under interleaved acoustic noise (056, In
Progress). Voxi's own eager streaming architecture (021, 026;
`internal/eager`) and `internal/audio`/`internal/record` are the Go-side
equivalents of FluidVoice's capture/chunk/STT pipeline.

FluidVoice's README also lists a Hugging Face model,
`nvidia/parakeet_realtime_eou_120m-v1` — "eou" = End-Of-Utterance — which
suggests a dedicated learned endpointing model may be doing chunk-boundary
detection in the realtime path, rather than (or in addition to) a simple
energy/VAD threshold. If confirmed, that would be materially more
sophisticated than Voxi's current acoustic gating and worth understanding
even if not portable.

This ticket is research-only: understand how FluidVoice detects silence,
decides chunk boundaries, and rejects bad/low-quality audio before it
reaches STT or gets typed, well enough to judge what (if anything) is
worth adopting into Voxi given how much prior work already exists here.

## 2. Research Questions

1. **Chunk-boundary triggers**: What triggers an utterance/chunk boundary
   in FluidVoice's realtime path — a silence/VAD energy threshold, the
   Parakeet EOU learned endpointing model, a fixed timeout, or some
   combination? Confirm or refute whether `nvidia/parakeet_realtime_eou_120m-v1`
   is actually wired into `ParakeetRealtimeProvider.swift`'s segmentation
   logic, versus just listed as an available model.
2. **Bad-chunk rejection**: How does FluidVoice decide a captured chunk is
   "bad" (too short, no speech energy, noise-only) and should be discarded
   rather than sent to STT or typed? Is there an explicit rejection gate in
   `ASRService.swift` or elsewhere, and how does it compare structurally to
   Voxi's acoustic-validation gate in issue 054 (same problem, different
   stack)?
3. **Capture/buffer lifecycle and race-safety**: How is audio buffered and
   threaded across capture -> chunk -> STT (`ThreadSafeAudioBuffer`,
   `AudioEngineRetirementDrain`, `AudioCaptureReadinessGate`,
   `AudioStartupGate`)? Are there lifecycle or drain/retirement patterns
   worth comparing against Voxi's recording start/stop race fix (057) or
   its stress-test findings (056)?
4. **EOU model value and portability**: If a dedicated EOU endpointing
   model is confirmed in use, does it plausibly outperform simple
   VAD/energy gating in a way Voxi could benefit from? Is an EOU-style
   model available in a form portable to Voxi's Linux/Go stack (e.g. ONNX
   export) independent of NVIDIA's NeMo/CoreML packaging, or is it
   effectively locked to that ecosystem?

## 3. Deliverables

- A `## Research Findings` section in this ticket answering the above,
  written from actually reading the source files listed above (not just
  the README/marketing copy or the model card alone).
- An explicit feasibility verdict for Voxi, in the style used in issue 062:
  adopt, adopt in reduced form (e.g. borrow only the rejection-gate
  placement or a buffer-lifecycle pattern, not the EOU model), or reject
  with reasons.
- If "adopt" or "reduced form": a rough sketch of what would change in
  `internal/eager`, `internal/audio`, or `internal/record`, and what new
  dependency (e.g. an ONNX EOU model) it would require — not a full
  implementation.

## 4. Non-Goals

- No code changes in this ticket.
- Not re-litigating Voxi's existing acoustic-gating design (054) or
  reopening 056/057 — this ticket only compares FluidVoice's approach
  against them for ideas.
- Not evaluating FluidVoice's vocabulary training (062), punctuation
  post-processing (063), or STT model licensing/portability (064) — those
  are covered by their own tickets.

## 5. Background

Raised 2026-09-06 after the user asked to evaluate FluidVoice
(`https://github.com/altic-dev/FluidVoice`) as a source of adoptable ideas
for Voxi, specifically: "also look how they manage silence detection and
chunking and rejecting bad chunks." Initial scan (shared across all
FluidVoice research tickets this session) established: the app is 100%
Swift on macOS-only frameworks (Speech, Accessibility API, CoreAudio,
CoreML) — the README's "Windows... on the way" is a waitlist signup, not
existing code, so there is no cross-platform core to fork or clone.
Licensing: FluidVoice is GPLv3, Voxi is AGPLv3 — compatible if literal code
were ever reused, though nothing here calls for that given the disjoint
stacks (Swift/macOS vs Go/Linux).
