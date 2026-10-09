# 186 — Build a cross-platform GUI demo for voxi features

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Feature
**Related**:

---

## 1. Problem & Motivation
Voxi's Go code, models, and logic need an approachable cross-platform front end (Linux, macOS,
Windows) for demonstrating and exercising the project's features, including for non-technical users.

/goal Choose a cross-platform Go SDK and ship a first running loom-based TUI prototype (gate 1);
only then build the real GUI (gate 2). Stop and report when blocked on a user decision (e.g. SDK
choice) or a denied permission.

## 2. Requirements
1. As much of the app as possible is Go.
2. All OS/platform interaction goes through an established SDK such as SDL (e.g. via Go bindings).
   No Python, no JS, no Java.
3. The SDK must make it easy to access the host's default microphones and media controls.
4. v1 scope: (a) speech input and output, (b) start/stop model servers, (c) manage voxi settings
   and features. Host keyboard access (typing/injection, modifier gating) is out of scope for v1.
5. Prerequisite: the GUI assumes a working voxi install and, if needed, extends/enhances the install
   paths: (1) curl installer, (2) `go install`, (3) a Windows/macOS install path for non-techies.
6. Prototype first as a loom-based TUI (ubunatic.com/loom). The chosen SDK must work with both TUIs
   and GUIs while the Go logic runs in the background.
7. TUI prototypes are `.ansi` files: inspectable with `cat` and verified with the loom CLI
   (`measure`/`eval`).

## 3. Plan & Gates
- **M1 — SDK choice**: compare candidates against requirements 1–3 and 6 (mic, media controls,
  TUI+GUI coexistence, Go bindings, platform support). Present a recommendation; the user picks.
- **M2 — TUI prototype**: `.ansi` mockups for scope 4a–4c, verified with loom `measure`/`eval`,
  then a first running loom TUI wired to voxi's Go code.
- **M3 — GUI (gate)**: starts only after M1 is decided and M2 runs. Build the GUI on the chosen SDK
  and verify builds and behavior per platform; record features that cannot be exercised via the GUI.

## 4. Acceptance
- SDK choice recorded in this ticket with rationale.
- `.ansi` prototypes committed and passing loom `measure`/`eval`; TUI runs against a real voxi install.
- Install prerequisites (requirement 5) checked; gaps filed or fixed.
