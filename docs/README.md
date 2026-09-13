# Voxi Documentation

In-depth references for architecture, decisions, and operations of the Voxi Linux Voice Input & Synthetic Typing Engine.

| Document | Description |
| :--- | :--- |
| [VoiceInput.md](VoiceInput.md) | Universal voice input CLI reference, streaming mode toggle, dotool injection, and audio DSP pipeline |
| [VoiceInputArchitecture.md](VoiceInputArchitecture.md) | ADR: Multi-tier architecture, evdev physical modifier key gating daemon, and GNOME Shell companion extension |
| [InstallationArchitecture.md](InstallationArchitecture.md) | User-scoped `voxi install`, optional privileged modifier-daemon setup, service activation, and installation failure lessons |
| [HeadlessTestingArchitecture.md](HeadlessTestingArchitecture.md) | Headless end-to-end container test harness, virtual PulseAudio mic streaming, persistent FIFO typing sinks, and host uinput isolation |
| [BenchBaseline.md](BenchBaseline.md) | `voxi bench` CPU vs GPU RTF baseline for an average dev machine (T14 Gen2 AMD) |
| [PublicationAndRelease.md](PublicationAndRelease.md) | Codeberg hosting philosophy, local CI quality gates, and release workflow |
| [ChunkDiagnostics.md](ChunkDiagnostics.md) | `voxi chunks` ring buffer, RMS/acoustic-gate fields, the Braille LEVEL sparkline (dual-column packing, log-scale calibration pitfalls), and `--color` |
| [LiveMicMeter.md](LiveMicMeter.md) | `audiolevel` package: capture, two-sided ballistics easing, decoupled paint/collect/capture cadences, truecolor gradient rendering, GNOME mic-indicator suppression, the `stty`-subprocess perf pitfall, the rolling sparkline export and its `examples/miclevel` loom TUI demo, and `loom` API pitfalls found along the way |
| [EagerDeliverySafety.md](EagerDeliverySafety.md) | Eager chunk identities, at-most-once delivery ledger, stop/queue semantics, visible failures, and conservative transcript safety limits |

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

