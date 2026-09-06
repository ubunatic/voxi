# FluidVoice Prior-Art Review, Chunk Diagnostics, and Three Rounds of Calibration

**Date:** 2026-09-06 / 2026-09-07
**Feature issues:** [062](../../issues/062-fluidvoice-automatic-vocabulary-training-from-user-corrections-research.md)-[069](../../issues/069-async-correction-detection-pipeline-for-voxi-feedback-auto-suggest-vocabulary-from-stored-history.md) (research), [070](../../issues/070-show-recorded-volume-rms-column-in-voxi-chunks-list.md)-[072](../../issues/072-2x-braille-resolution-for-the-level-sparkline-in-voxi-chunks-list-packed-dual-column-glyphs.md) (chunk diagnostics)
**Architecture reference:** [ChunkDiagnostics.md](../ChunkDiagnostics.md)

## Context

Asked to evaluate `altic-dev/FluidVoice` (a popular macOS dictation app) as prior art
for Voxi, then — independently, in the same session — asked to add a volume/level
diagnostic to `voxi chunks list`, then to colorize it, then to double its resolution.
The two halves of the session turned out to be a useful contrast: one was pure research
with no code risk, the other was a small feature that broke three times in ways no unit
test caught.

## Part 1: FluidVoice research (issues 062-065)

Four research tickets, one per angle (auto vocabulary training, spoken-punctuation
formatting, model licensing, silence/chunking), each required to answer its questions
by *reading the actual source*, not the README. This paid off directly:

- Issue 063 corrected its own premise mid-investigation: the ticket assumed FluidVoice's
  "dictation-literal" file handled number/date formatting: it turned out to handle
  slash-commands and @mentions instead. The research ticket says so explicitly rather
  than quietly working around the wrong assumption.
- Issue 064 turned up a real dated fact a web search caught but memory wouldn't have:
  Cohere Transcribe was open-sourced (Apache-2.0) in ~March 2026 and reportedly beats
  Whisper large-v3 — this became issue 066, a canary ticket, rather than being lost as a
  passing observation.
- Issue 067 (dispatched separately, mid-conversation) was asked to "decompile" a Swift
  package assumed closed-source; before doing anything, it checked GitHub and found
  `FluidInference/FluidAudio` is actually public and Apache-2.0, changing the entire
  framing from a legally-loaded reverse-engineering task to a plain source read. Worth
  noting: the original request's premise was wrong, and the right move was to verify
  before acting, not after.

Net effect: four research tickets closed with real verdicts (adopt / adopt-reduced /
reject), and four follow-on tickets (066-069) captured the actionable ideas without
overcommitting to any of them. None of this touched code, so the only risk was wasted
research effort — mitigated by grounding every claim in an actual file read or a live
web search, never in training-data recall of what FluidVoice "probably" does.

## Part 2: Chunk diagnostics (issues 070-072) — a real-time case study of undertested calibration

This half is the more interesting one for future agentic work in this repo.

