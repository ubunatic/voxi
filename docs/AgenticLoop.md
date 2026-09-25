<!-- harnez:variant=lite -->
---
title: Agentic Loop Practices (Lite)
weight: 40
---

# Agentic Loop Practices — Lite

Tagline-only variant. Same rules as the full doc, no prose/case-studies. See
`AgenticLoop.md` (`harnez docs variant <name> full`) for rationale and examples.

## 1. Core Invariants
1. **Parallel Read, Sequential Write** — many may read/grep at once; one writer per workspace at a time. **Sequential dispatch is the default for every task type, not just file-overlapping code edits.** Includes read-then-write races on shared sequential resources (e.g. ticket numbers).
2. **Canary & Test-Driven Verification** — verify with real test runs before declaring done; never assume an edit works.
3. **Zero Zombie Guarantee** — track and terminate every background process, timer, and subagent before ending a session.
4. **Responsive Host Orchestrator** — stay available to the user; delegating ≠ blocking on the child unless asked or truly required.
5. **In-Repository Single Source of Truth** — tickets/decisions/retros live in git (`issues/`, `docs/feedback/`, `docs/studies/`), not just chat.
6. **Context Discipline & Range-Bounded Ingestion** — never whole-file-read `AGENTS.md`/active system rules; avoid native tool slices on >100 line files; use `harnez read -L`/`-n` via CLI (`harnez read -I` is paused until issue 543, a memory blow-up, is fixed).
7. **Media & Demo Verification Gate** — **always ask the user for explicit confirmation** of recorded output before publishing/embedding.
8. **Deployment Transparency — 3-State Grounding (when applicable)** — **Local State**, **Deployed Artifact State**, and **Active Daemon State** are independent; local build/test proves nothing about the other two (remote-deploying projects only).
9. **One-Level Delegation** — `harnez agent --role orchestrator|developer|reviewer|advisor`: only an orchestrator starts helpers (one writer at a time); developers, reviewers and advisors are leaf workers that never run `harnez agent`, native subagents or delegating skills. harnez enforces it. Live checks that need an agent session are run by the orchestrator.

## 2. The 5-Phase Sprint Loop
Each phase's **Mechanics** and **Constraints** are summarized inline below; see the full doc for step-by-step detail.
- **### Phase 1: Parallel Advisory Discovery (Read-Only)** — concurrent read-only advisors find exact line ranges, return plans; never write or spawn side effects. **Kickoff & Commit Policy**: agree upfront whether subagents commit directly or return diffs.
- **### Phase 2: Sequential Development & Test Verification (Single-Threaded)** — one ticket at a time, TDD, workspace stays green every step. **Repro-before-fix for defect-shaped tickets**: bug/race tickets need a numeric baseline before the fix, not just passing gates. **Commit stale/failed work before discarding it**: commit/branch an abandoned attempt before reverting, except trivial one-liners. **Model selection:** escalate tier on ambiguity/architecture/security/scope growth, never trade away verification for speed.
- **### Phase 3: Pre-Commit Review Gate (Independent Reviewer)** — reviewer checks Test Assertion Rigor, **Docs & Ticket Sync**, **Backward Compatibility & Invariants**, **Token Efficiency & Code Clarity**, **Live/Real-Environment Verification for hooks & env-resolution features** (unit tests alone don't prove it live), **Root Cause vs. Symptom** for defensive fixes. Exit condition: commit or explicit user authorization — not "reviewed and left uncommitted."
- **### Phase 4: Process & Subagent Hygiene (Teardown & Drain)** — inspect and kill/drain lingering tasks, timers, subagents.
- **### Phase 5: Agentic Flow Quality Retrospective (Learning Capture)** — record friction in a durable doc/ticket; sync tracker (`issues/README.md`, `harnez status`/`index`); **Single Status field per ticket** kept in sync everywhere; **Closing gate**: close shipped+verified tickets before ending; `git status` before the retro.

