# 045: Editable Pre-Filled Transcript + Keyterm Prompt for Sample Recorder

**Status**: Proposed
**Priority**: P2 (Medium)
**Severity**: Moderate
**Category**: Enhancement
**Related**: [042 private dev sample recorder](042-private-dev-sample-recorder.md), [044 keyterm-dense sample recording follow-up](044-keyterm-dense-sample-recording-followup.md)

---

## 1. Problem & Motivation

Using `voxi feedback sample record <name>` in practice (issue 044) surfaced
two friction points in the correction step:

1. `promptText` in `internal/devsample/flow.go` asks for the corrected
   transcript as a blank line — the user has to retype the whole sentence
   from scratch, rather than starting from what `small.en` actually produced
   and editing just the wrong words with the cursor keys. For most
   utterances the raw ASR output is close but not exact, so a full retype is
   wasted effort and more error-prone than a small in-place correction.
2. The manifest's `keyterms` column (`corpus.tsv`'s 4th field) is never
   asked for or populated. Every sample recorded so far has an empty
   keyterms field, silently zeroing `keyterms_found`/`keyterms_total` for
   every fixture until someone edits `corpus.tsv` by hand afterward (as
   happened in issue 044 — see its Section 6 and issue 032 Section 7.2).
   This defeats the actual purpose of a keyterm-dense corpus unless the user
   remembers to hand-patch the TSV every time.

## 2. Desired Design

