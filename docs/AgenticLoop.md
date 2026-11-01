<!-- harnez:variant=lite -->
---
title: Agentic Loop Practices (Lite)
weight: 40
---

# Agentic Loop Practices — Lite

Tagline-only variant. Same rules as the full doc, no prose/case-studies. See `AgenticLoop.md` (`harnez docs variant agentic-loop full`) for rationale and examples.

## 1. Core Invariants
For TUI layout work, use a checked-in `.ansi` mockup under `docs/data/` as the visual spec; have the host draft it and dispatch implementation against it. See `TUIDesign.md` for details.

1. **Parallel Read, Sequential Write** — many may read/grep at once; one writer per workspace at a time. **Sequential dispatch is the default for every task type, not just file-overlapping code edits.** Includes read-then-write races on shared sequential resources (e.g. ticket numbers).
2. **Canary & Test-Driven Verification** — verify with real test runs before declaring done; never assume an edit works. External-contract tests (scp/ssh operands, sandbox behavior, sample ages, real CLI or hook JSON) take expected values from the real tool, a recorded sample, or the real clock, never from the code under test; add a canary or container check where unit tests cannot run the real tool.
3. **Zero Zombie Guarantee & Background Task Hygiene** — track and terminate every background process, timer, and subagent before ending a session; always set `HTO=0` for background tasks/jobs; rely on reactive completion notifications instead of polling or `sleep` loops.
4. **Responsive Host Orchestrator** — stay available; interactive hosts run agent start/wait in the host background runner, never synchronously through MCP.
5. **In-Repository Single Source of Truth** — tickets/decisions/retros live in git (`issues/`, `docs/feedback/`, `docs/studies/` when present, otherwise a ticket), not just chat.
6. **Context Discipline & Range-Bounded Ingestion** — never whole-file-read `AGENTS.md`/active system rules; avoid native tool slices on >100 line files; use `harnez read -L`/`-n` via CLI (`harnez read -I` is paused until issue 543, a memory blow-up, is fixed).
7. **Media & Demo Verification Gate** — **always ask the user for explicit confirmation** of recorded output before publishing/embedding.
8. **Deployment Transparency — 3-State Grounding (when applicable)** — **Local State**, **Deployed Artifact State**, and **Active Daemon State** are independent; local build/test proves nothing about the other two (remote-deploying projects only).
9. **One-Level Delegation** — `harnez agent --role orchestrator|developer|reviewer|advisor`: only an orchestrator starts helpers (one writer at a time); developers, reviewers and advisors are leaf workers that never run `harnez agent`, native subagents or delegating skills. harnez enforces it. Live checks that need an agent session are run by the orchestrator. **Completion Evidence**: host verifies every worker's "done" report with `git show --stat <commit>` and the built output; ticket-closing workers cite the commit that changed non-issue files.
10. **Disk Headroom Before Heavy Work** — before downloads or builds over a few GB, check `df -h` on the target and keep at least 10 GiB free; if short, use a named scratch disk or ask the user first.
11. **Study & Retro Ticket Extraction** — file every actionable follow-up, link study and tickets, and list allocated ticket numbers before completing the study/retro; explain deliberate non-goals.
12. **Repository Scope & Product Primacy** — the host repository's applications and tools are the sole product of this session; never proactively offer or initiate implementation in another repository; only the user may direct cross-repo tasks; outside this repository, only read for context or file a requested/blocking issue. Use `/focus` to re-ground on the active repository's deliverables and priorities and choose the next goal. Use `/trial [today|sprint|<ticket>]` for a concise checkpoint: ten verified things we can do and five cited things we cannot (alongside `/study`, `/retro`, `/focus`).
Decide low-risk, reversible choices yourself; record the choice and reason in the ticket's `## Decision` and report its location.
Limit plans to this session; turn lasting lessons into a check, rule/prompt line, or ticket and name the safeguard and its location.

### Multi-session peer assistance
`/peer-assistant` uses available native session messaging for explicit peer requests; it does not imply an always-on watcher. Persist handoffs and findings in tickets/docs, keep one writer per workspace, and track/clean up helpers.
Tickets carry the work; messages (`harnez agent send`) only nudge: "see ticket N in repo X".

