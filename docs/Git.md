---
title: Git Conventions
weight: 63
---

<!-- harnez:bundled -->
# Git conventions

Applies unless overridden by a project's own AGENTS.md.

## Workflow
- Work local, work on the repo's default branch (usually `main`, sometimes `master`) —
  no local feature branches unless the project's Repo Setup says otherwise.
- Commit proactively:
  - after intermediate steps once tests are clean
  - after finished features (with review pass)
  - between iteration attempts on a stuck bug — each attempted fix is a checkpoint you may
    need to roll back to; don't let several "should be fixed now" rounds pile up uncommitted
  - immediately after filing issue-tracker entries (`issues/NNN-*.md` plus
    `issues/README.md`), in their own small commit; tracker entries are
    durable metadata, not behavior changes, and should not be batched with
    unrelated code work
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
