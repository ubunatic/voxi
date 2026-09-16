# 124: JRufer/VoxCtrl Research — Architecture Comparison and Learnings for Voxi

**Status**: Research Complete
**Priority**: P3 (Low)
**Severity**: Informational
**Category**: Research
**Related**: [048 Injection fallback chain](048-typing-injection-fallback-chain.md), [027 Continuous listening/wake-word/turn-taking](027-continuous-listening-wake-word-turn-taking.md), [028 Local LLM post-process hook](028-post-process-local-llm-cleanup.md), [106 LLM cleanup service defaults](106-configure-llm-transcription-cleanup-service-defaults-for-lmcoder-integration.md), [121 LLM cleanup timeout latency](121-llm-cleanup-timeout-adds-1-5s-dead-latency-per-chunk-under-cpu-load.md), [122 Gemini Flash cleanup model](122-add-gemini-flash-3-7-support-as-llm-cleanup-model.md), [050 Optional warm-model daemon](050-optional-warm-model-daemon-transcription.md), [089 voxi-modifierd](089-voxi-modifierd-not-installed-on-this-dev-machine-modifier-gating-currently-inactive.md), [102 Multi-language notification audio packs](102-multi-language-audio-packs-for-the-modifier-release-notification-clip.md)

---

## 1. Problem & Motivation

User asked to survey [JRufer/VoxCtrl](https://github.com/JRufer/VoxCtrl) — a
Rust/Tauri voice dictation and desktop-automation broker — for ideas
applicable to Voxi. This is secondhand research (README/architecture-doc
fetch, not a hands-on build or clone), recorded here for future reference
and to seed follow-up tickets where a concrete gap exists.

VoxCtrl overlaps with Voxi's problem space (local STT, desktop keystroke
injection, hotkey-gated dictation) but is architected as a general-purpose
"voice gateway" with many output destinations, whereas Voxi is narrowly
focused on typing into the focused window plus a growing LLM-cleanup path.

## 2. What VoxCtrl Does Differently

- **Hotkeys via XDG Desktop Portal `GlobalShortcuts`** (Linux), not raw
  evdev — the desktop notifies the app when its registered shortcut fires;
  the app never sees other keystrokes. Voxi's `voxi-modifierd` instead reads
  raw evdev to detect modifier hold/release, which needs privileged device
  access ([089](089-voxi-modifierd-not-installed-on-this-dev-machine-modifier-gating-currently-inactive.md)).
  The portal path is more sandboxable/privacy-friendly but constrained to
  whatever gesture vocabulary the portal API exposes (their code supports
  `hold`/`toggle`/`double_tap`/`double_tap_hold` on top of it).
- **11 output-routing "targets"** behind one dispatch abstraction: keyboard
  injection (`wtype`/`xdotool`/`SendInput`), clipboard, shell exec, FIFO,
  TCP/Unix sockets, DBus signals, HTTP/webhook POST, timestamped file
  append, LLM chat endpoints, TTS playback — selected either by default
  target or a spoken prefix ("VoxCtrl notes, ..."). Voxi's output path is
  currently just direct typing (`dotool`) plus the newer LLM-cleanup hook;
  there's no generalized routing layer.
- **Injection fallback chain already implemented**: `wtype` on Wayland,
  `xdotool` on X11, `SendInput` on Windows — directly relevant prior art for
  Voxi's still-open [048](048-typing-injection-fallback-chain.md).