**Round 1** (issue 070): added `MeanRMS` display and a Braille sparkline. Shipped with
passing tests. First real recording the user checked rendered as **completely blank** —
indistinguishable from silence. Root cause: a linear RMS-to-glyph scale needed RMS
~1000 to show *any* level, but ordinary accepted speech sits at RMS 100-2000 (the
acoustic gate's own thresholds are ~120-150) — the exact range the chosen ceiling
crushed to zero. The unit tests used RMS 20 and 3000 as edge cases and never exercised
the middle of the range where real chunks actually live.

**Round 2**: fixed by switching to a log scale — better, but exposed a second,
independent bug: blank Braille was being used for two different meanings ("measured and
silent" vs "no data at all for this bucket"), making a genuinely quiet chunk
indistinguishable from a data gap. Fixed by reserving a literal ASCII space exclusively
for the no-data case and making the minimum audible glyph (`⣀`) the floor for any real
measurement.

**Round 3**: even with the semantic fix, the *log scale's ceiling* was still wrong — set
from synthetic test tones alone (2048), it made real conversational speech (which
routinely spikes to 600-1500 per bucket) read as 3-4 out of 4 dots on every recording.
Only caught because the user looked at real, varied output and said "I think they are
lower [than 3 dots]." Fixed by pulling actual per-bucket RMS values out of real stored
`.wav` chunk files via a throwaway debug tool, and locking those exact real-world values
into a permanent regression test (`TestSparklineLevelRealWorldCalibration`) so this
specific miscalibration can't silently return.

**The pattern across all three rounds**: every fix passed `go build`/`make check` before
being shipped, every time. The tests were well-written and asserted exact values, not
just "non-empty output." They were simply testing the wrong inputs — hand-picked
synthetic values chosen for convenience (round numbers, obvious edge cases) rather than
the actual distribution of values the feature would see in production. This is the same
failure class `~/.claude/docs/AgenticLoop.md` already documents under "Unit-Test-Only
Confidence for Hook/Environment Features," just in a new domain (audio-quantization) that
document's existing examples don't cover. See [ChunkDiagnostics.md §3.3](../ChunkDiagnostics.md#33-rms-to-level-calibration-logarithmic-floor-80-ceiling-8192)
for the technical detail; the process lesson is recorded here.

**What I'd do differently**: pull a handful of real recorded chunks through any new
sensor-data-quantization code *during* initial implementation, not after a user reports
it looks wrong. Round 1's bug and round 3's bug were both the same class of miss and
both would have been caught by the same five-minute step (a throwaway debug tool reading
real `.wav` files, exactly what round 3 eventually did) — doing it once, upfront, in
round 1 would have saved two follow-up round-trips.

## Part 3: Fresh-sprint with explicit "make the judgment calls" delegation (071, 072)

Issues 071 (colorize the sparkline) and 072 (double its resolution) were both filed as
*design-proposal* tickets — they prescribed the concrete mechanism but explicitly posed
open questions (color scheme, flag shape; color/no-data interaction after packing) as
unresolved. Both were then implemented via `/fresh-sprint` with the user explicitly
authorizing the dev agent to decide the open questions itself rather than stopping to
ask.

Both landed cleanly: the dev agent picked defensible options (a git/ls/grep-style
`--color=auto|always|never` tri-state; coloring by the louder side of a packed pair),
documented *why* in the ticket's Implementation Notes, and both held up under an
independent inline diff review afterward (dot-bit arithmetic and color-decode logic were
manually verified, not just trusted from the agent's self-report). This is a pattern
worth repeating: pose the open questions explicitly in the ticket text, then explicitly
authorize the implementing agent to resolve them and require it to write down its
reasoning — it keeps a human out of the loop for genuinely low-stakes design choices
without losing the paper trail of *why* a choice was made, which matters when a later
session needs to revisit it.

## Part 4: A cross-repo papercut, handled without duplicating tracker noise

Mid-session, `harnez find issues "is:open" -n 5` failed (`-n` isn't a supported flag).
Rather than filing a fresh complaint, the right tracker (`~/projects/harnez`, harnez's
own repo — not this one) was checked first via `harnez find`, which found the exact
request already tracked as issue 217 with a scoped design. The response was to add a
recurrence note with today's concrete repro plus a broader survey of other harnez
commands with the same gap, then bump its priority given the live friction — not to
re-file a duplicate. The subagent doing this also correctly noticed unrelated
in-progress changes already sitting in that sibling repo's working tree and committed
only the ticket file, leaving them untouched.

## Efficiency notes

Nine sequential dev/research dispatches ran across this session (four research
subagents in parallel-but-sequential succession for 062-065, then one each for
066/067/069/070, then two fresh-sprints for 071/072, then one cross-repo filing agent
for the harnez ticket). Every dispatch self-verified with `go test`/`make check` before
reporting back; every code-producing dispatch's diff was independently spot-checked
inline afterward rather than trusted at face value — this caught nothing wrong in 071 or
072 specifically, but it's cheap insurance and it's how the dot-bit arithmetic in
[ChunkDiagnostics.md §3.1](../ChunkDiagnostics.md#31-dual-column-packing-2x-resolution-issue-072)
got confirmed correct rather than merely reported-correct.

## Minor housekeeping note (not fixed here — flagged for the user)

`docs/IssueTracking.md`'s own "Allowed Values" list for `Status` is
`Open | In Progress | Blocked | Closed | Draft`. This repo's actual practice for
research-shaped tickets has, for a while (issues 039, 047, 051, and now 062-065), used
`Research Complete` as a de facto terminal status instead. It works and is consistent
with itself, but it's a real drift from the documented allowed values that a future
`harnez status`-style audit might flag. Worth either updating `IssueTracking.md` to
formally allow `Research Complete` as a Closed-equivalent terminal state, or migrating
those tickets to bare `Closed` — a decision for the user, not made here.
