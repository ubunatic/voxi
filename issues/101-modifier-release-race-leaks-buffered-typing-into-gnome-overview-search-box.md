# 101 — Modifier-Release Race Leaks Buffered Typing Into GNOME Overview Search Box

**Status**: Open
**Priority**: P1 (High)
**Severity**: Major
**Category**: Spec/Design
**Related**: [089 voxi-modifierd not installed](089-voxi-modifierd-not-installed-on-this-dev-machine-modifier-gating-currently-inactive.md), [083 reject runaway repeated dotool injection](083-prevent-runaway-repeated-dotool-desktop-injection.md), [092 eagerSessionManager.Toggle check-then-act race](092-eagersessionmanager-toggle-has-a-check-then-act-race-under-concurrent-sigusr1-socket-invocation.md)

---

## 1. Problem

Physical modifier-key gating (`voxi-modifierd`) correctly prevents dotool
from injecting keystrokes *while* a modifier (e.g. Super) is held down. This
was just verified installed and active in 089. But the gate is release-edge,
not target-window-aware: the instant the modifier is released, any buffered
eager-transcribed text fires immediately into whatever now has focus.

On GNOME, releasing the bare Super key (with no other key pressed) opens the
Shell Activities overview. That is the *same physical key-up event* that
ends voxi's modifier gate. So the release that lets voxi type and the
release that pops GNOME's overview are one and the same input event —
whatever text was buffered lands in the overview's search box instead of
the window the user actually meant to type into. This has been reproduced,
not hypothesized.

### Confirmed mechanism (current code)

- `internal/typing/typing.go` `TypeText()` (lines ~63-66) gates injection by
  calling `modifiers.WaitModifiersReleased(ctx, 5*time.Second)` immediately
  before building and submitting the `dotoolc`/`dotool` script. There is no
  delay, confirmation step, or secondary trigger between "modifiers read as
  released" and "keystrokes are sent."
- `internal/modifiers/modifiers.go` `WaitModifiersReleased()` polls the
  shared modifier-state file (`/run/voxi/modifiers`) on a 10ms ticker and
  returns as soon as the mask reads zero. So injection fires within ~10ms
  of the physical key-up — well inside the window where GNOME Shell is
  also reacting to that same key-up to raise the overview.
- The daemon (`internal/modifiers/modifiers.go`) tracks Ctrl/Alt/Super/Shift
  generically via `WatchedModifierKeys`; it has no concept of "this modifier
  release is also bound to a WM/DE action," nor does it need to for gating
  to work as designed — the gap is entirely in what happens *after* release
  is detected, not in detection itself.
- `internal/shortcut/shortcut.go` configures a GNOME custom keybinding for
  `<Super>x` (Super+X) that invokes voxi's own toggle command — a
  DE-level keybinding, independent of the evdev-based modifier daemon. This
  is the existing "explicit hotkey" plumbing referenced in the options
  below.

## 2. Practical Implication

Voxi behaves exactly as gating was specified — it is not injecting while a
modifier is held — and the leak still happens. This is a sharper version of
the injection-safety trust problem than 083: 083 is about pathological or
duplicated *content*; this is about *timing/target-window* safety at the
release edge itself. `docs/Roadmap.md`'s "Now" framing ("no pathological,
stale, or duplicated text is ever injected; physical modifier gating is
actually active; stopping means stopped") is not fully satisfied while a
correctly-gated release can still misdirect text into an unintended,
unknown-focus surface.

The GNOME-overview case is the reproduced instance, but the underlying gap
is general: *any* WM/DE/compositor binding that reacts to the release of a
gating modifier (Super or otherwise) — raising a launcher, switching
workspace, whatever — races voxi's flush the same way.

## 3. Scope

This is a **spec/design ticket, not an implementation ticket.** The intent
is to lay out candidate mechanisms so they can be prototyped and compared,
not to prescribe one. Do not implement any of the options below without
further decision.

Open questions to resolve before implementation, not answered here:

- Which modifiers should trigger buffering — Super specifically, or any
  modifier in `modifiers.WatchedModifierKeys` (configurable)?
- What should the exact flush trigger be, and where does it live relative
  to `TypeText`'s existing `WaitModifiersReleased` call?
- Which notification channel (if any) is appropriate, and does it require
  new plumbing or reuse of `voxi monitor`/the GNOME Shell extension?