**Editable pre-filled transcript**: run the captured audio through Voxi's
existing `small.en` transcription path (unprompted, same as eager's default)
immediately after capture, and present that raw transcript as the *initial
line content* of a real readline-style input (cursor left/right, backspace,
word-edit) rather than an empty prompt. The user corrects only what's wrong
and hits Enter; an unedited Enter accepts the raw ASR output verbatim.

**Keyterm prompt**: after the corrected-text step, add a second prompt
asking for keyterms (comma or pipe separated, matching `corpus.tsv`'s `|`
convention), pre-populated by intersecting the corrected text against the
existing static/user vocabulary (`spec/models.yaml`'s `speech_context.terms`
plus `~/.config/voxi/vocabulary.txt`) so recognized terms are suggested
rather than typed from memory. Empty input keeps the keyterms field empty
(no behavior change for callers who skip it), matching today's default.

## 3. Implementation Plan

1. Canary-first: confirm what's available in this environment/Go stdlib for
   a readline-style editable-line prompt (candidates: `golang.org/x/term`
   raw-mode + a minimal line editor, or a small existing readline-style
   dependency if one is already an indirect dependency — check `go.mod`
   before adding anything new, per the project's "avoid deps" convention in
   `docs/Go.md`). If no clean fit exists, a documented fallback (print the
   raw transcript above the prompt, let the user copy/edit) is acceptable
   but should say why the true line-editing goal wasn't met.
2. Run the just-captured PCM through the existing eager `small.en`
   transcription call (no `--speech-context` prompt — this recorder samples
   real-world ASR error, prompting would bias what it's meant to measure)
   to get the raw transcript for step 1's pre-fill.
3. Extend the keyterm-suggestion prompt using the existing speech-context
   term sources; write the result into `corpus.tsv`'s 4th field.
4. Unit tests for: pre-fill defaulting to the raw transcript on unedited
   Enter, keyterm suggestion intersection logic, and empty-keyterms fallback
   behavior unchanged from today.
5. Update issue 042's `## 6. Implemented Behavior` and this ticket with the
   final flow.

## 4. Acceptance Criteria

- `voxi feedback sample record <name>` shows the raw ASR transcript as
  editable, cursor-navigable starting content rather than a blank line.
- A keyterm prompt follows the transcript step, pre-suggesting recognized
  vocabulary terms found in the corrected text.
- `corpus.tsv`'s keyterms column is populated by default for newly recorded
  samples without a manual follow-up edit.
- `go test ./...`, `make check`, `make install` pass.

## 5. Non-goals

- No change to `list`/`play`/`remove` behavior.
- No retroactive fix of already-recorded samples' empty keyterms columns
  (issue 044 already hand-patched the ones needed so far).
- No full `$EDITOR` invocation — an inline editable line is the target UX,
  not spawning an external editor (that remains a documented future
  nice-to-have per issue 042 Section 2).

## 6. Implemented Behavior

```text
$ voxi feedback sample record my-toolchain
Recording with pw-record... speak now, then press Enter to stop.
Captured 3.2s of audio.
Raw ASR guess: "My toolchain in the agentic environment uses a harness tool."
Enter the corrected transcript (what you actually said): <Enter>
Suggested keyterms: Voxi|dotool
Press Enter to accept, or type replacement keyterms (comma or | separated): <Enter>
Saved sample "my-toolchain" (~/.config/voxi/samples/my-toolchain.wav)
```

**Readline-style editing — real implementation (follow-up, approved
dependency)**: the fallback described above was superseded once the user
explicitly approved `golang.org/x/term` as a dependency for this purpose.
`internal/devsample/lineedit.go` adds a raw-mode line editor: `promptText`
and `promptKeyterms` both start the prompt line pre-filled with the raw
transcript / suggested keyterms, cursor at the end — the same feel as a
shell's up-arrow history recall — and let the user edit in place rather than
only accept-as-is or retype-in-full. `docs/Go.md`'s dependency allowlist now
lists `x/term` alongside `cobra` and `yaml.v3`.

Key bindings implemented: Left/Right (move one character), Ctrl-Left/
Ctrl-Right (move one word, xterm's `ESC[1;5D`/`ESC[1;5C`), Home/End (both the
letter form `ESC[H`/`ESC[F` and the tilde form `ESC[1~`/`ESC[4~`, plus the
SS3 form `ESC O H`/`ESC O F` some terminals send), Backspace and Delete
(`ESC[3~`), Enter to submit (including unmodified — an untouched Enter
returns the pre-fill verbatim, same contract as before), and Ctrl-C to abort
the whole `Record` call (returns a sentinel `errAborted`, propagated by both
prompts; safe because neither prompt runs after any disk write in `Record`).
An unrecognized escape sequence is parsed as far as it can be and then
treated as a no-op rather than corrupting the edit buffer or desyncing the
byte stream.

This only activates against a real terminal: `useLineEditor` requires both
an output writer and `stdinFile` (`d.Stdin` asserted to `*os.File`) to pass
`term.IsTerminal`. Piped stdin, and every existing test's
`strings.Reader`/`bytes.Reader` stdin, are never an `*os.File`, so they take
the unchanged pre-045 fallback path (print the suggestion, read one plain
line) — no existing test needed to change its expectations, only its call
signature (an added `stdinFile *os.File` parameter, `nil` in all of them).

Terminal-compatibility caveats found while implementing this: (1) a bare
Escape keypress has no follow-up bytes, so the escape-sequence reader arms a
50ms read deadline (`tty.SetReadDeadline`) after seeing `ESC` and treats a
timeout as "ignore this keypress" rather than hanging the prompt forever;
this depends on the terminal fd supporting deadlines, which Linux ttys
generally do, but if `SetReadDeadline` itself errors the code silently skips
the timeout (accepting the lone-Escape hang risk) rather than breaking normal
use. (2) The escape sequences handled are what xterm-family terminals
(xterm, GNOME Terminal/VTE, kitty, alacritty, foot, ...) send in their
default (non-application) cursor-key mode; a terminal in application cursor
mode or an unusual/legacy emulator may send a sequence not listed above.

**Verified vs. not verified**: `internal/devsample/lineedit_test.go` unit-
tests the line-editor logic directly — a pure `lineEditor.apply` fed
sequences of already-parsed key events (char insert at cursor, backspace,
delete, left/right, word-left/word-right via Ctrl-arrows, Home/End,
Enter-submits-unmodified-prefill, Ctrl-C cancels) plus the byte-level ANSI
parser (`readKeyEvent`/`readCSISequence`) fed raw escape-sequence bytes
through a `bufio.Reader` — all without a real TTY. What is **not** verified
by this change: actual raw-mode capture and escape-sequence parsing against
a genuine terminal emulator, since this sandbox has no interactive TTY to
exercise `term.MakeRaw` end-to-end. That needs the user's confirmation on a
real terminal.

**Raw-transcript pass**: `internal/devsample/transcribe.go` adds
`transcribeRawTranscript`, called from `Record` right after capture. It does
**not** import `internal/eager` to reuse its transcription call: `eager`
already imports `internal/feedback`, and `internal/feedback` imports
`internal/devsample` (for the `sample` subcommands) — `devsample -> eager`
would close an import cycle. Instead it duplicates the small subset of
`eager`'s transcription invocation actually needed: resolve `voxtype` via
`d.LookPath`, resolve `spec.LoadModels().DefaultModel` (`small.en`), write
the captured PCM to a temp WAV, run `voxtype --model <model> --threads 6 -q
transcribe <wav>` with **no** `--initial-prompt` (unprompted, matching
issue's requirement that this recorder samples real-world ASR error rather
than a speech-context-biased result), and clean the output with the existing
`internal/asr.CleanWhisperTranscript`. Any failure (voxtype missing, no
`LookPath` resolver wired into `deps.Dependencies`, model spec load error,
...) is non-fatal: `Record` prints a `Note: raw ASR transcript unavailable
(...)` line and falls back to today's blank-prompt behavior exactly.

**Keyterm suggestion**: after the transcript is confirmed, `Record` loads
`spec.LoadModels().SpeechContext.Terms` (the static shipped vocabulary) plus
`~/.config/voxi/vocabulary.txt` via the existing
`speechcontext.VocabularyPath` / `speechcontext.ParseVocabulary` helpers —
the same two sources eager's opt-in `--speech-context` prompt draws from.
`suggestKeyterms` (in `transcribe.go`) intersects those terms
case-insensitively against the corrected transcript, in source order,
deduplicated. The result is shown as `Suggested keyterms: A|B|C` (the same
`|`-joined shape as `corpus.tsv`'s 4th column); a blank Enter accepts it
verbatim (empty when nothing matched — identical to every sample recorded
before this ticket), and typing a comma- or `|`-separated line replaces it
(`normalizeKeyterms` splits on either separator, trims, and dedups
case-insensitively).

**Manifest**: `devsample.Sample` gained a `Keyterms string` field;
`ParseManifest`/`FormatManifest` in `internal/devsample/sample.go` now
read/write `corpus.tsv`'s 4th column instead of always leaving it blank.
`scripts/speech_context_bench` needed no change — it already reads that
column.

**Tests**: `internal/devsample/transcribe_test.go` covers `suggestKeyterms`
(intersection, case-insensitive dedup, no-match-is-empty) and
`normalizeKeyterms` (comma/`|` splitting, dedup, blank input).
`internal/devsample/flow_test.go` adds: `promptText` unedited-Enter-accepts-
raw-default and typed-line-overrides-default, `promptText` still rejecting
blank input when there is no raw default (pre-045 behavior preserved),
`promptKeyterms` blank-accepts-suggestion / typed-replaces / empty-
suggestion-stays-empty, and two `Record`-level integration tests: one wiring
a stubbed `transcribeFn` through to a populated manifest keyterms field, and
one confirming the graceful fallback (with a printed note) when
`transcribeFn` errors. All prior `flow_test.go`/`sample_test.go` tests pass
unmodified — none of them stub `transcribeFn`, so they exercise the
`d.LookPath == nil` fallback path for real.

Verified with `go build ./...`, `go vet ./...`, `go test ./...`,
`make check`, and `make install` on 2026-09-02. This change touches only
`internal/devsample` (a manually-invoked dev subcommand, not the always-
running daemon) and `internal/feedback`'s manifest-adjacent code paths; it
does not touch `internal/eager`, `internal/modifiers`, or the agent's
always-running paths, so `make install` is sufficient — `make
restart-service` is not needed.
