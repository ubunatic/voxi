---
title: Agentic Loop Practices
weight: 40
---

# Agentic Loop Practices — Multi-Agent Sprint Workflow

This document establishes the canonical practice for orchestrating multi-agent development loops. It defines the lifecycle, synchronization invariants, role archetypes, and quality gates required to conduct rapid, collision-free agentic sprints.

Capability names vary by agent harness. In the examples below, repository search
means tools such as `grep_search`, bounded reads mean line-range reads,
background-task inspection means commands such as `manage_task list`, and
subagent lifecycle control means commands such as `manage_subagents kill`.

---

## 1. Core Philosophy & Invariants

Agentic software engineering scales effectively when concurrency is structured and single-threaded write locks are strictly preserved.

1. **Parallel Read, Sequential Write**:
   - Multiple subagents may concurrently explore, read, grep, and analyze the codebase.
   - Only **one** agent may modify files, write code, or execute build mutations in a shared workspace at any given time.
   - Concurrent writes produce race conditions, broken intermediate states, git conflicts, and corrupt dependencies.
   - **Sequential dispatch is the default for every task type, not just file-overlapping code edits.** Two subagents each doing "read-only" investigation or ticket-filing work can still race on a shared, sequentially-allocated resource they both read and then write independently — e.g. two agents each scanning `issues/README.md` for "the next free issue number" and landing on the same one, because each read a snapshot that was already stale by the time it wrote (see `docs/studies/2026-08-30-usage-panel-integration-and-the-agy-collector-cliff.md`). File-level non-overlap is not sufficient evidence that parallel dispatch is safe. Dispatch one subagent at a time unless the user explicitly requests parallel execution for a specific task — and even then, verify the run actually was concurrent and check for this class of race afterward.

2. **Canary & Test-Driven Verification**:
   - Every task must be verified with real test executions (`go test ./...`, `make smoke`, canary probes) before declaring completion.
   - Never assume an edit succeeds without observing passing assertions.

3. **Zero Zombie Guarantee**:
   - Every spawned background process, schedule timer, or subagent must be tracked, accounted for, and explicitly terminated before concluding a session.
   - Orphaned processes, lingering watch commands, and abandoned poll loops degrade system resources and corrupt future test runs.

4. **Responsive Host Orchestrator**:
   - The host remains the user's always-available coordination surface while child agents work.
   - A user request to "hand this to a subagent" means delegate and keep the main chat responsive; it does not imply permission to block on `wait_agent`, `manage_subagents wait`, or equivalent.
   - Wait for a child only when the user explicitly asks to wait, or when the next user-visible integration step truly cannot proceed without that result.
   - After dispatch, report the handoff and continue with non-overlapping local work or return control to the user instead of occupying the host turn with an idle wait.

5. **In-Repository Single Source of Truth**:
   - Tickets (`issues/*.md`), architectural decisions, retrospectives (`docs/feedback/`, `docs/studies/`), and specifications (`spec/`) live in the git tree.
   - Session context, learnings, and friction logs must be committed to the repository rather than abandoned in ephemeral agent chat contexts.

6. **Context Discipline & Range-Bounded Ingestion**:
   - Never execute whole-file reads on files already present in the active system prompt (`AGENTS.md`, `CLAUDE.md`, system rules).
   - Prefer index consultation, `grep_search`, and range-bounded reads (`StartLine`/`EndLine`) over bulk document ingestion. In-file warning banners are ineffective once returned into message history.

7. **Media & Demo Verification Gate**:
   - When creating, updating, or adding media assets (e.g. reels, WebM demos, terminal recordings, screenshots) intended for documentation or websites, **always ask the user for explicit confirmation** that the recorded visual output matches their exact expectations before publishing or embedding it.
   - Never automatically publish or embed unverified recordings (guarding against invisible typing, missing UI frames, or unexpected rendering artifacts).

