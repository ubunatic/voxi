# Voxi Issues & Feature Roadmap

| # | File | Title | Status |
|---|------|-------|--------|
| 001 | [001-website-integration.md](001-website-integration.md) | 001: Website Integration and Public Documentation | In Progress |
| 021 | [021-fluent-streaming-typing.md](021-fluent-streaming-typing.md) | Fluent, streaming voice-input typing with enter-to-stop | Research spike complete — local streaming proven viable and working on this workstation (opt-in, not shipped as a `harnez tools` feature). See Findings below. |
| 022 | [022-gnome-transcriber-ui.md](022-gnome-transcriber-ui.md) | GNOME transcriber UI: history, retype, and typing-speed controls | Complete — CLI plumbing, tests, canary probes, and GNOME Shell companion extension implemented |
| 024 | [024-gnome-typing-feedback-icon.md](024-gnome-typing-feedback-icon.md) | GNOME transcriber UI: Visual typing feedback indicator (keyboard icon during synthesis) | Open — P4 Low, extension unused; Enhancement to issue 022 |
| 025 | [025-voice-input-volume-animation.md](025-voice-input-volume-animation.md) | Voice Input: Real-time audio input volume / VU meter animation in recording indicator | Open — P4 Low, extension unused; Enhancement to issues 022 & 024 |
| 026 | [026-continuous-eager-sentence-streaming.md](026-continuous-eager-sentence-streaming.md) | Continuous Eager Sentence Streaming Dictation | Implemented (Go Harnez Pipeline) |
| 027 | [027-continuous-listening-wake-word-turn-taking.md](027-continuous-listening-wake-word-turn-taking.md) | Continuous Listening, Wake-Word Activation & Verbal Turn-Taking | Proposed / Research & Planning |
| 028 | [028-post-process-local-llm-cleanup.md](028-post-process-local-llm-cleanup.md) | Local LLM Post-Process Hook for Dictation Cleanup & Voice Commands | Proposed / Research & Design |
| 029 | [029-single-voxi-agent-mode-orchestration.md](029-single-voxi-agent-mode-orchestration.md) | Single Voxi Agent for Mode Orchestration | In Progress / Architecture Design; docs, systemd unit, and install slice started |
| 030 | [030-public-readiness-checklist.md](030-public-readiness-checklist.md) | 030: Public Release Readiness & Publication Checklist | Complete |
| 031 | [031-claude-code-and-agent-cli-voice-pipeline-research.md](031-claude-code-and-agent-cli-voice-pipeline-research.md) | 031: Claude Code and agent CLI voice pipeline research | Proposed / Research |
| 032 | [032-small-en-project-vocabulary-biasing.md](032-small-en-project-vocabulary-biasing.md) | 032: Small.en Project Vocabulary Biasing for Technical Dictation | Implemented / Opt-in Canary |
| 033 | [033-user-stop-word-feedback.md](033-user-stop-word-feedback.md) | 033: User Stop-Word Feedback for Dictation Hallucinations | Complete |
| 034 | [034-isolated-silence-artifact-feedback.md](034-isolated-silence-artifact-feedback.md) | 034: Isolated Silence-Artifact Feedback for Ambiguous Dictation Words | Complete |
| 035 | [035-ssh-remote-transcription-server-research.md](035-ssh-remote-transcription-server-research.md) | 035: SSH-Managed Remote Voxi Transcription Server Research | Proposed / Research |
| 036 | [036-modularize-monitor-collector-and-renderer.md](036-modularize-monitor-collector-and-renderer.md) | 036: Modularize Monitor Collector and TUI Renderer | Complete — split into collector.go/render.go/monitor.go |
| 037 | [037-code-quality-and-test-coverage-roadmap.md](037-code-quality-and-test-coverage-roadmap.md) | 037: Comprehensive Code Quality, Test Coverage, and Modularization Plan | Open |
| 038 | [038-vocabulary-feedback-command.md](038-vocabulary-feedback-command.md) | 038: Vocabulary Feedback Command | Closed — resolved in cb8b214 |
