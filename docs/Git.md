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
- Two dev remotes: Codeberg (`codeberg.org/<username>/<reponame>`) and GitHub (same path).
  Development happens on both; neither is a mirror.
- Codeberg is canonical for releases: `go get` resolves there, so tags go to Codeberg first.
- Keep both `main` branches equal: every `main` update fast-forwards and goes to both remotes.
  Never force either `main`; never set a push mirror (it overwrites the other side).

### Branches from either remote
Cloud agents (Claude Code web, Jules, Codex) usually work on GitHub branches.
- At start and before the final push, merge Codeberg `main`:
  `git fetch https://codeberg.org/<username>/<reponame> main && git merge FETCH_HEAD`.
- Reserve ticket numbers after that fetch; on a clash, renumber yours.
- Integrate by fast-forward, then push `main` (and tags) to both remotes
  (`<cb>`/`<gh>` = your remote names, e.g. `origin`/`github`):
  `git merge --ff-only <branch> && git push <cb> main && git push <gh> main`.

<!-- harnez:stop -->