8. **Deployment Transparency — 3-State Grounding**:
   - Remote deployment status has three independent states that must never be conflated:
     **Local State** (checkout, configs, unit tests), **Deployed Artifact State** (remote
     filesystem binaries, permissions, config overlays), and **Active Daemon State** (remote
     process table, systemd units, active crontab entries).
   - Agents must never declare remote deployment status without probing the live host over SSH
     (`crontab -l`, `ls -la`, `file`, `head`). A clean local build or passing local test is
     evidence about Local State only. See
     [docs/practices/DeploymentTransparency.md](DeploymentTransparency.md) for the full
     invariant, minimum probes, and anti-patterns.

---

## 2. The 5-Phase Agentic Sprint Loop

```
       Phase 1: Parallel Advisory Discovery
       [Advisor A]   [Advisor B]   [Advisor C]
            \             |             /
             v            v            v
       Phase 2: Sequential Development & TDD
          (Task 1 -> Task 2 -> Task 3)
                          |
                          v
       Phase 3: Pre-Commit Review Gate
             [Independent Reviewer]
                          |
                          v
       Phase 4: Process & Subagent Hygiene
         (inspect, drain, and terminate tasks)
                          |
                          v
       Phase 5: Flow Quality Retrospective
       (docs/feedback/, docs/studies/, issues)
```

### Phase 1: Parallel Advisory Discovery (Read-Only)
- **Goal**: Rapidly audit requirements, discover existing implementations, identify affected files, and evaluate technical feasibility without code collisions.
- **Mechanics**:
  - The Host Orchestrator spawns concurrent read-only advisor subagents (e.g. one per ticket or feature area).
  - Advisors perform focused repository searches and bounded reads, evaluate whether requirements are already partially or fully met, and identify exact line ranges for changes.
  - Advisors return concise findings and structured implementation plans to the Host.
- **Kickoff & Commit Policy**:
  - Establish commit authority upfront. If operating under an ask-first harness, ask the user during kickoff for permission to commit local verified checkpoints proactively so the user can walk away without returning to uncommitted progress.
  - Clarify whether subagents will commit directly or return diffs for the orchestrator to commit on their behalf.
- **Constraints**: Advisors must never write files, run mutating commands, or spawn untracked side effects.

### Phase 2: Sequential Development & Test Verification (Single-Threaded)
- **Goal**: Implement planned changes cleanly, incrementally, and with continuous test verification.
- **Mechanics**:
  - The Host Orchestrator (or a dedicated dev subagent executing sequentially) addresses tasks one ticket at a time.
  - Test-Driven Verification: Write or adapt unit tests alongside or prior to code changes.
  - Validate intermediate milestones with fast test suites (`go test ./...`).
  - Keep the workspace in a compilable, passing state at every step.
  - **Commit stale/failed work before discarding it**: When an implementation attempt is abandoned — because it regressed a gate, because a cleaner strategy was found, or because it was simply wrong — do not `git checkout --`/`git reset --hard`/`git stash drop` it away as the first move. Commit it first, on the current branch or a throwaway one (e.g. `git commit -m "wip: attempt N, reverted — see issue NNN" --no-verify` only if hooks block a WIP commit, otherwise a normal commit), *then* revert the working tree with `git revert` or by checking out the prior commit. This keeps the failed attempt in `git log`/`git reflog` as a real, diffable artifact instead of only as prose in a ticket. A short-lived local branch (`git branch attempt-2-endpoint-cone`) pointing at the WIP commit is even better when more than one attempt is worth preserving side-by-side. Only skip this for genuinely trivial, single-line experiments where the narrative description *is* the diff (e.g. "tried threshold=50, tried threshold=25, both failed" needs no commit) — the bar is "would a future reader want to `git diff` this," not "is this attempt tidy."

**Model selection:** Use a fast capable model for clear, bounded, testable subagent tasks. Use a more capable model for ambiguity, architecture, security, deep debugging, broad changes, or final review. Escalate on uncertainty, failed checks, or scope growth; never trade away verification for speed.

