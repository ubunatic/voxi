<!-- harnez:variant=lite -->
# Issue Tracking Rules (Lite)

In-repo tracker in `issues/`, one file per ticket (`issues/NNN-kebab-case-title.md` or `issues/GH<n>-kebab-case-title.md`). Full doc with rationale: `harnez docs variant issue-tracking full`.

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
- Cloud agents use `harnez issues new --remote "Title"`: authenticated `gh issue create` on the repo's `github` remote allocates permanent `GH<n>`, writes `issues/GH<n>-<slug>.md` with `# GH<n> — Title`, and links the GitHub body to that path. Missing/unauthenticated gh fails; no local fallback counter. Failures after creation identify the existing GitHub issue.
- Setting `CLAUDE_CODE_REMOTE` (including empty) selects remote mode automatically. `new` prints the selected mode to stderr. GH IDs cannot be renumbered; local next-number allocation ignores them.
- All status verbs, show/find/index/lint accept both namespaces: `GH42` and `gh42` select GH42; `42` stays local 042. README sorts local tickets first, then GH IDs numerically. Duplicate GH IDs are errors; reconcile the files preserving the ID (local renumber repair below does not apply).
- `harnez issues <verb> <n> [reason]` — Update Status, regenerate index, commit in one step (`open`, `start`, `block` [reason req], `close`, `done` [alias for close], `draft`).
- `harnez issues close <n> [reason]` (or `done`) — Already Closed: exit 0, preserve existing reason, no writes or commit. Use `--amend-reason` to explicitly correct the reason in one commit; requires an already-Closed ticket. Close refuses unless a commit outside `issues/` names the ticket; `--no-code` for a ticket that needed no code.
- `harnez issues close <n> [reason] --model <spec> --session <id>` — Record `**Closed-By**` under Status; defaults are `HARNEZ_AGENT_MODEL` and `HARNEZ_SESSION_ID`, with flags taking precedence. Missing parts are omitted, generated indexes exclude this line, and a later close replaces it.
- `harnez issues mv <old> [new]` — Renumber ticket, rename file, rewrite title header and exact filename references in issues/ and docs/, resync index, commit. Bare numbers stay unchanged.
- Duplicate numbers after a merge: `harnez index` and `harnez status` fail with all colliding paths. Run `harnez issues mv -d . issues/NNN-extra-ticket.md` for each extra file (next free number by default); use the path because the number is ambiguous. If the title lacks a number, add `# NNN — Title` before retrying.
- On a pushed branch, prefer `harnez issues reconcile [--dry-run]`: renumbers only your own colliding tickets (added in `<ref>..HEAD`), rewrites references, repairs indexes, commits once, rolls back on failure. Agents use `harnez issues reconcile --pull` instead of `git pull` (`--fetch` previews only). No hook runs it implicitly.
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

- `issues/README.md` lists active tickets only (Open, In Progress, Blocked, Draft) and links to generated `issues/BACKLOG.md` and `issues/CLOSED.md`.
- `harnez index` regenerates all three indexes; `harnez index --stage` also stages only indexes rewritten by that run. `harnez issues lint` checks completeness across them. Never hand-edit rows.
- `harnez find -d <repo> issues -a status:active` searches the active set. `harnez issues lint` and `harnez issues status` warn above the active cap configured in `spec/issues.yaml`.
- **Backlog**: defer with `harnez issues backlog <n…> [reason]`, return with `promote <n…>`; `is:open` excludes Backlog, `status:unresolved` includes it.
- **Duplicates**: `harnez issues new` refuses when `find issues similar` scores a match at or above 0.65 in any status; reopen/promote it, or pass `--force`.
- **Generated files** carry a `managed by harnez index` top line and `harnez:begin/end` blocks; `.gitattributes` uses `merge=union` and the `harnez init` post-merge hook rebuilds them on merge. A one-sided deletion wins over an unchanged row; renames are delete+add, and competing row edits conflict.
- **Archiving**: Closed tickets may move to `issues/archive/NNN-kebab-case.md`; `harnez index` keeps them listed in `issues/CLOSED.md`.

## 5. Lifecycle Invariants

1. **Test Verification Before Closure** — Never mark `Closed` without running test suite and verifying assertions.
2. **Atomic Index Sync** — Status changes in files must immediately reflect in `issues/README.md` (`harnez index` or `harnez issues <verb>`).
3. **Immediate Tracker Commit** — Commit ticket updates and synced index immediately in their own small commit; do not batch behind code changes.
4. **Traceability** — Link study notes, ADRs, tickets, commits in `**Related**:`.
5. **Study Findings Become Tickets** — File every actionable study/retro follow-up in its owning repo, link the source study from the ticket, list ticket numbers in the study, and justify non-goals before completing the phase.
6. **Closing Is Part Of Done** — A task/session is NOT done until every touched ticket has `Status` closed and `issues/README.md` indexed.
7. **Goal-Centric & As-Needed Milestones** — For new tickets only, define a `/goal` or a clear Goal statement and acceptance criteria; no backlog migration is required. Only decompose into numbered milestones (M1, M2...) when multi-step staged execution is truly needed; otherwise keep issues lean and brief. Agents picking up an issue must check live code status before beginning work.

<!-- harnez:stop -->
