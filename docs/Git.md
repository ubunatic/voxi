---
title: Git Conventions
weight: 63
---

<!-- harnez:bundled -->
# Git conventions

Applies unless overridden by project-local instructions for coding agents,
commonly stored in an `AGENTS.md` file at the repository root.

## Workflow
- Workflow follows `git.mode` in `.harnez/repo.yaml` (key absent = solo; set it with
  `harnez init --repo-mode <mode> [--protected-main]`; the project's Repo Setup restates it):
  - `solo`: work local, on the repo's default branch (usually `main`, sometimes `master`);
    no local feature branches.
  - `team` / `fork`: work on a local topic branch and open a PR. Ticket commits ride on the
    current topic branch.
    - `team`, AGit by default: on a forge with AGit (Forgejo, Gitea), tickets and small PRs reuse
      one local branch and push no branch: `git push origin HEAD:refs/for/<default-branch>/<topic>`.
      Pushes update the topic's open PR, and once it is merged or closed the next push opens a new
      PR. A topic is per pusher: by default the pusher's forge username (e.g. `flashcoder`), and
      "push to main" means this AGit push to `refs/for/main/<topic>`, never a direct push to
      `main`. An update that is not a fast-forward is rejected; do not override that with
      `-o force-push=true`. After a squash or rebase merge, reset the local branch to
      `origin/<default-branch>`, or the merged commits reappear in the next PR.
    - Otherwise one branch per ticket or sprint (`issue/<n>-<slug>`, `sprint/<n>`), pushed: for
      work others push to, long sprints that need rebasing, forks, and GitHub, which has no AGit.
  - `protected_main` (team/fork only): do not commit to or push the default branch unless the
    repository admin explicitly authorizes it in the current session, and never force-push.
    `harnez issues` and other auto-committing commands refuse on it and print the `git switch -c`
    command to run. `harnez release` is the exception: the admin, or an agent asked to release,
    may run it there; the forge's branch protection decides whether the push is accepted.
- Commit proactively:
  - Default preference: commit autonomously after intermediate steps once tests are clean, after finished features (with review pass), between iteration attempts on a stuck bug, and immediately after filing issue-tracker entries.
  - Ask-first harness interaction: If running in a harness that mandates explicit user confirmation for `git commit`, ask the user **upfront before starting work / during session kickoff** for authorization to commit local checkpoints proactively. This enables the user to step away without returning to uncommitted progress.
  - Orchestrator delegation: When an orchestrating agent operates under an ask-first rule, it must clarify commit authority with the user at kickoff and specify whether spawned subagents commit directly or return diffs for the orchestrator to commit on their behalf.
- Review before commit:
  - small fixes: commit directly if all tests pass and existing test assertions remain intact
  - non-trivial changes / features: perform a dedicated review pass (host orchestration or a fresh reviewer subagent) before committing to verify test rigor, doc/ticket accuracy, and codebase clarity
- Do not push until the user confirms pushing. One confirmation covers the session:
  push later finished work without asking again, unless the user narrows or revokes consent.
- Solo: "push" means `main` and tags to both dev remotes. Ask again only for force pushes,
  new remote branches, or a remote that diverged. Team/fork: "push" means the topic branch.
- Solo: do not create remote branches or PRs unless asked. Team/fork: pushing the topic
  branch is the delivery; the PR is opened by the host or owner (`tea`/`gh` per forge).

## Commit messages
- Use [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/#summary):
  `type(scope)?: summary`, e.g. `feat(cli): add init command`.
- Common types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`.
- Mark a breaking change with `!` after the type or scope and a `BREAKING CHANGE:` footer, e.g.:
  ```
  feat(cli)!: drop the --old flag

  BREAKING CHANGE: the --old flag is removed; use --new instead.
  ```

## Hygiene
- Do not commit secrets.
- Do not commit large files unless tracked via git-lfs.
- Do not commit PII.
- Harnez hooks warn when starting a session or committing on a detached HEAD.
- Cross-repo tag and spec reads must never check out tags in a shared checkout; use
  `git -C <repo> show <tag>:<path>`, `git worktree add`, or scratch clones. Checking out
  a tag leaves the repository in a detached HEAD state and risks causing subsequent agent
  commits in that shared checkout to land off-branch.

## Remotes
- Two dev remotes: Codeberg (`codeberg.org/<username>/<reponame>`) and GitHub (same path).
  Development happens on both; neither is a mirror.
- Codeberg is canonical for releases: `go get` resolves there, so tags go to Codeberg first.
- Keep both `main` branches equal: every `main` update fast-forwards and goes to both remotes.
  Never force either `main`; never set a push mirror (it overwrites the other side).
  A branch filter does not make a mirror safe: a Codeberg mirror filtered to `main` still
  coincided with every other GitHub branch being deleted and the open PRs on them closing.
- Check every destination for divergence before pushing anything. `harnez release` uses
  all Git remotes configured in the repo, checks branch and release tag updates first,
  then pushes both atomically per remote. A later connection failure or concurrent remote
  update can still interrupt synchronization; resolve the error and retry with `--continue`.
- `uman push` uses the upstream by default. It pushes multiple remotes only when
  `push.default_remotes` lists them in `.uman.toml`; it does not sync tags or check all
  destinations before the first push. Do not assume an unconfigured `uman push` syncs both.

### Keeping GitHub `main` level, without CI or a mirror
- Before a Jules session on `main`, harnez fast-forwards GitHub `main` to Codeberg's `main` with one
  plain push: no force, prune or mirror. A failed or diverged sync warns and names the commit Jules
  will read. Settings live in `spec/agent.yaml` (`jules.sync_*`).
- Owner's clone, optional: one remote `both` whose push URLs are Codeberg and GitHub, so one push
  sends `main` to both. `origin` keeps one URL, because a hook can fail a whole push but only for
  that remote:
  ```
  git remote add both <codeberg-url>
  git remote set-url --push both <codeberg-url>
  git remote set-url --add --push both <github-url>
  git config remote.pushDefault both
  git config remote.both.push refs/heads/main:refs/heads/main
  ```
  Use the exact strings that `git remote get-url --push origin` and `git remote get-url github`
  print for `<codeberg-url>` and `<github-url>`. The release skip compares URL strings exactly, so
  `.git` suffixes, `ssh://` against `https://`, or a trailing slash make it miss the match.
  A `pre-push` hook on this clone can refuse a push to GitHub unless Codeberg already holds the
  commit. `harnez release` skips a remote whose push URLs are all pushed by other remotes, so
  `both` does not run during a release.
  Pitfalls for such a hook: it runs during `git push --dry-run`, it can fail only the whole push to
  that URL (not one ref), and it runs with empty stdin when that URL is already up to date. A plain
  `git push` here sends `main` even from a topic branch. The hook lives in `.git/hooks`, so it is
  per clone and unversioned.
- Contributor clone on a forge with AGit: send each push to the topic's PR without typing the refspec:
  ```
  git config remote.origin.push HEAD:refs/for/main/<topic>
  ```
  Replace `<topic>` with the topic name in that clone.

### Branches from either remote
Cloud agents (Claude Code web, Jules, Codex) usually work on GitHub branches.
- At start and before the final push, merge Codeberg `main`:
  `git fetch https://codeberg.org/<username>/<reponame> main && git merge FETCH_HEAD`.
- Reserve ticket numbers after that fetch; on a clash, renumber yours.
- Integrate by fast-forward, then push `main` (and tags) to both remotes
  (`<cb>`/`<gh>` = your remote names, e.g. `origin`/`github`):
  `git merge --ff-only <branch>`, then check both destinations before any actual push:
  `git push --dry-run --atomic <cb> main --tags && git push --dry-run --atomic <gh> main --tags`.
  After both checks succeed:
  `git push --atomic <cb> main --tags && git push --atomic <gh> main --tags`.

<!-- harnez:stop -->
