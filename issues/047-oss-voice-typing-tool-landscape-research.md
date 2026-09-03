# 047: Research the Best Current OSS Voice-Typing Tool and How It Works

**Status**: Research Complete
**Priority**: P3 (Low)
**Severity**: Informational
**Category**: Research
**Related**: [039 OSS STT landscape and custom-vocabulary research](039-oss-stt-landscape-and-custom-vocabulary-research.md) (engine/backend-level survey), [031 Claude Code and agent CLI voice-pipeline research](031-claude-code-and-agent-cli-voice-pipeline-research.md)

---

## 1. Problem & Motivation

Issue 039 surveyed STT *engines/backends* (whisper.cpp, faster-whisper, NeMo
Parakeet, sherpa-onnx, etc.) and how custom vocabulary is handled at the
decoder level. It deliberately did not evaluate full end-to-end open-source
*voice-typing tools* — the complete product: capture trigger (push-to-talk,
VAD-driven continuous, hotword), daemon/session architecture, keystroke
injection method, desktop integration (X11/Wayland), and UX conventions
(streaming partials vs. batch-on-release, correction workflow, command mode).

Voxi is itself one such tool. Understanding the strongest current OSS
competitor/prior-art in this space — not just its STT backend, but its whole
architecture and UX — gives a concrete reference point for evaluating Voxi's
own design choices (eager streaming, evdev modifier gating, `dotool`
injection, GNOME Shell companion extension) against what else exists.

## 2. Research Questions

1. What is currently the strongest, most actively maintained open-source (or
   source-available) voice-typing/dictation tool for desktop Linux (Wayland
   and X11), and separately, is there a clearly stronger tool on another
   desktop OS (macOS/Windows) worth knowing about even if not directly
   portable? Candidates to check at minimum: `nerd-dictation`, Talon Voice
   (proprietary core but widely used, worth noting why it's excluded if so),
   FUTO Voice Input / FUTO Keyboard, `Speech Note`, `vosk`-based dictation
   utilities, whisper.cpp-based desktop wrappers (e.g. `whisper-typer`,
   `WhisperWriter`, `superwhisper`-alikes), and anything more recent
   (2025-2026) that supersedes these.
2. How does its capture/trigger architecture work — push-to-talk hotkey,
   continuous VAD-gated streaming, or hybrid? How does it decide when an
   utterance starts/ends?
3. How does it inject text into the focused application — clipboard paste,
   synthetic keystrokes (`ydotool`/`dotool`/`xdotool`/`wtype`), an IME/input-
   method integration, or an accessibility-API route? What Wayland-specific
   constraints did it have to solve?
4. Does it support custom/technical vocabulary, and if so, by what
   mechanism (initial-prompt, grammar, dictionary/lexicon, post-processing
   replacement rules)? Compare against Voxi's current approach (issue 032/046
   — Whisper initial-prompt biasing, now on by default).
5. What is its correction/editing UX — in-place text edit, undo-last-
   utterance, command-mode grammar for punctuation/formatting?
6. What is its licensing, maintenance activity (recent commits/releases),
   and community size, as a proxy for how much confidence to place in it as
   a reference architecture?

## 3. Deliverables

- A short comparison writeup (in this ticket, `## Research Findings`)
  covering the above questions for the single strongest candidate identified
  (deep dive), plus a brief comparison table against 2-4 runner-up
  candidates and against Voxi's current architecture.
- An explicit recommendation: any concrete architectural or UX idea worth a
  follow-up ticket for Voxi, or a note that Voxi's current design already
  compares favorably and no follow-up is warranted.

## 4. Non-goals

- No engine-level re-litigation of issue 039's STT backend survey — cite it
  rather than repeating it.
- No implementation in this ticket — research and a writeup only. Any
  adopted idea gets its own follow-up ticket, same pattern as 039 spawning
  040/041.
- No commitment to switching Voxi's architecture; this is prior-art research
  to inform, not a design mandate.

## 5. Research Findings (2026-09-02)

### 5.1 Strongest candidate: Vocalinux (deep dive)