## 3. Lean Fresh-Handoff Pattern (`/lean-sprint`)
For a single focused ticket: **Clean Goal Handoff** (one objective; **Trust the Base Framework** — don't restate base rules already in the system prompt; stay responsive) → **Autonomous Execution & Self-Verification** → **Confidence-Gated Inline Review** (skip a formal reviewer when tests pass and confidence is high; escalate on cross-subsystem risk) → **Fast Hygiene** / **Fast Hygiene & Status Sync** (kill children immediately). ### Workflow Selection Matrix: **Overhead** and **Host Responsiveness** both favor the full 5-phase loop for multi-ticket/major/broad work; lean fresh-handoff for one ticket/bug fix.

## 4. Calibrated Friction Reporting
**Substantive Sessions Only** (real hurdles, not routine) · **Zero Repetitive Noise** (no boilerplate on known quirks) · **Actionable Root Causes** (blocker, failure mode, workaround tried, recommended fix).

**Name items in reports**: give each milestone/ticket a short name at least once ("M3 (single-write removal)", "498 (Claude resume)"), not a bare "M3" or "498".

## 5. Role Taxonomy
| Role | Tools | Job | Lifecycle |
|---|---|---|---|
| Host Orchestrator | Full | Coordinates, manages subagents, talks to user | Persistent |
| Ephemeral Advisor | Read-only | Audits tickets, researches feasibility | Killed after Phase 1 |
| Dev Worker | Write + tests | Implements, writes tests | Single-threaded per workspace |
| Independent Reviewer | Read-only + diff | Audits diff vs. acceptance criteria | Spawned in Phase 3 |

## 6. Anti-Patterns to Avoid
- **Parallel Writing** — multiple write-permitted subagents on one workspace at once.
- **Blocking Handoff Waits** — treating a handoff as license to block the main chat.
- **Silent Verification** — assuming a fix works without running tests/canaries.
- **Unit-Test-Only Confidence for Hook/Environment Features** — green `go test` ≠ proof a hook/env-resolution feature works live; require one real end-to-end check after a genuine restart/re-apply.
- **Deployment State Conflation** — calling something "deployed"/"scheduled" from local build/test success alone, without probing the live host.
- **Blind Revert of Failed Work** — `checkout --`/`reset --hard`/`stash drop` on a failed attempt without committing it first.
- **Reviewed-But-Uncommitted Carryover** — finishing review and moving on with verified work still uncommitted.
- **Narrow String-Substitution Edits Over Structured Patches** — prefer `apply_patch`/whole-block replacement; measured **11.1%** failure rate for `Edit` vs. **4.2%** for `apply_patch` (2.6×) in the same repo.
- **Baking Real Credentials In For A Fast Dev Loop** — no real hostnames/MACs/credentials "temporarily"; use RFC-1918/example values + a secret scanner from commit one.
- **Orphaned Background Tasks** — leftover `tail -f`/watch loops/timers after work is done.
- **Lost Context / Ephemeral-Only Retrospectives** — friction/bugs discussed in chat but never written to a durable doc/ticket.
- **Rubber-Stamp Reviews** — a review that doesn't actually inspect assertions or diffs.
- **Unbounded Doc Ingestion** — whole-file-reading `AGENTS.md`/bundled docs already in the active prompt.
- **Unverified Media Publishing** — publishing recordings/screenshots without explicit user confirmation.
- **Prompt Micromanagement** — restating base rules/tool docs the harness already provides in a subagent prompt.
- **Friction Noise Over-Reporting** — repetitive, low-signal friction reports on routine tasks.
- **Blocking `sleep` Waits** — long/chained `sleep` to wait out CI/deploy/queue; use harness notifications or a scheduled-wakeup loop instead.
- **Chat-Visible Empty Polling** — re-checking a long job on a fixed short interval with no new output; prefer a real completion signal or a detached job + log file.
- **Buffered Long-Running Output** — piping a long command through `tail`/`grep`/`sort`/`wc`/`head`, which shows nothing until it exits; run plain or `tee` to a log.
- **`cd`-scoped commands** — bare `cd` leaking into later unrelated calls in a shared shell; use `-C`/`--prefix`/`--manifest-path` or a subshell `(cd dir && cmd)`.
- **Chatty Watch Wrappers** — wrapping a poll-and-redraw CLI (`gh run watch`, `docker logs -f`) in a routine agent-called target without quieting it; poll the tool's own status query on a matched interval and print one summary line on completion instead.

<!-- harnez:stop -->
