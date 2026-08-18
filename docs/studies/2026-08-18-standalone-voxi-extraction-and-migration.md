# Case Study: Standalone Voice Input Engine Extraction (`ubunatic/voxi`) & Decoupling

- **Date:** 2026-08-18
- **Context:** Architectural extraction of the Linux voice input stack from `harnez` into `ubunatic/voxi` (Issue 029).
- **Primary Reference:** [docs/VoiceInput.md](../VoiceInput.md), [docs/VoiceInputArchitecture.md](../VoiceInputArchitecture.md).

---

## 1. Header & Context

Over issues 020 through 028, Harnez developed a comprehensive voice input engine for Linux/Wayland: continuous eager sentence streaming with rolling Whisper inference, physical modifier key gating daemon (`harnez-modifierd`), direct synthetic keystroke injection via `dotoolc`, btop-styled TUI latency monitor, and a GNOME Shell companion extension.

While highly performant, this stack introduced kernel input drivers, system-scope root daemons, audio DSP pipelines, and GPU Vulkan monitors into Harnez—a codebase whose core mission is declarative AI agent harness management (`settings.json`, `AGENTS.md`, multi-agent token quota tracking).

The goal of this session was to cleanly extract the entire voice stack into a dedicated, standalone repository [**`ubunatic/voxi`**](https://github.com/ubunatic/voxi) (`voxi`), migrate running services, update desktop keybindings, and restore Harnez to its lean declarative focus.

---

## 2. Executive Summary

- **Extracted Standalone Repository (`voxi`)**: Initialized Go module `ubunatic.com/voxi` with `cmd/voxi` and `cmd/voxi-modifierd`. Migrated audio DSP ring buffers, Whisper/Parakeet normalization, evdev modifier daemon (`voxi-modifierd`), typing engine with modifier gating and clipboard fallback, btop-styled TUI monitor dashboard, GNOME companion extension (`voxi@ubunatic.com`), systemd unit definitions, canaries, and documentation.
- **Decoupled Harnez**: Completely removed `internal/tools/` and `spec/` from `harnez`. Cleaned CLI down to core agent management (`apply`, `init`, `diff`, `clean`, `status`, `usage`).
- **Live System Migration**: Replaced running `harnez-modifierd.service` with `voxi-modifierd.service` (monitoring 9 evdev keyboards with sub-10ns gating). Updated GNOME custom shortcut `<Super>x` to call `/home/uwe/go/bin/voxi record toggle`.
- **Public & Workspace Integration**: Registered `voxi` in root `.uman.toml`, created initial landing page `website/index.html`, synced to `ubunatic.com/voxi`, and initialized `issues/001-website-integration.md`.

---

## 3. What Worked Well

1. **Subagent Delegation for Repository Bootstrapping**: Spawning a dedicated self subagent to scaffold `voxi`, migrate audio/typing packages, and adjust imports allowed rapid parallel execution without context fragmentation.
2. **Clean Domain Boundary**: By making `voxi` independent, its release lifecycle and packaging (`goreleaser`, RPM, AUR) are cleanly separated from agent harness tooling.
3. **Multi-Repo Scripting Discipline**: Running repo-scoped commands explicitly (`cd /path/to/repo && ...`) ensured clean separation during multi-repo git operations.

---

## 4. Honest Post-Mortem (Failures, Bugs & Near-Misses)

### 4.1. Desktop Execution Environment PATH Stripping
- **Failure**: After updating the GNOME `<Super>x` shortcut to `/home/uwe/go/bin/voxi record toggle`, pressing the key failed to start recording.
- **Investigation via Journal**: Checking `journalctl --user` revealed:
  ```text
  voxi[835589]: Error: voxtype not found on PATH: exec: "voxtype": executable file not found in $PATH
  ```
  `gsd-media-keys` executes commands in a minimal environment (`PATH=/usr/local/bin:/usr/bin`), omitting `~/.local/bin` and `~/go/bin` where user-installed binaries reside.
- **Fix**: Added fallback binary resolution in `voxi/internal/deps/deps.go` (`lookPathWithFallbacks`) that checks `~/.local/bin` and `~/go/bin` when standard `exec.LookPath` fails in desktop environments.

### 4.2. Incomplete Initial Decoupling in Harnez
- **Near-Miss**: The initial extraction left `harnez tools voice-input` as a wrapper command in Harnez.
- **Resolution**: Through user alignment, we executed a total removal of the `tools` subsystem in Harnez. This prevented lingering domain confusion and cut over 1,100 lines of dead code and obsolete schemas.

---

## 5. Quality & Invariants Audit

| Invariant / Metric | Result | Verification Method |
|---|---|---|
| **Zero Voice Code in Harnez** | Passed | Recursive grep for `voice`/`voxtype` in Harnez returned 0 hits in active code. |
| **Voxi Test Suite** | Passed | `go test ./...` and `go test -tags debug ./...` pass with 0 errors. |
| **Harnez Test Suite** | Passed | `go test ./...` runs in <15ms with 0 errors. |
| **Modifier Daemon Live State** | Active | `voxi-modifierd.service` running active on `/run/voxi/modifiers` (mode `0644`). |
| **Desktop Keybinding** | Verified | `<Super>x` bound to `/home/uwe/go/bin/voxi record toggle`. |
| **Workspace Glue (`uman`)** | Verified | `voxi` added to `.uman.toml`; site synced to `ubunatic.com/voxi`. |

---

## 6. Efficiency & Velocity Assessment

- **Extraction & Decoupling Duration**: ~45 minutes total for full repo scaffolding, package migration, live service cutover, bugfix, documentation transfer, and archive reconciliation.
- **Agentic Flow Efficiency**: Immediate reactive wakeup between parent agent and subagent enabled concurrent scaffolding while maintaining full traceability in the main thread.

---

## 7. Key Learnings & Evergreen Upstream

1. **Desktop Shortcuts & Minimal PATHs**: Any CLI utility intended for global desktop shortcut binding (`gsd-media-keys`, Hyprland, Sway) must account for non-interactive PATH restrictions or provide built-in `~/.local/bin` / `~/go/bin` lookups.
2. **Decouple Early Before Architectural Drift Hardens**: Extracting the system daemon and audio DSP code out of the harness tool restored single-responsibility clarity to both projects.

---

## 8. File & Diff Summary

### Voxi (`ubunatic/voxi`)
- Scaffolding & Packages: `cmd/voxi`, `cmd/voxi-modifierd`, `internal/audio`, `internal/asr`, `internal/modifiers`, `internal/typing`, `internal/eager`, `internal/monitor`, `internal/record`, `internal/deps`, `internal/debug`.
- Companion & Units: `contrib/gnome-shell-extension`, `systemd/voxi-modifierd.service`, `systemd/voxi-eager.service`.
- Canaries & Scripts: `scripts/canary_nested/main.go`.
- Documentation & Web: `docs/VoiceInput.md`, `docs/VoiceInputArchitecture.md`, `docs/studies/`, `website/index.html`, `issues/`.

### Harnez (`ubunatic/harnez`)
- Removed `internal/tools/` and `spec/tools/voice-input.yaml`.
- Removed `scripts/canary_nested/`.
- Archived `issues/020-tools-command-os-tools.md` and `issues/029-extract-ubunatic-voxi-standalone.md`.
