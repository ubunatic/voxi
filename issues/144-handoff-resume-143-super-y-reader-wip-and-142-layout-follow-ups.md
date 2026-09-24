# 144 — Handoff: resume 143 (Super+Y reader WIP) and 142 layout follow-ups

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Minor
**Category**: Handoff
**Related**: 143 (§5 pause notes), 142 (§5 live-check follow-ups), 141 (TTS MVP, closed), 139 (install owns units, closed), harnez 543 (`harnez read -I` memory leak)

---

## 1. Context

A Claude Code session (2026-09-24, run under `harnez agent chat`, Claude session
`a46e2ba4-b23a-44eb-aa19-f25b2d50d892`) delivered the following:
- 139: `voxi install` owns all units, including R2T2.
- 141: monitor-gated TTS with Festival, `voxi say`, MPRIS media keys, Super-x
  stopping TTS, and opt-out install.
- 142: the unified IN/OUT monitor feed.

It stopped with two things unfinished:

- **143 is paused mid-implementation.** Uncommitted, untested work-in-progress
  from developer `sel-dev` is in the working tree: `cmd/voxi/main.go`,
  `internal/shortcut/*`, `internal/tts/{command,manager,socket}*`. The resumable
  harnez agent session is `sel-dev`
  (`01a0d3cc-11c7-7b11-9405-1a36a0729809`, codex:terra:low). The approved plan
  and the host's changes to it are in 143 §2–§5:
  - `voxi say --interrupt --from primary|clipboard` calls `wl-paste` itself,
    with no shell pipe in the binding;
  - a `notify-send` notification on errors;
  - an atomic `replace` socket request;
  - `voxi shortcut setup` adds Super+Y and Super+Shift+Y.
- **142 is open, with follow-ups from the user's live check** (142 §5): the `[t]`
  transcript feed goes back to 4 visible lines, and the TTS box moves to the
  hardware box's old position.

## 2. /goal

143 and 142 are both closed and verified live by the user:
- Super+Y reads the primary selection and Super+Shift+Y reads the clipboard,
  both interrupting current speech.
- The monitor layout matches 142 §5.
- The working tree is clean, and this ticket is closed.

## 3. How to resume

1. Check `git status`, `git diff` and `git log` first. The WIP may be stale, or
   someone else may have changed it.
2. Run it as a lean sprint (`/lean-sprint 143 142`) with one writer at a time:
   143 first, then 142 §5. You can either resume `sel-dev`
   (`harnez agent resume --name sel-dev "..."`), or dispatch a fresh developer
   that picks up the WIP diff.
3. **Memory hazard:** do not pass several files to `harnez read -I` until harnez
   543 is fixed. It reached 16 GB and was OOM-killed twice this session. Use
   `harnez read --text -L` one file at a time, and pass the same rule to every
   developer.
4. Run the 143 canary (can `wl-paste` read the selection and clipboard when
   launched from a GNOME shortcut?) before the user's live check.

## 4. Environment notes

- Voxi-side changes the session made to `~/.config/voxi/config.yaml`:
  `llama_server_path` points at lmcoder's
  `~/.local/share/lmcoder/llama.cpp/b10590/llama-server`, which is needed by
  `voxi-r2t2.service`. Update it if lmcoder moves to a newer build.
- TTS canaries live in `scripts/canary_tts/`. The engine WAV samples are in
  `~/.cache/voxi/tts-canary/`.