- Does this interact with `internal/shortcut`'s existing Super+X binding,
  or does it need a distinct binding/signal path?
- Should buffering be a global always-on behavior, a config option, or opt-in?

## 4. Options to Prototype (none chosen — evaluate independently)

### Option A — App/compositor-specific detection

Best-effort detect "did releasing this modifier just open the GNOME
overview (or an equivalent WM/DE surface)" and suppress or delay typing for
that specific case.

Explicitly flagged as unsatisfying by design: it is narrow to one DE's
behavior, does not generalize to other window managers/compositors or their
own Super-triggered UI, and voxi's focus-detection has no reliable way to
know which application/surface holds focus immediately after a modifier
release. Listed for completeness, not favored.

### Option B — Buffered/batch eager mode while a gating modifier is held

While a gating modifier is detected pressed, keep transcribing eagerly but
buffer the output instead of streaming it to the injector. On release,
do **not** auto-flush from `WaitModifiersReleased` returning — instead
require a distinct, explicit trigger (e.g. reusing the existing Super+X
toggle shortcut from `internal/shortcut`, or a new dedicated binding) to
flush the buffered batch as one write.

This decouples "modifier physically released" (which may simultaneously
trigger unrelated WM/DE behavior) from "user has confirmed it is safe to
type now." Needs design for: how long buffering can persist, what happens
if the user never sends the flush trigger, and whether buffering should
also suppress normal (non-modifier-triggered) eager typing or only the
release-edge case.

### Option C — User feedback while buffering/stopped

However buffering is signaled, the user needs to know typing is paused
rather than actively streaming. Candidates, all open and unevaluated:

- **Literal placeholder text** (e.g. `!!!`) injected at the point where
  text would have gone, as a visible "something is different" cue. Cheap,
  but relies on the user noticing an unexplained artifact.
- **Spoken-style textual note** (e.g. "transcription stopped, press
  Super+X to finish") injected into the focused surface once a gating
  modifier-hold is detected. **Self-contradictory as stated** — this was
  flagged during design discussion: injecting *any* text into an
  unknown-focus window while gating is active is exactly the
  injection-safety problem this ticket exists to prevent. Recorded here as
  a rejected idea, not a viable option.
- **Non-injection notification channel** — the actual direction worth
  prototyping. Candidates raised: extending `voxi monitor`'s existing TUI
  status line, a desktop notification (`notify-send`-style), the GNOME
  Shell companion extension's own UI surface, or a distinct audible/visual
  cue. Working names floated for this mechanism during discussion:
  "transcribe-stop notification" and "modifier interceptor" — neither
  committed, listed here as terms to search for/reuse if either direction
  is prototyped.
- **Spoken (TTS) notification** — a variant of the non-injection channel
  above, verified feasible in this session: `spd-say` (speech-dispatcher)
  is installed and working, with `rhvoice` + `mbrola`/`mbrola-en1`
  installed and configured as a noticeably better-sounding voice than the
  stock `espeak-ng` module (enabled via a user-level
  `~/.config/speech-dispatcher/speechd.conf` override, no system file
  touched; `rhvoice` set as `DefaultModule`). A short spoken cue (e.g.
  "typing paused, press Super+X to finish") avoids the injection-target
  problem entirely since it never writes into the focused window. Tradeoff
  to evaluate: audible interruption may be unwanted in some environments
  (open office, calls) compared to a silent visual cue.
  - **Simplification, revised during design discussion**: skip live TTS
    synthesis for the initial implementation. Doing the general dynamic
    case (detect which TTS system/voice is installed, compose the message
    text including the actual configured hotkey, invoke the right engine)
    is real scope — implementation effort disproportionate to what's
    needed to close this ticket. Instead, ship **one pre-recorded WAV
    clip** ("Typing paused. Stop recording to finish.") committed to the
    repo and embedded in the `voxi` binary (Go `embed`), played back with
    a simple audio player call — no TTS engine dependency, no runtime
    detection, no per-invocation synthesis latency, fully self-contained.
    English-only for this ticket; the fixed wording sidesteps needing to
    interpolate the user's actual configured shortcut into the phrase.
    Multi-language audio-pack support (compressed, git-tracked WAV/FLAC
    per language) is out of scope here — see the follow-up ticket filed
    for that.

## 5. Non-Goals For This Ticket

- No code changes.
- No decision on which option ships — the user intends to prototype
  multiple variants directly.
