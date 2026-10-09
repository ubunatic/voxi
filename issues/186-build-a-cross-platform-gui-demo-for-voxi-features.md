# 186 — Build a cross-platform GUI demo for voxi features

**Status**: Blocked — awaiting user decisions on TUI, monitor, settings and media scope
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Feature
**Related**: [lmcoder #174 — cross-platform builds](../lmcoder/issues/174-provide-cross-platform-lmcoder-builds-for-linux-macos-and-windows.md)

---

## 1. Problem & Motivation
Voxi's Go code, models, and logic need an approachable cross-platform front end (Linux, macOS,
Windows) for demonstrating and exercising the project's features, including for non-technical users.

/goal Deliver and verify a loom-based TUI prototype, settle how it relates to `voxi monitor` and
`voxi settings`, and only then begin the real GUI. Stop and report when blocked on a user decision
or a denied permission.

## 2. Requirements
1. As much of the app as possible is Go.
2. All OS/platform interaction goes through an established SDK such as SDL (e.g. via Go bindings).
   No Python, no JS, no Java.
3. The SDK must make it easy to access the host's default microphones and media controls.
4. v1 scope: (a) speech input and output, (b) start/stop model servers through their owning tool,
   (c) manage voxi settings and features. `lmcoder` is a required companion for its model-server
   workflows; detect/report missing tools and use supported `lmcoder` lifecycle commands rather than
   controlling its services directly. Host keyboard access (typing/injection, modifier gating) is
   out of scope for v1.
5. Prerequisite: the GUI assumes a working voxi install and, if needed, extends/enhances the install
   paths: (1) curl installer, (2) `go install`, (3) a Windows/macOS install path for non-techies.
6. Prototype first as a loom-based TUI (ubunatic.com/loom). The chosen SDK must work with both TUIs
   and GUIs while the Go logic runs in the background.
7. TUI prototypes are `.ansi` files: inspectable with `cat` and verified with the loom CLI
   (`measure`/`eval`).
8. `lmcoder` is a required tool for its model-server workflows and must provide Linux, macOS, and
   Windows builds (tracked in [lmcoder issue 174](../lmcoder/issues/174-provide-cross-platform-lmcoder-builds-for-linux-macos-and-windows.md)).

## 3. Plan & Gates
- **M1 — SDK choice (decided)**: use Fyne for the Go GUI and malgo (miniaudio) for default
  microphone/speaker access. Selected for Go-native APIs and established cross-platform support;
  target Linux, macOS and Windows from the first GUI release. Loom remains the TUI renderer, with
  both front ends intended to share Go feature/service logic. Fyne/malgo do not provide host media
  controls; investigate per-platform integrations rather than treating in-app buttons as equivalent.
- **M2 — TUI prototype**: use the host-drafted visual specs in `docs/data/voxi-tui-*.ansi`, map
  server lifecycle and audio paths, and build a first running loom TUI wired to voxi's Go code.
  Each mockup must pass loom `measure`/`eval` and `check-box`. Detect the required `lmcoder` tool and
  use its supported lifecycle commands for its model servers. Record unresolved platform/API or
  user decisions in this ticket before crossing the next gate.
- **M3 — GUI (gate)**: starts only after M1 is decided and M2 runs. Build the GUI on Fyne with
  malgo audio and verify builds and behavior on Linux, macOS and Windows; implement or explicitly
  resolve host media-control coverage on each platform and record features that cannot be exercised.

## 4. Acceptance
- SDK choice recorded in this ticket with rationale.
- `.ansi` prototypes committed and passing loom `measure`/`eval`; TUI runs against a real voxi install.
- Install prerequisites (requirement 5) checked; gaps filed or fixed.

## 5. Handoff
- The SDK decision is Fyne + malgo; Linux, macOS, and Windows are all target platforms. `lmcoder`
  is a required companion; cross-platform builds are tracked in lmcoder issue 174.
- The host-drafted speech, servers, and settings mockups are committed under `docs/data/` and passed
  loom `measure`, `eval`, and `check-box`.
- A first loom TUI and shared frontend layer exist in `examples/voxi-tui/` and `internal/frontend/`.
  The current source snapshot passed `make test-q1` during wrap, and the worker reported successful
  live rendering and `make install`; this is not final acceptance or a GUI implementation.
- Before continuing, the user must decide whether the new UI starts as `voxi tui` or becomes a
  future `voxi monitor` replacement, whether `voxi settings` remains standalone, whether the TUI
  needs host media controls or only Linux status, and whether settings edit/reset must work in M2.
  The architecture recommendation is to share the service layer, keep existing commands unchanged,
  finish the TUI, and migrate monitor panels only after parity.
- Known prototype gaps: settings edit/reset are placeholders; some settings/ASR readiness labels do
  not reflect authoritative settings or local server state; transcript and mic level are placeholders.
  Correct misleading states and settle scope before treating M2 as complete. Do not begin GUI work
  or close this issue until those decisions and acceptance criteria are resolved.
