---
title: Agentic Loop Practices
weight: 40
---

# Agentic Loop Practices — Multi-Agent Sprint Workflow

This document establishes the canonical practice for orchestrating multi-agent development loops. It defines the lifecycle, synchronization invariants, role archetypes, and quality gates required to conduct rapid, collision-free agentic sprints.

---

## 1. Core Philosophy & Invariants

Agentic software engineering scales effectively when concurrency is structured and single-threaded write locks are strictly preserved.

1. **Parallel Read, Sequential Write**:
   - Multiple subagents may concurrently explore, read, grep, and analyze the codebase.
   - Only **one** agent may modify files, write code, or execute build mutations in a shared workspace at any given time.
   - Concurrent writes produce race conditions, broken intermediate states, git conflicts, and corrupt dependencies.

2. **Canary & Test-Driven Verification**:
   - Every task must be verified with real test executions (`go test ./...`, `make smoke`, canary probes) before declaring completion.
   - Never assume an edit succeeds without observing passing assertions.

3. **Zero Zombie Guarantee**:
   - Every spawned background process, schedule timer, or subagent must be tracked, accounted for, and explicitly terminated before concluding a session.
   - Orphaned processes, lingering watch commands, and abandoned poll loops degrade system resources and corrupt future test runs.

4. **In-Repository Single Source of Truth**:
   - Tickets (`issues/*.md`), architectural decisions, retrospectives (`docs/feedback/`, `docs/studies/`), and specifications (`spec/`) live in the git tree.
   - Session context, learnings, and friction logs must be committed to the repository rather than abandoned in ephemeral agent chat contexts.

5. **Context Discipline & Range-Bounded Ingestion**:
   - Never execute whole-file reads on files already present in the active system prompt (`AGENTS.md`, `CLAUDE.md`, system rules).
   - Prefer index consultation, `grep_search`, and range-bounded reads (`StartLine`/`EndLine`) over bulk document ingestion. In-file warning banners are ineffective once returned into message history.

6. **Media & Demo Verification Gate**:
   - When creating, updating, or adding media assets (e.g. reels, WebM demos, terminal recordings, screenshots) intended for documentation or websites, **always ask the user for explicit confirmation** that the recorded visual output matches their exact expectations before publishing or embedding it.
   - Never automatically publish or embed unverified recordings (guarding against invisible typing, missing UI frames, or unexpected rendering artifacts).

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
         (manage_task list/kill, drain)
                          |
                          v
       Phase 5: Flow Quality Retrospective
       (docs/feedback/, docs/studies/, issues)
```

### Phase 1: Parallel Advisory Discovery (Read-Only)
- **Goal**: Rapidly audit requirements, discover existing implementations, identify affected files, and evaluate technical feasibility without code collisions.
- **Mechanics**:
  - The Host Orchestrator spawns concurrent read-only advisor subagents (e.g. one per ticket or feature area).
  - Advisors perform deep grep/read searches, evaluate whether requirements are already partially or fully met, and identify exact line ranges for changes.
  - Advisors return concise findings and structured implementation plans to the Host.
- **Constraints**: Advisors must never write files, run mutating commands, or spawn untracked side effects.

### Phase 2: Sequential Development & Test Verification (Single-Threaded)
- **Goal**: Implement planned changes cleanly, incrementally, and with continuous test verification.
- **Mechanics**:
  - The Host Orchestrator (or a dedicated dev subagent executing sequentially) addresses tasks one ticket at a time.
  - Test-Driven Verification: Write or adapt unit tests alongside or prior to code changes.
  - Validate intermediate milestones with fast test suites (`go test ./...`).
  - Keep the workspace in a compilable, passing state at every step.

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

### Phase 4: Process & Subagent Hygiene (Teardown & Drain)
- **Goal**: Prevent zombie accumulation, orphan processes, and stuck background tasks.
- **Mechanics**:
  - Inspect running background tasks (`manage_task list`).
  - Explicitly kill or drain completed, idle, or lingering background jobs, schedule timers, and watch subprocesses.
  - Terminate child subagents (`manage_subagents kill / kill_all`) that have finished their tasks.
  - Ensure the host and system state is pristine.

### Phase 5: Agentic Flow Quality Retrospective (Learning Capture)
- **Goal**: Continually refine agent workflows, document tooling friction, and persist session insights.
- **Mechanics**:
  - Record session friction, harness observations, and process recommendations in `docs/feedback/` (agentic workflow feedback) or `docs/studies/` (in-depth engineering case studies).
  - Update issue tracker status (`issues/README.md`) and run `harnez status` to ensure zero drift between issues and indices.
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
| **Ephemeral Advisor** | Read-Only Tools (grep, find, read, url) | Audits tickets, performs feasibility research, identifies code paths | Ephemeral (terminated after Phase 1) |
| **Dev Worker** | Write Tools, Compiler, Test Runner | Implements concrete changes, writes unit tests, ensures compilation | Single-threaded per workspace |
| **Independent Reviewer** | Read-Only Tools, Diff Inspection | Audits git diff against acceptance criteria, verifies test rigor | Ephemeral (spawned in Phase 3) |

---

## 6. Practical Recipes & Anti-Patterns

### Anti-Patterns to Avoid
- ❌ **Parallel Writing**: Spawning multiple subagents with write permissions on the same workspace simultaneously.
- ❌ **Silent Verification**: Assuming a fix works without running test commands or canary scripts.
- ❌ **Orphaned Background Tasks**: Leaving background `tail -f`, watch loops, or timers running after work is completed.
- ❌ **Lost Context / Ephemeral-Only Retrospectives**: Discussing important harness friction or bugs in chat without writing them down to `docs/feedback/` or `issues/`.
- ❌ **Rubber-Stamp Reviews**: Running a review pass that does not inspect actual test assertions or file diffs.
- ❌ **Unbounded Doc Ingestion**: Executing whole-file read tools on `AGENTS.md` or bundled reference docs whose summaries are already in the active system prompt.
- ❌ **Unverified Media Publishing**: Publishing or embedding demo reels, WebM files, or UI screenshots on websites or documentation without explicit user confirmation of the visual output.
- ❌ **Prompt Micromanagement**: Overburdening subagent dispatches with redundant base rules, tool definitions, or style guides already present in the harness system prompt.
- ❌ **Friction Noise Over-Reporting**: Emitting repetitive, low-signal friction reports on fast, routine tasks.