### Phase 3: Pre-Commit Review Gate (Independent Reviewer)
- **Goal**: Enforce quality standards and catch regressions before changes are committed.
- **Mechanics**:
  - The Host spawns an independent Reviewer subagent (or executes a dedicated review pass).
  - The Reviewer audits the working tree diff (`git diff`) against requirements.
  - Review Checklist:
    - **Test Assertion Rigor**: Are tests asserting specific outcomes or merely executing code without assertions?
    - **Docs & Ticket Sync**: Are all issue status tags, README indices, and docs updated in sync with code?
    - **Backward Compatibility & Invariants**: Does the change uphold project invariants and CLI design boundaries?
    - **Token Efficiency & Code Clarity**: Is the code concise, readable, and free of redundant abstractions?
    - **Live/Real-Environment Verification for hooks & env-resolution features**: for any change that installs a live
      agent hook, writes global config (`apply`), or resolves state from the ambient environment (branch name,
      session env vars, cwd), passing `go test ./...` is not sufficient evidence it works — test fixtures routinely
      supply explicit args or isolated temp dirs that mask exactly the resolution failures real usage hits (e.g. a
      branch-name heuristic that assumes per-ticket branches when the user never creates them; two independently
      unit-tested hooks that only collide once both are installed together). Require one real, live end-to-end
      check after a genuine restart/re-apply against the actual environment before the ticket is done — see the
      2026-08-31 `harnez-tool-observability` case study for the concrete failure this caught (docs/studies/).

### Phase 4: Process & Subagent Hygiene (Teardown & Drain)
- **Goal**: Prevent zombie accumulation, orphan processes, and stuck background tasks.
- **Mechanics**:
  - Inspect running background tasks with the harness's task-management capability.
  - Explicitly kill or drain completed, idle, or lingering background jobs, schedule timers, and watch subprocesses.
  - Terminate child subagents with the harness's subagent lifecycle controls after they finish.
  - Ensure the host and system state is pristine.

### Phase 5: Agentic Flow Quality Retrospective (Learning Capture)
- **Goal**: Continually refine agent workflows, document tooling friction, and persist session insights.
- **Mechanics**:
  - Record session friction, harness observations, and process recommendations in `docs/feedback/` (agentic workflow feedback) or `docs/studies/` (in-depth engineering case studies).
  - Update issue tracker status (`issues/README.md`) and run `harnez status` to ensure zero drift between issues and indices; run `harnez index` (issue 148) to regenerate `issues/README.md` and `docs/README.md`'s studies table from their source files instead of hand-editing rows.
  - **Closing gate**: for every ticket touched this session whose work is now shipped and
    verified, flip its `Status` header to `Closed` before ending the session — do not let a
    green build and a commit stand in for closing the ticket. See
    [IssueTracking.md](IssueTracking.md) §5 ("Closing Is Part Of Done") and the `smarthome`
    tracker-drift case (`docs/studies/2026-09-04-three-days-to-a-public-release.md` §4.2).
  - Prepare clean, conventional commit messages.

---

## 3. The Lean Fresh-Handoff Pattern (`/fresh-sprint`)

```
   Host Orchestrator
          |
   (1) Clean Goal Handoff (problem, tickets, verification targets)
          |
          v
     [Fresh Dev Subagent]
          |
   (2) Autonomous Dev & Self-Verification (TDD, go test, make check)
          |
          v
   (3) Confidence-Gated Inline Review
       ├── High Confidence / Passing Tests ──> Return directly to Host (Inline Diff Check)
       └── High Ambiguity / Regressions    ──> Escalate to Independent Reviewer Subagent
          |
          v
   (4) Fast Hygiene & Teardown (kill child agents, zero zombies)
```

For focused, day-to-day tickets, running the full 5-phase ceremony with separate advisor and reviewer subagents introduces unnecessary latency and token overhead. The **Lean Fresh-Handoff** pattern provides a lightweight, fast-path alternative:

1. **Clean Goal Handoff**:
   - The Orchestrator dispatches a fresh subagent with a single, clear objective: problem statement, target tickets/specs, and explicit verification criteria.
   - **Trust the Base Framework**: Avoid micromanaging standard workspace rules, tool descriptions, or language conventions already provided by the base system prompt.
   - **Stay Responsive**: After dispatch, the Orchestrator returns control to the main chat or continues only with non-overlapping local work. Do not block on the dev subagent by default.
2. **Autonomous Execution & Self-Verification**:
   - The dev subagent implements changes and validates them using repo-native verification commands (`go test ./...`, `make check`, canary probes).
3. **Confidence-Gated Inline Review**:
   - If automated tests pass cleanly and confidence is high, skip dispatching an independent reviewer subagent.
   - The primary orchestrator performs a rapid inline diff review before finalizing.
   - Escalate to a formal reviewer agent only if there is cross-subsystem blast radius, missing automated test coverage, or unexpected complexity.
4. **Fast Hygiene**:
   - Immediately terminate child subagents (`manage_subagents kill`) and clear background tasks.

### Workflow Selection Matrix

| Dimension | Formal 5-Phase Loop (`/sprint`) | Lean Fresh-Handoff (`/fresh-sprint`) |
|---|---|---|
| **Scope** | Multi-ticket sprints, major features, broad refactors | Single focused ticket, bug fix, localized feature |
| **Discovery** | Parallel read-only advisor subagents | Targeted orchestrator/dev grep & range-bounded reads |
| **Review Gate** | Independent reviewer subagent mandatory | Confidence-gated inline review (escalate on risk) |
| **Overhead** | Higher compute/tokens, maximum verification depth | Minimal compute/latency, rapid turnaround |
| **Host Responsiveness** | Host may coordinate multiple workers but remains user-responsive | Host dispatches and returns control; no default blocking wait |

---

## 4. Calibrated Friction Reporting Standard

Agentic retrospectives and tooling feedback are vital for evolving harnesses, but must remain calibrated to avoid feedback fatigue:

1. **Substantive Sessions Only**: Capture authentic tool, environment, or sandbox friction **only** after non-trivial sessions where real hurdles occurred.
2. **Zero Repetitive Noise**: Do not emit repetitive boilerplate or complain about known, trivial environment quirks on routine, fast iterations.
3. **Actionable Root Causes**: When reporting friction in `docs/feedback/` or tickets, state the concrete blocker, failure mode, attempted workaround, and a recommended harness or tooling fix.

---

## 5. Role Taxonomy & Constraints

| Role | Permitted Tools & Capabilities | Primary Responsibilities | Lifecycle |
|---|---|---|---|
| **Host Orchestrator** | Full Toolset (Subagents, Read, Write, Exec, Tasks) | Coordinates overall plan, sequences dev work, manages subagents, interacts with user | Persistent (lives throughout session) |
| **Ephemeral Advisor** | Read-only repository search and retrieval | Audits tickets, performs feasibility research, identifies code paths | Ephemeral (terminated after Phase 1) |
| **Dev Worker** | Write Tools, Compiler, Test Runner | Implements concrete changes, writes unit tests, ensures compilation | Single-threaded per workspace |
| **Independent Reviewer** | Read-Only Tools, Diff Inspection | Audits git diff against acceptance criteria, verifies test rigor | Ephemeral (spawned in Phase 3) |

---

## 6. Practical Recipes & Anti-Patterns

### Anti-Patterns to Avoid
- ❌ **Parallel Writing**: Spawning multiple subagents with write permissions on the same workspace simultaneously.
- ❌ **Blocking Handoff Waits**: Treating "hand this to a subagent" as permission to block the main chat while waiting for the child. The host is always the responsive orchestrator.
- ❌ **Silent Verification**: Assuming a fix works without running test commands or canary scripts.
- ❌ **Unit-Test-Only Confidence for Hook/Environment Features**: Treating a green `go test ./...` as proof a
  hook-installing or environment-resolution-dependent feature actually works in production. Eight tickets shipped
  with passing, well-written unit tests on 2026-08-31 (`harnez-tool-observability`) while automatic capture was
  completely non-functional in real usage. Manual code review (not tests) caught two cross-ticket integration bugs
  (two independently-tested `PreToolUse` hooks racing once both were installed; a rewrite that broke on shell
  metacharacters an outer shell re-interpreted). But a branch-name-shaped-ticket heuristic that could never match
  this user's actual workflow, and a schema-version guard that trusted a pre-existing file, both passed every unit
  test *and* code review — they were only found by restarting a real session, adding debug logging, and checking
  real output against the real DB.