- **LLM cleanup runs in an isolated sidecar process** (`voxctrl-llm-sidecar`,
  `llama.cpp`-backed, Superwhisper's s1-mini model) specifically so text
  normalization cannot block the main dictation path. This is the same
  latency problem Voxi is fighting in
  [121](121-llm-cleanup-timeout-adds-1-5s-dead-latency-per-chunk-under-cpu-load.md)/[122](122-add-gemini-flash-3-7-support-as-llm-cleanup-model.md):
  worth checking whether Voxi's cleanup call is already off the hot typing
  path or still synchronous within it.
- **On-demand model unloading**: idle-timeout-driven eviction of heavy TTS
  models from RAM/VRAM — same shape of idea as Voxi's warm-model daemon
  discussion in [050](050-optional-warm-model-daemon-transcription.md), just
  applied to unloading rather than keeping warm; the two are complementary
  (stay warm while active dictation is likely, evict after idle).
- Multiple STT backends with fallback (whisper.cpp, Moonshine, Parakeet
  TDT, remote HTTP) — same "swappable ASR backend" shape Voxi already has
  via [066](066-canary-cohere-transcribe-and-nemotron-3-5-streaming-as-alternative-asr-backends.md)/[074](074-wire-cohere-transcribe-in-as-an-additional-selectable-asr-backend.md);
  nothing new to adopt here, just confirms the pattern is a reasonable
  general shape for the space.
- Self-updating with SHA-256-verified atomic binary replacement, and MCP
  (Claude Desktop/Cursor) JSON-RPC integration — out of scope for Voxi's
  current priorities (Voxi is released through harnez, not self-update; MCP
  integration isn't a stated goal), noted only for completeness.

## 3. What's NOT Novel / Already Covered

- VAD + noise suppression before transcription — Voxi's `internal/audio`
  segmenter already does comparable work (see
  [065](065-fluidvoice-silence-detection-chunking-and-bad-chunk-rejection-research.md)'s
  conclusion on a similar comparison against FluidVoice).
- Hot-reloadable TOML/JSON config — Voxi already has a YAML spec system
  ([Spec.md](../docs/Spec.md)) serving the same role.

## 4. Candidate Follow-Ups (Not Started — Flagging Only)

1. When picking up [048](048-typing-injection-fallback-chain.md), read
   VoxCtrl's actual fallback-selection logic (Wayland/X11/session detection)
   as a second reference alongside whatever `wtype`/`xdotool` conventions
   are already assumed.
2. When next touching the LLM-cleanup latency work
   ([121](121-llm-cleanup-timeout-adds-1-5s-dead-latency-per-chunk-under-cpu-load.md)/[122](122-add-gemini-flash-3-7-support-as-llm-cleanup-model.md)/[106](106-configure-llm-transcription-cleanup-service-defaults-for-lmcoder-integration.md)),
   evaluate whether isolating cleanup into a separate long-lived
   sidecar/subprocess (rather than an inline synchronous call per chunk)
   would remove the timeout from the hot path the way VoxCtrl's sidecar
   does. This is an architecture idea, not a scoped design — needs its own
   ticket if pursued.
3. If Voxi ever grows beyond direct-typing output (e.g. shell-command
   dispatch, webhook targets, or the wake-word/turn-taking work in
   [027](027-continuous-listening-wake-word-turn-taking.md) needs
   multi-destination routing), VoxCtrl's target-registry-plus-spoken-prefix
   pattern is a reasonable reference design. No current demand identified —
   don't build speculatively.
4. XDG Desktop Portal `GlobalShortcuts` as a lower-privilege alternative
   input path to raw evdev is worth a standalone research spike only if
   `voxi-modifierd`'s privileged evdev requirement becomes a real
   deployment obstacle; not a problem today per
   [089](089-voxi-modifierd-not-installed-on-this-dev-machine-modifier-gating-currently-inactive.md)'s
   resolution.

## 5. Non-Findings / Explicit Non-Adoption

- Self-updating binary mechanism: not needed, Voxi ships via harnez release
  pipeline.
- MCP server integration: no stated Voxi goal to expose itself to
  Claude Desktop/Cursor as a tool.
- Six-engine offline TTS stack: Voxi has no TTS output surface today; out
  of scope unless a future ticket defines one.

## 6. Verification

Desk research only (GitHub README + architecture-summary fetch,
2026-09-16). No code was cloned, built, or run. Treat specifics (crate
names, exact model names) as secondhand and re-verify before relying on
them in an implementation ticket.
