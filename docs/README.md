# Voxi Documentation

In-depth references for architecture, decisions, and operations of the Voxi Linux Voice Input & Synthetic Typing Engine.

| Document | Description |
| :--- | :--- |
| [VoiceInput.md](VoiceInput.md) | Universal voice input CLI reference, streaming mode toggle, dotool injection, and audio DSP pipeline |
| [VoiceInputArchitecture.md](VoiceInputArchitecture.md) | ADR: Multi-tier architecture, evdev physical modifier key gating daemon, and GNOME Shell companion extension |
| [TypingLayoutArchitecture.md](TypingLayoutArchitecture.md) | Why typed text can have Y/Z swapped: dotool vs compositor layout, active GNOME input source detection, standalone-dotool vs `dotoold` restart trade-off, and the settings-save restart pitfalls (`try-restart`, `Key` enum) |
| [InstallationArchitecture.md](InstallationArchitecture.md) | Converged Go/Make/curl installation, optional modifier daemon, Podman verification boundaries, and installation pitfalls |
| [HeadlessTestingArchitecture.md](HeadlessTestingArchitecture.md) | Headless end-to-end container test harness, virtual PulseAudio mic streaming, persistent FIFO typing sinks, and host uinput isolation |
| [ASREngines.md](ASREngines.md) | The three engine shapes (voxtype, crispasr, HTTP) and who owns each process, spec-driven per-model endpoint/response format, R2T2 via llama-server incl. the iGPU VRAM cap, keyterm biasing, and the no-speech sentinel |
| [BenchBaseline.md](BenchBaseline.md) | `voxi bench` CPU vs GPU RTF baseline for an average dev machine (T14 Gen2 AMD) |
| [PublicationAndRelease.md](PublicationAndRelease.md) | Codeberg hosting philosophy, local CI quality gates, and release workflow |
| [ChunkDiagnostics.md](ChunkDiagnostics.md) | `voxi chunks` ring buffer, RMS/acoustic-gate fields, the Braille LEVEL sparkline (dual-column packing, log-scale calibration pitfalls), and `--color` |
| [LiveMicMeter.md](LiveMicMeter.md) | `audiolevel` package: capture, two-sided ballistics easing, decoupled paint/collect/capture cadences, truecolor gradient rendering, GNOME mic-indicator suppression, the `stty`-subprocess perf pitfall, the rolling sparkline export and its `examples/miclevel` loom TUI demo, and `loom` API pitfalls found along the way |
| [EagerDeliverySafety.md](EagerDeliverySafety.md) | Eager chunk identities, at-most-once delivery ledger, stop/queue semantics, visible failures, and conservative transcript safety limits |
| [MicSelfCheck.md](MicSelfCheck.md) | Mic self-check run alongside each recording (issue 177): `pw-dump` diagnosis, stall/zero signal judgement, narrow repairs, WirePlumber Bluetooth autoswitch pitfalls, `voxi mic` |
| [LLMTranscriptCleanup.md](LLMTranscriptCleanup.md) | Local LLM cleanup request contract, chunk context, fallback behavior, and real-model validation limits |
| [TTSReading.md](TTSReading.md) | `voxi say` narration, lmcoder hosts and sessions, chunk queue behavior, pause trimming, and observed model latency |
| [SampleStore.md](SampleStore.md) | Decision record (issue 169): chunk vs sample glossary, `dictation`/`noise`/`voice` purposes, sample store layout, `voxi sample` command map, migration plan, implementation notes (all shipped) |

---

## Language & Engineering Guides

- [Go Conventions](Go.md)
- [Makefile Conventions](Make.md)
- [Git Conventions](Git.md)
- [Markdown Conventions](Markdown.md)
- [Canary-First Development](Canary.md)
- [Spec-Driven Development](Spec.md)

---

## Technical Case Studies (**`docs/studies/`**)