- ❌ **Deployment State Conflation**: Declaring a remote binary "deployed" or a job "scheduled" based on local build/test success or a clean `scp`/push exit code, without probing the live host (see [DeploymentTransparency.md](DeploymentTransparency.md)).
- ❌ **Blind Revert of Failed Work**: Running `git checkout --`, `git reset --hard`, or `git stash drop` on a failed implementation attempt without first committing it somewhere recoverable. A prose summary of what was tried is not a substitute for the actual diff — it cannot be `git diff`ed, re-applied, or independently re-verified against the gate it was tested against.
- ❌ **Narrow String-Substitution Edits Over Structured Patches**: The existing "prefer
  `apply_patch`/whole-block replacement over narrow string substitution" rule was written from
  intuition; `smarthome`'s `harnez stats` now backs it with numbers — `Edit` failed at **11.1%**
  across 108 calls vs. `apply_patch` at **4.2%** across 24 calls in the same repo (2.6× the rate),
  see `docs/studies/2026-09-04-three-days-to-a-public-release.md` §4.6. Prefer `apply_patch` when
  both are available.
- ❌ **Baking Real Credentials In For A Fast Dev Loop**: Hardcoding real device hostnames, MACs,
  subnets, or credentials "temporarily" to speed up local iteration, intending to scrub before
  publication. `smarthome` did this for two days and had to run a full history-sanitization pass
  (new commits, rewritten tickets) before its public release could ship — a public-release gate
  that was only caught by a human decision, not tooling (`docs/studies/2026-09-04-three-days-to-a-public-release.md`
  §4.3). Start with RFC-1918/example values and a credential-source seam from the first commit;
  wire a secret scanner into the project's `check`/`test` target immediately, not retroactively.
- ❌ **Orphaned Background Tasks**: Leaving background `tail -f`, watch loops, or timers running after work is completed.
- ❌ **Lost Context / Ephemeral-Only Retrospectives**: Discussing important harness friction or bugs in chat without writing them down to `docs/feedback/` or `issues/`.
- ❌ **Rubber-Stamp Reviews**: Running a review pass that does not inspect actual test assertions or file diffs.
- ❌ **Unbounded Doc Ingestion**: Executing whole-file read tools on `AGENTS.md` or bundled reference docs whose summaries are already in the active system prompt.
- ❌ **Unverified Media Publishing**: Publishing or embedding demo reels, WebM files, or UI screenshots on websites or documentation without explicit user confirmation of the visual output.
- ❌ **Prompt Micromanagement**: Overburdening subagent dispatches with redundant base rules, tool definitions, or style guides already present in the harness system prompt.
- ❌ **Friction Noise Over-Reporting**: Emitting repetitive, low-signal friction reports on fast, routine tasks.
- ❌ **Blocking `sleep` Waits**: Using a long `sleep N` — or a loop of short sleeps — to wait out a CI run, deploy, remote queue, or background process. A blocking sleep burns the agent's own turn and context budget for its full duration with no record of what was being waited for if the session is interrupted mid-wait, and a sleep-loop wastes cycles on empty polls instead of yielding control until state actually changes. Prefer letting harness-tracked background work notify on completion; when polling genuinely-external state is unavoidable, use the harness's scheduled-wakeup or interval-loop mechanism (e.g. `/loop`, `manage_task` notifications) so the agent yields between checks, and match the interval to how fast the watched state actually changes rather than a fixed short interval "just in case." Never chain long leading sleeps to route around a harness restriction on blocking sleep — that defeats the restriction's purpose.