**[Vocalinux](https://github.com/VocaHQ/vocalinux)** ("Free, open-source, 100%
offline voice dictation for Linux... works on X11 + Wayland") is the
strongest current OSS candidate for full end-to-end desktop Linux
voice-typing, and by a clear margin over the other candidates checked. It is
the closest thing in the OSS space to a direct architectural peer of Voxi:
a system-tray daemon that types into whatever application has focus, rather
than a self-contained note-taking workspace (Speech Note) or a mobile
keyboard component (FUTO). Repo facts as of this research pass (checked via
`gh api`): 793 stars, AGPL-3.0, created 2025-04-12, **last push
2026-09-02** (same day as this research), and **nightly releases published
daily** (`nightly-2026-09-02`, `nightly-2026-09-01`, ...) alongside tagged
releases (`v0.16.1`, 2026-08-30) — the most active release cadence of any
candidate surveyed.

- **RQ2 — Trigger architecture.** Hybrid, closest of any candidate surveyed
  to Voxi's own model. Default is push-to-talk (hold Right Alt/Option,
  release to stop), with an optional toggle mode. Layered on top, Vocalinux
  ships a **Silero VAD** (neural, ONNX-based) model that it uses — when
  `onnxruntime` is available — "to drop silence-only buffers before
  recognition," i.e. VAD is used as a pre-filter/quality gate around the
  hotkey-bounded capture window, not as Voxi's continuous always-listening
  utterance-boundary detector. This is architecturally weaker than Voxi's
  `internal/eager` design (continuous VAD-gated streaming with no key held
  down) but stronger than a plain fixed-window push-to-talk tool.
- **RQ3 — Injection method / Wayland constraints.** Multi-strategy with
  runtime auto-detection, the most sophisticated injection layer of any
  candidate surveyed: IBus-based injection is preferred when available
  (works across both X11 and Wayland via the same input-method bridge, and
  as of v0.10.1+ preserves the user's XKB keyboard layout rather than
  forcing IBus's default US layout — a real Wayland-specific bug they had to
  solve, structurally the same class of problem `internal/typing`'s dotool
  path and `internal/modifiers` gating solve for Voxi); `wtype` is used for
  native-Wayland virtual-keyboard-protocol compositors; `ydotool` pastes
  through the clipboard (layout-independent) as a further fallback; and
  terminals get an explicit `Ctrl+Shift+V` paste path since raw synthetic
  keystrokes are unreliable in terminal emulators. Voxi, by contrast, commits
  to a single injection path (`dotool`, gated by `internal/modifiers` evdev
  modifier-release waiting) plus a separate explicit clipboard fallback
  (`wl-copy`) — simpler and more predictable, but without Vocalinux's
  automatic multi-backend fallback chain or IBus route.
- **RQ4 — Custom vocabulary.** **None found.** A GitHub code search across
  the repo for `vocabulary` and `initial_prompt` returned zero matches, the
  FAQ documents no vocabulary/dictionary feature, and the only
  domain-adaptation hook mentioned anywhere in project docs is a generic
  "pipe transcription through an external command" post-processing step
  (which a user could wire to a local LLM for correction, but it is not a
  first-class biasing/vocabulary mechanism at all — it's just an arbitrary
  shell hook). This is a clear, concrete gap versus Voxi's issue-032/046
  `internal/speechcontext` initial-prompt biasing (explicit user vocabulary +
  static spec terms + repo-derived terms, on by default) — Vocalinux simply
  has no equivalent mechanism, category (a)/(b)/(c) alike.