## 2. The 5-Phase Sprint Loop
Each phase's **Mechanics** and **Constraints** are summarized inline below; see `(harnez docs variant agentic-loop full)` for step-by-step detail.
- **Phase 1: Parallel Advisory Discovery (Read-Only)** — concurrent read-only advisors find exact line ranges, return plans; never write or spawn side effects. **Kickoff & Commit Policy**: agree upfront whether subagents commit directly or return diffs.
- **Phase 2: Sequential Development & Test Verification (Single-Threaded)** — one ticket at a time, TDD, workspace stays green every step. **Repro-before-fix for defect-shaped tickets**: bug/race tickets need a numeric baseline before the fix, not just passing gates. **Commit stale/failed work before discarding it**: commit/branch an abandoned attempt before reverting, except trivial one-liners. **Model selection:** escalate tier on ambiguity/architecture/security/scope growth, never trade away verification for speed.
- **Phase 3: Pre-Commit Review Gate (Independent Reviewer)** — reviewer checks Test Assertion Rigor, **Docs & Ticket Sync**, **Backward Compatibility & Invariants**, **Token Efficiency & Code Clarity**, **Live/Real-Environment Verification for hooks & env-resolution features** (unit tests alone don't prove it live), **Root Cause vs. Symptom** for defensive fixes. After a developer's green full run, rerun only touched packages (`go test <pkgs>`); run a full suite only if the developer did not run one, at release, or with the user's authorization. Exit condition: commit or explicit user authorization — not "reviewed and left uncommitted."
- **Phase 4: Process & Subagent Hygiene (Teardown & Drain)** — inspect and kill/drain lingering tasks, timers, subagents.
- **Phase 5: Agentic Flow Quality Retrospective (Learning Capture)** — record friction in a durable doc/ticket; extract actionable follow-ups as linked tickets and list numbers (or justify non-goals); sync tracker (`issues/README.md`, `harnez status`/`index`); **Single Status field per ticket** kept in sync everywhere; **Closing gate**: close shipped+verified tickets before ending; `git status` before the retro.

## 3. Two-Agent Lean Sprint (`/lean-sprint`)
For a single focused ticket: Host Orchestrator (**zero coding, no exploratory repo digging, diff-first review**) → Cheap Searcher (`--role advisor --model cheap`; bounded read-only audits) → Focused Developer (`--role dev --model lean` for standard, `std` for moderate, `smart` for complex, `fast` for easy, or `cheap` for low-cost work; TDD, unit tests, verification, milestone commits) → Host review and hygiene. Maintain a running report in `docs/studies/` when present; otherwise, keep the same progress notes in the target issue as a fallback. Use a date-prefixed `two-agent-lean-sprint` filename and short task slug, recording findings, milestones, verification, and token/time observations. Use prompt files for long or complex handoffs. The legacy single-developer workflow remains available via `/lean-sprint-v1`.

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
- **Goals Without Exit Clauses** — a `/goal` without an exit clause deadlocks the Stop hook when progress needs user input. Circuit breaker: on a loop, a hard architectural/OS blocker, or no progress, clear the goal and write a failure report (tried, why it failed, resume point).
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
- **Shell `&` Detaching** — never `&`/`nohup`/`disown` a command; use the harness's tracked background facility.
- **Machine-Background Workers** — run `HTO=0 harnez agent start/resume` as one session-background job (Claude Code: Bash `run_in_background: true`, finish within 30 minutes; AGY: `run_command` with small `WaitMsBeforeAsync`; Codex: `exec_command` + `write_stdin` until exit). Your own background tool is the only completion signal; no polling. See `harnez agent start --help`. `run_in_background`: Claude Code Bash tool parameter only; not a harnez flag.
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

<!-- harnez:model-picks:begin -->
These are the shipped picks. User models.yaml overrides are listed by `harnez agent models --picks`.

| Role | Trait | Meaning | Chain | Default |
|---|---|---|---|---|
| advisor | cheap | Low-cost work. | luna:low, haiku:low | yes |
| advisor | smart | Difficult judgment. | opus:low, opus:med, sol:low |  |
| developer | ai | AI-oriented work. | unset |  |
| developer | cheap | Low-cost work. | luna:med |  |
| developer | fast | Fast work. | haiku:med |  |
| developer | gui | Graphical interfaces. | lean, std |  |
| developer | lean | Efficient standard work. | terra:low | yes |
| developer | smart | Difficult judgment. | sol:med, opus:med |  |
| developer | std | Standard work. | terra:med, opus:low |  |
| developer | tui | Terminal interfaces. | smart |  |
| reviewer | smart | Difficult judgment. | sol:low | yes |
<!-- harnez:model-picks:end -->

<!-- harnez:stop -->
