---
title: Issue Tracking Practices
weight: 45
---

<!-- harnez:bundled -->
# Issue Tracking Practices — Priority & Metadata Standards

This document defines the canonical standards for tracking bugs, features, and technical tasks across `harnez`-managed repositories. Every project maintains an in-repository issue tracker in `issues/` with consistent priorities, metadata headers, and lifecycle rules.

---

## 1. Issue Priority Schema

Priorities define **scheduling urgency** — the order and immediacy with which work must be addressed.

| Priority | Level | Description & Escalation Trigger | Target SLA / Lifecycle |
|---|---|---|---|
| **P0** | **Critical** | Blocker, data loss, security vulnerability, broken build, or critical regression violating core invariants. Halts regular development ("stop the line"). | Immediate resolution; fix precedes all other tasks. |
| **P1** | **High** | Core functionality broken, major workflow impediment, key API regression, or high-urgency milestone deliverable. No acceptable workaround exists. | Current sprint / next immediate release cycle. |
| **P2** | **Medium** | Normal feature, standard bug fix, performance optimization, UX polish, or refactoring without active blockage. Workaround may exist. | Standard backlog; prioritized during routine planning. |
| **P3** | **Low** | Minor cosmetic glitch, typo, nice-to-have suggestion, speculative idea, or non-urgent documentation improvement. | Opportunistic; addressed when touching related code. |

---

## 2. Priority vs. Severity

Do not conflate **Priority** with **Severity**:

- **Severity** measures **technical impact and damage** (how severely the system is broken).
- **Priority** measures **scheduling urgency** (how quickly the team/agent must fix it).

*Examples:*
- A spelling error in the primary brand header on the homepage is **Low Severity** (cosmetic) but **P1 / High Priority** (reputational urgency).
- A catastrophic memory leak in an obscure, deprecated, unadvertised CLI flag that nobody uses is **Critical Severity** (crash/resource exhaustion) but **P3 / Low Priority** (low operational risk).

---

## 3. Standard Ticket Metadata Schema

Every issue file is placed under `issues/NNN-kebab-case-title.md` (e.g. `issues/042-standardized-issue-priority-schema.md`).

The top of each ticket MUST contain the standardized metadata block:

```markdown
# NNN — Title of the Issue

**Status**: Open | In Progress | Blocked — <reason> | Closed — resolved in <commit> | Draft
**Priority**: P0 (Critical) | P1 (High) | P2 (Medium) | P3 (Low)
**Severity**: Critical | Major | Moderate | Minor
**Category**: Bug | Feature | Architecture | Documentation | Performance | Refactor | Agentic Ergonomics
**Related**: [Doc / Ticket / Commit references]

---

## 1. Problem & Motivation
...

## 2. Technical Specification / Findings
...

## 3. Implementation & Verification Plan
...
```

### Allowed Values

- **Status**:
  - `Open`: Unresolved, ready to be worked on.
  - `In Progress`: Actively being worked on in current session.
  - `Blocked — <reason>`: Waiting on upstream dependency or external resolution.
  - `Closed — <resolution>`: Completed and verified with tests (e.g. `Closed — resolved in 58d1fa3`, `Closed — invalid`).
  - `Draft`: Tentative proposal or placeholder.
- **Priority**: `P0 (Critical)`, `P1 (High)`, `P2 (Medium)`, `P3 (Low)`
- **Severity**: `Critical`, `Major`, `Moderate`, `Minor`
- **Category**: `Bug`, `Feature`, `Architecture`, `Documentation`, `Performance`, `Refactor`, `Agentic Ergonomics`, `Infrastructure`

---

## 4. Issues Index & Archive Conventions

### 4.1 Index Table (`issues/README.md`)

The tracker index `issues/README.md` maintains a synchronized inventory of all tickets. It must be updated whenever a ticket is added, modified, or closed:

```markdown
# Issues

| # | File | Title | Status |
|---|------|-------|--------|
| 001 | [archive/001-diff-clean-wrong-path.md](archive/001-diff-clean-wrong-path.md) | diff and clean operate on wrong file | Closed — resolved |
| 042 | [042-standardized-issue-priority-schema.md](042-standardized-issue-priority-schema.md) | Standardized issue priority schema | Closed |
```

`harnez status` includes an issue tracker linter that verifies:
1. Every ticket on disk is indexed in `issues/README.md`.
2. Every table link points to a valid file (accounting for `issues/` and `issues/archive/`).
3. Ticket status declared in the file matches the status in the index table.
4. No duplicate ticket numbers exist.

### 4.2 Archiving Closed Issues

When an issue is closed and verified, move it to `issues/archive/NNN-kebab-case.md` to keep the active `issues/` directory clean and focused. Update the markdown link in `issues/README.md` accordingly.

---

## 5. Lifecycle Invariants

1. **Test Verification Before Closure**: Never mark a ticket `Closed` without executing the test suite (`go test ./...`, `make check`) and confirming assertion rigor.
2. **Atomic Index Synchronization**: Whenever ticket status changes in the file, immediately update `issues/README.md`.
3. **Immediate Tracker Commit**: After creating or updating issue-tracker files, commit the ticket file and synchronized index immediately in their own small commit. Do not batch tracker metadata with unrelated code or defer it to a later feature-work checkpoint.
4. **Traceability**: Link relevant study notes (`docs/studies/`), retrospectives (`docs/feedback/`), ADRs, and commits in the `**Related**:` header.
