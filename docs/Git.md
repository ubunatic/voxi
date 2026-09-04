---
title: Git Conventions
weight: 63
---

<!-- harnez:bundled -->
# Git conventions

Applies unless overridden by project-local instructions for coding agents,
commonly stored in an `AGENTS.md` file at the repository root.

## Workflow
- Work local, work on the repo's default branch (usually `main`, sometimes `master`) —
  no local feature branches unless the project's Repo Setup says otherwise.
- Commit proactively:
  - Default preference: commit autonomously after intermediate steps once tests are clean, after finished features (with review pass), between iteration attempts on a stuck bug, and immediately after filing issue-tracker entries.
  - Ask-first harness interaction: If running in a harness that mandates explicit user confirmation for `git commit`, ask the user **upfront before starting work / during session kickoff** for authorization to commit local checkpoints proactively. This enables the user to step away without returning to uncommitted progress.
  - Orchestrator delegation: When an orchestrating agent operates under an ask-first rule, it must clarify commit authority with the user at kickoff and specify whether spawned subagents commit directly or return diffs for the orchestrator to commit on their behalf.
- Review before commit:
  - small fixes: commit directly if all tests pass and existing test assertions remain intact
  - non-trivial changes / features: perform a dedicated review pass (host orchestration or a fresh reviewer subagent) before committing to verify test rigor, doc/ticket accuracy, and codebase clarity
- Do not push unless asked.
- Do not create remote branches or PRs unless asked.

## Commit messages
- Use [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/#summary):
  `type(scope)?: summary`, e.g. `feat(cli): add init command`.
- Common types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`.

## Hygiene
- Do not commit secrets.
- Do not commit large files unless tracked via git-lfs.
- Do not commit PII.

## Remotes
- Canonical origin: `codeberg.org/<username>/<reponame>`.
- A "mirror" may exist at `github.com/<same-username>/<same-repo>`.
- The mirror's default branch is kept in sync via Codeberg's platform sync — do not push it there directly.
- The mirror may carry contributor branches so GitHub users can contribute there.

Only if explicitly asked:
- push to the mirror to keep it in sync
- pull from mirror contributor branches to integrate contributions
