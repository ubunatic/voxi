# Voxi Documentation

In-depth references for architecture, decisions, and operations of the Voxi Linux Voice Input & Synthetic Typing Engine.

| Document | Description |
| :--- | :--- |
| [VoiceInput.md](VoiceInput.md) | Universal voice input CLI reference, streaming mode toggle, dotool injection, and audio DSP pipeline |
| [VoiceInputArchitecture.md](VoiceInputArchitecture.md) | ADR: Multi-tier architecture, evdev physical modifier key gating daemon, and GNOME Shell companion extension |

---

## Language & Engineering Guides

- [Go Conventions](Go.md)
- [Makefile Conventions](Make.md)
- [Git Conventions](Git.md)
- [Markdown Conventions](Markdown.md)
- [Canary-First Development](Canary.md)
- [Spec-Driven Development](Spec.md)

---

## Technical Case Studies (`docs/studies/`)

| Case Study | Topic |
| :--- | :--- |
| [2026-08-18-standalone-voxi-extraction-and-migration.md](studies/2026-08-18-standalone-voxi-extraction-and-migration.md) | Architectural extraction of the voice input engine from harnez into standalone voxi |
| [2026-08-18-continuous-eager-streaming-and-resource-monitor.md](studies/2026-08-18-continuous-eager-streaming-and-resource-monitor.md) | Continuous eager sentence streaming, GPU acceleration, and btop TUI dashboard |
| [2026-08-18-modifier-key-gating-and-system-daemon.md](studies/2026-08-18-modifier-key-gating-and-system-daemon.md) | Physical modifier key gating, dedicated evdev daemon architecture, and synthetic input safety |
| [2026-08-18-gnome-shell-companion-and-devkit-testing.md](studies/2026-08-18-gnome-shell-companion-and-devkit-testing.md) | GNOME Shell companion extension and devkit/nested Wayland testing harness |
| [2026-08-18-voxtype-popular-applications.md](studies/2026-08-18-voxtype-popular-applications.md) | Survey of speech recognition models and Wayland input backends in production Linux apps |
| [2026-08-18-omarchy-voxtype-configuration.md](studies/2026-08-18-omarchy-voxtype-configuration.md) | Omarchy 4.0.0 voice dictation reference architecture and config breakdown |
