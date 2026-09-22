---
title: Model Roles Practices
weight: 48
---

<!-- harnez:bundled -->
# Model Roles

How to choose which model plays which role in an agentic sprint, how to evaluate candidates
cheaply, and how to read what a sprint costs. Cheap models do most of the work. A stronger
host keeps them honest by reviewing diffs.

For the sprint loop itself, see `@docs/AgenticLoop.md`.

## Roles

| Role | Job | What matters |
|---|---|---|
| Host / orchestrator | Writes pre-work into the ticket, dispatches, reviews diffs, commits the ticket. Writes no code. | Judgment on diffs, catching bugs hidden by tests |
| Advisor | Read-only discovery before development: milestones, traps with `file:line`, scope cuts | Grounded code claims, spotting stale tickets |
| Developer | Implements one milestone, runs tests, commits | Following the pre-work, not faking green tests |
| Reviewer | Independent read-only review at the risk seam | A different blind spot from the developer |
| Mechanical worker | Formatting, renames, bulk edits with an explicit acceptance test | Cheap, obedient |

## Assigning models

Rules that held up in practice:

1. **Put the strongest model in the host seat, and keep it out of the code.** Its value is
   reviewing diffs and writing pre-work. Letting it code burns the expensive quota on the
   cheap part.
2. **Use two cheap advisors from different vendors, not one expensive one.** Different
   vendors have different blind spots: one finds code-level traps, the other finds
   cross-document and dependency issues. When they agree, the host can write pre-work
   straight from their answers. When they disagree, the disagreement marks the risk seam.
3. **Match developer strength to the ticket.** The lowest tier handles a clear, small ticket.
   An interface or design change needs a medium tier: low tiers are more likely to "fix" a
   failing test by changing its assertion to match the bug.
4. **Make the reviewer a different vendor from the developer.** Aim it at the seam the
   advisors named, not at the whole diff.
5. **Keep the weakest models away from judgment.** They cite wrong tickets, propose fixes the
   ticket forbids, and make weak cuts. Give them mechanical work with an explicit acceptance
   test only.

Example assignment (2026-09, Codex and Claude subscriptions):

| Role | Model |
|---|---|
| Host | claude opus |
| Advisors | codex terra:low + claude sonnet |
| Developer, clear small ticket | codex luna:low |
| Developer, interface or design change | codex luna:med |
| Reviewer | claude sonnet |
| Mechanical worker | claude haiku |

Model lineups change every few months. Rerun the evaluation below when they do, instead of
trusting an old table.

## Host review checklist

A cheap developer's report ("tests pass, open problems: none") is not evidence. On every
milestone diff, check:

- **Changed or removed assertions come first.** "Updated one stale assertion" is where a bug
  hides behind green tests.
- **Every acceptance test listed in the milestone is actually in the diff.** Cheap models
  skip one and still report no open problems.
- **Negative assertions exist.** A removal feature needs a test that the thing is *gone*, not
  only that the neighbours survived.
- **Out-of-scope behaviour changes.** A loosely worded pre-work line ("X must survive") can
  make the developer change semantics nobody asked for. Check the design doc before
  accepting.
- **The tests actually ran.** Under a one-test-run budget a developer may commit unverified
  work. Run the suite yourself when in doubt.
- **Check reviewer findings against the design.** An independent reviewer can be confidently
  wrong. Before acting on a "blocking" finding, verify it against the spec.

Write findings into the ticket as the next milestone's pre-work, not as new tickets.

## Plan before writing

Start a developer with a read-only planning turn ("read-only: plan milestone M1, no edits"),
then resume it with write access. The plan costs one short turn and regularly finds gaps in
the host's pre-work, such as more callers than the ticket names, or fields a test must skip.

## Evaluating models

Run this when choosing a lineup, or when a new model appears.

1. Write one read-only advisory prompt about a real upcoming sprint: order, per-ticket
   approach, delegation, traps with `file:line`, and a cut list, with a line limit.
2. Send the identical prompt to every candidate in parallel, each with an explicit reasoning
   effort. Don't rely on the CLI default.
3. Grade every answer against the repo, never against the other answers. Verify each trap
   claim with a grep or a range read.
4. Build a fact-check grid: one row per verifiable finding, one column per model, marked
   ✓ / ✗ / ~. Add a row for wrong citations.
5. Weigh the grid against cost (wall time, uncached tokens, tool calls). Compare token counts
   only within one vendor, because vendors count differently.

The most valuable single signal is **stale-ticket detection**. A ticket whose fix already
landed is the cheapest waste to avoid, and only models that read the code before planning
catch it.

## Reading subscription cost

- Subscription quotas report whole percentage points per window. One advisory run doesn't
  move them. Measure batches: a whole sprint on one model, starting at the beginning of a
  fresh short window.
- The long (weekly) window is the binding constraint for a multi-ticket sprint. Check it
  before every milestone and set a stop threshold, such as 99%.
- Record the quota before and after each sprint, and write the numbers into the retro.
- Per-turn quota readings live in the provider's session logs. Deleting an agent session can
  delete them, so read them first.
- When one vendor nears its stop threshold, move the remaining milestones to another vendor's
  developer. This works because the ticket carries all the context.