- **RQ5 — Correction/editing UX.** Command-mode only, no in-place text
  editing surface. Documented voice commands: `new line`, `delete that`
  (deletes the last sentence), `select all`, `period`/`comma`/other
  punctuation insertion, `capitalize` (capitalizes the next word). Commands
  are optional and toggleable in Settings. There is no undo-last-utterance
  history stack beyond the single "delete that," and no interactive
  correction dialog — comparable in scope to what a `stopWords`/silence-
  artifact filter buys Voxi (`internal/eager`'s `acceptTranscript`), but
  Vocalinux's command grammar is more developed than anything currently in
  Voxi.
- **RQ6 — License/activity/community.** AGPL-3.0 (copyleft — worth noting
  explicitly since Voxi's own license posture should be checked for
  compatibility if any code/technique were ever borrowed, though this
  research found nothing borrowable at the source level, only architectural
  ideas). 793 stars, single primary maintainer ("Jatin K Malik" /
  VocaHQ), 55 open issues, daily nightly builds — small but unusually fast-
  moving community for a ~1.5-year-old project, and the most credible
  "actively maintained" signal of any Linux-native candidate surveyed.

### 5.2 Comparison table

| Tool | Trigger | Injection | Wayland | Vocabulary mechanism | License / activity |
|---|---|---|---|---|---|
| **Vocalinux** (strongest) | Push-to-talk hotkey (default) or toggle, + Silero VAD silence-filtering on top | Auto-detected: IBus (X11+Wayland) → `wtype` (native Wayland) → `ydotool`-via-clipboard fallback → explicit `Ctrl+Shift+V` paste in terminals | Native, actively worked on (XKB-layout-preserving IBus fix, per-compositor docs) | **None** — no biasing/dictionary/grammar mechanism found | AGPL-3.0; pushed same day as this research, nightly release cadence |
| `nerd-dictation` | Manual start/stop (external script/keybind calls `begin`/`end`; no built-in VAD) | Pluggable: `xdotool` (X11 default), or `dotool`/`ydotool`/`wtype` per its own `readme-ydotool.rst` for Wayland | Bolted on via alternate injection backends, not designed-in; Wayland issue #45 open since 2022 before workarounds landed | None built-in — VOSK backend *could* support grammar/dynamic vocab (see issue 039 findings) but nerd-dictation's own script doesn't expose it | MIT-style permissive; 1,914 stars (largest of any candidate) but last push 2025-10-10 — a single-file script effectively feature-frozen for years |
| Speech Note (`mkiol/dsnote`) | Manual record button / global shortcut (via XDG `GlobalShortcuts` portal on Wayland) | Not a system-wide "type into focused app" tool by default — primarily a standalone Qt note/translate workspace with its own text buffer; clipboard/export-oriented | Requires compositor XDG `GlobalShortcuts` portal support for shortcuts | None documented beyond engine defaults | MPL-2.0; 1,622 stars, pushed same day as this research — very active, but different product category (dictation notebook, not a keystroke-injecting typing daemon) |
| FUTO Voice Input | Tap-to-record (Android keyboard mic key) | N/A — Android IME text-commit API, not applicable to desktop injection | N/A (Android only) | None documented (whisper.cpp `initial_prompt`/grammar exist at the engine level per issue 039, but FUTO's app layer doesn't expose it) | Source-available (FUTO's own license, not OSI-approved); 319 stars, pushed 2025-09-16 — slower-moving, and out of scope as a desktop reference architecture |
| Talon Voice (reference only, not OSS) | Continuous listening + an extensive spoken command/grammar language | Native OS accessibility/injection APIs (platform-specific) | **Excluded from desktop-Linux comparison**: X11-only, Wayland explicitly not planned ("Wayland lacks the APIs Talon needs") | Command grammar + phonetic alphabet + user-defined vocabulary lists — most sophisticated of anything surveyed, but... | Proprietary core (free to use, source not open); worth knowing about for command-grammar UX ideas only, not as an architectural donor |

### 5.3 Comparison against Voxi's current architecture

Voxi's architecture — `internal/eager`'s continuous VAD-gated streaming
capture, `internal/modifiers`/`voxi-modifierd`'s kernel-evdev-level modifier
gating, `internal/typing`'s single-path `dotool` injection with a `wl-copy`
clipboard fallback, and `internal/speechcontext`'s on-by-default initial-
prompt vocabulary biasing (issues 032/046) — compares favorably against
Vocalinux, the strongest OSS peer found, on exactly the dimensions Voxi has
invested in deliberately: **Voxi is the only tool surveyed (OSS or
otherwise, Vocalinux and Talon included) with an on-by-default, layered
custom-vocabulary biasing mechanism** (explicit user terms + static spec
terms + repo-derived terms) — every other OSS candidate has none, and even
Talon's much richer command grammar doesn't automatically pull in
project-local terms the way Voxi's repo-derived vocabulary does. Voxi's
continuous eager-streaming trigger (no key held, no explicit start/stop
command) is also more advanced than every OSS candidate's capture model —
Vocalinux, the closest peer, still requires holding a key or toggling
state, and nerd-dictation requires external script-driven start/stop with
no VAD at all. Kernel-evdev modifier gating (`voxi-modifierd`) has no
equivalent anywhere in the survey; every other tool solves hotkey/typing
collisions at the compositor or IBus layer rather than at the kernel evdev
level.

Where Voxi does *not* yet compare favorably: Vocalinux's **injection layer
is more robust** — it auto-detects and falls back across IBus, `wtype`,
`ydotool`+clipboard, and an explicit terminal-paste path, while Voxi commits
to a single `dotool` path with one clipboard fallback and no runtime
auto-selection; a `dotool`-unavailable or misbehaving-compositor edge case
that Vocalinux's fallback chain absorbs silently could leave Voxi with a
harder failure. Voxi also has **no command-mode grammar** at all today
(punctuation-by-voice, "delete that", "select all", explicit
capitalize-next-word) — Vocalinux ships a modest but real one, and Talon's
is far more developed; Voxi's only analogous mechanism is the
`stopWords`/silence-artifact filter in `internal/eager`'s
`acceptTranscript`, which suppresses bad transcripts rather than offering
any spoken editing/correction vocabulary.

### 5.4 Recommendation

**One concrete follow-up ticket is warranted; no architecture change is
mandated by this research.**

- **Follow-up ticket idea — "Injection fallback chain for `internal/typing`"**:
  when `dotool`/`dotoold` is unavailable or a `TypeText` call fails, instead
  of surfacing an error, fall back to Vocalinux's pattern: try `wtype` (if
  present) for native-Wayland compositors, then fall back to the existing
  `wl-copy` clipboard path with a user-visible "paste with Ctrl+V" cue,
  mirroring Vocalinux's explicit terminal-paste handling. Concrete
  integration point: `internal/typing/typing.go`'s `TypeText`, right after
  the existing `dotoolDaemonReady`/`LookPath("dotool")` checks currently
  return a hard error — this is additive to the existing single-path design,
  not a replacement of `dotool` as the primary method.
- No follow-up is warranted for vocabulary/biasing, trigger architecture, or
  kernel-level modifier gating — this research found Voxi's current design
  in `internal/speechcontext`, `internal/eager`, and
  `internal/modifiers`/`voxi-modifierd` already exceeds every OSS candidate
  surveyed (including the strongest, Vocalinux) on those specific
  dimensions, so no architecture change is recommended there. A
  command-mode grammar (Talon/Vocalinux-style spoken punctuation/editing
  commands) is a plausible longer-term UX idea but is deliberately *not*
  proposed as a concrete follow-up ticket here — it's a larger, more
  speculative UX investment than the injection-fallback gap, and would
  benefit from its own dedicated design discussion rather than being filed
  reactively from this survey.

Sources: [VocaHQ/vocalinux](https://github.com/VocaHQ/vocalinux),
[vocalinux.com FAQ](https://vocalinux.com/faq/),
[vocalinux.com Wayland notes](https://vocalinux.com/wayland/),
[ideasman42/nerd-dictation](https://github.com/ideasman42/nerd-dictation),
[nerd-dictation Wayland support issue #45](https://github.com/ideasman42/nerd-dictation/issues/45),
[nerd-dictation readme-ydotool.rst](https://github.com/ideasman42/nerd-dictation/blob/main/readme-ydotool.rst),
[mkiol/dsnote (Speech Note)](https://github.com/mkiol/dsnote),
[Speech Note on Flathub](https://flathub.org/en/apps/net.mkiol.SpeechNote),
[futo-org/voice-input](https://github.com/futo-org/voice-input),
[FUTO Voice Input](https://voiceinput.futo.tech/),
[Talon Voice review noting Wayland exclusion](https://www.stork.ai/en/talon-voice),
[savbell/whisper-writer](https://github.com/savbell/whisper-writer) (checked,
last push 2024-08-24 — excluded from the comparison table as effectively
unmaintained relative to Vocalinux/Speech Note/nerd-dictation).
