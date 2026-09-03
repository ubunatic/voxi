# 048: Injection Fallback Chain for TypeText (wtype / clipboard-paste)

**Status**: Proposed
**Priority**: P3 (Low)
**Severity**: Minor
**Category**: Enhancement
**Related**: [047 OSS voice-typing tool landscape research](047-oss-voice-typing-tool-landscape-research.md) (Section 5.1/5.2 — Vocalinux comparison)

---

## 1. Problem & Motivation

Issue 047's research compared Voxi's keystroke-injection design against
Vocalinux, currently the strongest OSS peer surveyed. Vocalinux auto-detects
and falls back across multiple injection strategies (IBus, `wtype`,
`ydotool`-via-clipboard, an explicit terminal paste path), while Voxi
commits to a single path: `internal/typing.TypeText` requires `dotool`/
`dotoold` and returns a hard error if neither is available
(`internal/typing/typing.go:78-80`). There is a separate, unrelated
`CopyText` helper (`typing.go:88-96`) that shells out to `wl-copy`, but
`TypeText` never falls back to it — if `dotool` is missing, dictation simply
fails with no automatic degradation to "at least get the text somewhere
useful."

This is a low-priority robustness gap, not a broken feature: `dotool` is
Voxi's documented, intentional dependency, and this ticket does not propose
replacing it as the primary path.

## 2. Desired Design

Add one additional fallback strategy to `TypeText`, tried only after the
existing `dotoolc`-then-`dotool` attempts both fail to find their binary:

1. `dotoolc` (existing, unchanged, tried first).
2. `dotool` (existing, unchanged).
3. **New**: `wtype`, if present on `PATH` — a native Wayland
   virtual-keyboard-protocol injector, structurally the same class of tool
   as `dotool` but with different compositor support characteristics; worth
   trying as a like-for-like keystroke-injection fallback rather than a
   degraded one.
4. **New**: if none of the above are available, copy the text to the
   clipboard via the existing `CopyText`/`wl-copy` path and return a
   distinguishable "typed via clipboard fallback, press paste" outcome
   (exact signaling mechanism — a typed error wrapping a sentinel, a bool
   return, or a caller-visible notification — is an implementation
   decision; whatever it is, the eager pipeline's caller needs to be able to
   tell "text was injected" apart from "text is sitting in the clipboard,
   user must paste manually" so it doesn't silently claim success).

## 3. Implementation Plan

1. Canary-first: confirm `wtype` behaves as expected for the same test
   utterances `dotool`'s canary used, on this machine's compositor, before
   wiring it into `TypeText`.
2. Extend `TypeText` in `internal/typing/typing.go` with the two new
   fallback branches after the existing `dotool` check (around line 78-84),
   reusing `BuildDotoolCommands`'s already-built command stream only if
   `wtype`'s command syntax is compatible — otherwise add a small
   `wtype`-specific command builder.
3. Decide and implement the caller-visible signal for the clipboard-fallback
   outcome; update whatever caller in `internal/eager` currently treats a
   `TypeText` return as "the utterance was delivered" so it can distinguish
   the clipboard-fallback case (e.g. skip marking the utterance "typed" in
   history, or log/notify differently).
4. Unit tests: each fallback tier triggers correctly when earlier tools are
   unavailable (mock `LookPath`), and the clipboard-fallback signal is
   distinguishable by the caller.

## 4. Acceptance Criteria

- `TypeText` no longer hard-fails when `dotool`/`dotoold` are both absent
  but `wtype` or `wl-copy` are available.
- The existing `dotoolc`/`dotool` path is unchanged in priority and
  behavior when either is available (no regression to the common case).
- The clipboard-fallback outcome is distinguishable from a real keystroke
  injection by the caller — no silent "success" claim when the user still
  has to manually paste.
- `go test ./...`, `make check`, `make install` pass.

## 5. Non-goals

- No IBus-based injection route (Vocalinux's other fallback) — out of scope
  for this ticket; `dotool`/`wtype`/clipboard is a smaller, sufficient step.
- No change to `dotool`/`dotoold` remaining the primary, default path.
- No terminal-specific paste-cue handling (Vocalinux's `Ctrl+Shift+V`
  special-case) — not addressed here.
- No X11 injection path — Voxi is Wayland-only per its project scope.