| File | Topic |
|------|-------|
| [studies/2026-08-18-continuous-eager-streaming-and-resource-monitor.md](studies/2026-08-18-continuous-eager-streaming-and-resource-monitor.md) | Continuous Eager Sentence Streaming, AMD Radeon Vulkan 1.4 GPU acceleration, plosive & stop-consonant audio protection, zombie pipeline teardown, and Option A Btop Grid Resource Monitor TUI |
| [studies/2026-08-18-gnome-shell-companion-and-devkit-testing.md](studies/2026-08-18-gnome-shell-companion-and-devkit-testing.md) | GNOME 45–50 companion extension, live nested devkit canary testbed, Mutter window focus coordination, and zero-leak recording toggle |
| [studies/2026-08-18-modifier-key-gating-and-system-daemon.md](studies/2026-08-18-modifier-key-gating-and-system-daemon.md) | Physical Modifier Key Gating, Dedicated Daemon Architecture, and Synthetic Input Safety |
| [studies/2026-08-18-omarchy-voxtype-configuration.md](studies/2026-08-18-omarchy-voxtype-configuration.md) | Omarchy 4.0.0 ("Quattro") Arch Linux distribution, Voxtype voice-to-text daemon (`peteonrails/voxtype`), Hyprland Wayland compositor integration, systemd user services, output drivers, status bars, and audio feedback. |
| [studies/2026-08-18-standalone-voxi-extraction-and-migration.md](studies/2026-08-18-standalone-voxi-extraction-and-migration.md) | Standalone Voice Input Engine Extraction (`ubunatic/voxi`) & Decoupling |
| [studies/2026-08-18-voxtype-popular-applications.md](studies/2026-08-18-voxtype-popular-applications.md) | Linux Wayland/X11 compositors (Hyprland, Sway, GNOME, KDE, River), status bars (Waybar, Polybar), editor workflows (Obsidian, Neovim), LLM post-processing (Ollama), meeting pipelines, and engine setups (Whisper, Parakeet, Soniox). |
| [studies/2026-08-24-single-agent-mode-orchestration.md](studies/2026-08-24-single-agent-mode-orchestration.md) | Single Agent Mode Orchestration |
| [studies/2026-09-07-fluidvoice-review-and-chunk-diagnostics.md](studies/2026-09-07-fluidvoice-review-and-chunk-diagnostics.md) | FluidVoice Prior-Art Review, Chunk Diagnostics, and Three Rounds of Calibration |
| [studies/2026-09-09-spec-audit-and-cross-agent-implementation.md](studies/2026-09-09-spec-audit-and-cross-agent-implementation.md) | Spec/Code-Quality Audit, Roadmap Reconciliation, and Cross-Agent Implementation & Review |
| [studies/2026-09-13-llm-cleanup-evaluation.md](studies/2026-09-13-llm-cleanup-evaluation.md) | LLM cleanup transcript fidelity evaluation |
| [studies/2026-09-15-agy-cleanup-latency-and-session-strategies.md](studies/2026-09-15-agy-cleanup-latency-and-session-strategies.md) | agy (Antigravity) subprocess latency for LLM transcript cleanup: cold-spawn vs -c vs persistent stream-JSON, the ~13k-token fixed tool-schema tax, and the local-model near-miss timeout finding that came from live dictation during the same investigation |
| [studies/2026-09-26-ansi-design-tour-and-compact-monitor.md](studies/2026-09-26-ansi-design-tour-and-compact-monitor.md) | Terminal User Interface (TUI), ANSI design prototyping, `voxi monitor --compact`, visual CLI testing, display width calculation, Go text layout engines, and Web App comparative engineering effort. |
| [studies/2026-09-26-linux-desktop-tts-reading-pipeline.md](studies/2026-09-26-linux-desktop-tts-reading-pipeline.md) | Text-to-Speech (TTS) engine, GNOME Wayland selection reading hotkeys, LLM document narration, pip-free native Piper ONNX runtime, multi-voice library, and persistent systemd daemon queue ownership with STT recording epoch muting. |
| [studies/2026-09-27-local-voice-cloning-research.md](studies/2026-09-27-local-voice-cloning-research.md) | Local voice cloning research (2026-09-27) |


<!-- End of studies index. -->
