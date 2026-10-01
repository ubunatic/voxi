<!-- harnez:variant=lite -->
# Issue Tracking Rules (Lite)

In-repo tracker in `issues/`, one file per ticket (`issues/NNN-kebab-case-title.md`). Full doc in `docs/IssueTracking.md`.

## 1. Priority Schema — Scheduling Urgency

| Priority | Level | Description | Target SLA |
|---|---|---|---|
| **P0** | Critical | Blocker, data loss, security vuln, broken build. Stop the line. | Immediate; precedes all other work |
| **P1** | High | Core feature broken, major workflow blocker, API regression. No workaround. | Current sprint / next release |
| **P2** | Medium | Normal feature, standard bug fix, perf, polish, non-blocking refactor. | Standard backlog |
| **P3** | Low | Minor cosmetic glitch, typo, speculative idea, non-urgent doc fix. | Opportunistic |

## 2. Priority ≠ Severity

- **Severity** = technical impact (how broken).
- **Priority** = scheduling urgency (how fast to fix).
- *Example:* Brand header typo = Low Severity, **P1**. Obscure dead flag crash = Critical Severity, **P3**.

## 3. Standard Ticket Metadata & Operations

### 3.1 Allocation & Status CLI
- `harnez find issues next [--json]` — Read-only next free number (`max(allocated)+1`). Never ad hoc shell (`ls | grep`).
- `harnez issues new ["Title"] [--json]` — Atomically reserve number + create placeholder with `O_CREATE|O_EXCL`. Write directly to printed path.
- `harnez issues <verb> <n> [reason]` — Update Status, regenerate index, commit in one step (`open`, `start`, `block` [reason req], `close`, `done` [alias for close], `draft`).
- `harnez issues mv <old> [new]` — Renumber ticket, rename file, rewrite title header, resync index.
- `harnez issues list [filter]` — List matching tickets (default `is:open`).

### 3.2 Metadata Header (Top of Every Ticket)

```markdown
# NNN — Title of the Issue

**Status**: Open | In Progress | Blocked — <reason> | Closed — <resolution> | Draft
**Priority**: P0 (Critical) | P1 (High) | P2 (Medium) | P3 (Low)
**Severity**: Critical | Major | Moderate | Minor
**Category**: Bug | Feature | Architecture | Documentation | Performance | Refactor | Agentic Ergonomics | Infrastructure
**Related**: [Doc / Ticket / Commit references]

---

## 1. Problem & Motivation
...
## 2. Technical Specification / Findings
...
## 3. Implementation & Verification Plan
...
```

- **Status rules**: `Blocked` requires `<reason>`. `Closed` requires `<resolution>` in words (do NOT record closing commit hash; trace via `git log`). Optional `— <note>` allowed on `Open`, `In Progress`, `Draft`.

## 4. Issues Index & Archive Conventions

- `issues/README.md` is managed by `harnez index` (idempotent, supports `--check` for CI diff/drift verification). Never hand-edit rows.
- `harnez status` lints index completeness, link validity, status parity, and duplicate numbers.
- **Archiving**: Move closed tickets to `issues/archive/NNN-kebab-case.md` and update index link.

## 5. Lifecycle Invariants

1. **Test Verification Before Closure** — Never mark `Closed` without running test suite and verifying assertions.
2. **Atomic Index Sync** — Status changes in files must immediately reflect in `issues/README.md` (`harnez index` or `harnez issues <verb>`).
3. **Immediate Tracker Commit** — Commit ticket updates and synced index immediately in their own small commit; do not batch behind code changes.
4. **Traceability** — Link study notes, ADRs, tickets, commits in `**Related**:`.
5. **Closing Is Part Of Done** — A task/session is NOT done until every touched ticket has `Status` closed and `issues/README.md` indexed.
6. **Goal-Centric & As-Needed Milestones** — For new tickets only, define a `/goal` or a clear Goal statement and acceptance criteria; no backlog migration is required. Only decompose into numbered milestones (M1, M2...) when multi-step staged execution is truly needed; otherwise keep issues lean and brief. Agents picking up an issue must check live code status before beginning work.

<!-- harnez:stop -->
