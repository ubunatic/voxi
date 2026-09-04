---
title: Make Conventions
weight: 62
---

<!-- harnez:bundled -->
# Make Conventions

> **Who this is for** — anyone adding a target to a project Makefile. Reference material: grep it, don't read it.
>
> **Read this if** — you need the `⚙️` sentinel or the self-documenting `help` recipe.
>
> **Takeaways**
> 1. `help` is the first target; bare `make` prints usage.
> 2. The `⚙️` sentinel (and `🤖` for managed targets) replaces per-target `.PHONY` bookkeeping.
> 3. Express order as dependencies, and always run the locally built binary (`./$(BINARY)`).

---

Default language assumed: Go.
Apply to other languages accordingly.

## Structure

- First target is the default goal — always `help`
- All targets declared `.PHONY` using the `⚙️` sentinel trick (see below)
- One blank line between targets

## Variables

Example:

```makefile
BINARY  := harnez              # output binary name
CONFIG  := config.yaml         # default config file
TARGET  := $(HOME)/.claude     # installation target dir
PROJECT := .                   # project root (passed to tool as -p)
PREFIX  ?= /usr/local          # overridable install prefix

export MYAPP_SOME_FEATURE=1
```

- Use `:=` for immediate assignment (most vars)
- Use `?=` for env-overridable vars (`PREFIX`)
- Align `=` signs for readability

## Phony declaration — `⚙️ 🤖` sentinels

```makefile
.PHONY: ⚙️ 🤖  # ⚙️ = manual/once, 🤖 = managed
```

Adding one of the sentinels as a prerequisite on every target (e.g. `build: ⚙️  # ...` or `help: 🤖  # ...`) causes Make to treat all targets as phony without listing each name twice.

- `🤖` represents targets actively **managed** (reconciled/updated) by `harnez` (like `help`).
- `⚙️` represents targets **manually** defined or generated once (like `build`, `test`, `release`), which `harnez` will not automatically overwrite.

## Self-documenting help target

```makefile
_prim := \033[36m
_rst  := \033[0m

help: 🤖  # show this help
	@grep -E '^[a-zA-Z_-]+:.*[⚙🤖].*#+' $(MAKEFILE_LIST) | \
	awk 'BEGIN {FS = ":.*#+ "}; {printf "    $(_prim)%-15s$(_rst) %s\n", $$1, $$2}'
```

Every target that should appear in help gets a `  # description` comment on the same line as the rule header. `help` scrapes them automatically.

## Build dependency pattern

Action targets depend on `build` so the binary is always fresh:

```makefile
build: ⚙️  # build the binary
	go build -o $(BINARY) .

apply: ⚙️ build  # apply config.yaml to the Claude Code config directory
	./$(BINARY) apply -c $(CONFIG) -t $(TARGET) -p $(PROJECT)
```

- `build` rebuilds only when sources change (Make's normal rules apply)
- Action targets invoke `./$(BINARY)` — the locally-built binary, not the one on `$PATH`.
  If needed, the user can override this rule if development is close to his system.

## Install target (Go)

```makefile
install: ⚙️ build  # install the binary to PREFIX/bin (default: /usr/local/bin)
	go install .
	@sudo install -m 0755 $(BINARY) $(PREFIX)/bin/$(BINARY) && \
	  echo "✅ Installed for all users" || echo "⚠️ System install failed"
```

Install approach is usually: do local + try global
- `go install` puts the binary in `$(GOPATH)/bin` (user-local)
- `sudo install -m 0755` copies to `$(PREFIX)/bin` for system-wide availability
- `|| echo …` degrades gracefully when `sudo` is unavailable

## Check target

```makefile
check: ⚙️  # run static analysis and tests
	go vet ./...
	go test ./...

check-fast: ⚙️  # fast local feedback loop
	go test ./...

test: ⚙️ check  # alias for check
```

`make check` is the standard verification target used by development-flow docs.
Always run `go vet` before `go test`; vet catches issues tests may not exercise.
Keep `make test` as a compatibility alias when a repo already exposes it.
Add `make check-fast` when full checks are slow; it should keep broad coverage but use cheap settings. Example: `trafficsim` runs one focused model via `MODEL=...`.

## Deployment target parity

Any project with mutating provisioners (pushes a binary, config, or schedule to a remote host)
must expose the same four self-documenting targets, so deploy/verify is never ad hoc SSH
one-liners. See [docs/practices/DeploymentTransparency.md](../practices/DeploymentTransparency.md)
for why the query targets (`run`/`status`) must probe the live host rather than assume success
from a completed `deploy`.

```makefile
deploy: ⚙️ build  # deploy binary, configs, and cron schedules (DRY=1 for dry-run)
	@scripts/deploy.sh $(if $(DRY),--dry-run)

run: ⚙️  # query live deployment health (process, service, or job status)
	@scripts/deploy.sh --status

status: ⚙️ run  # alias for run

backup: ⚙️  # sync state snapshots from the remote host
	@scripts/backup.sh
```

- `make deploy [DRY=1]` — ships binary, configs, and cron/systemd schedules; `DRY=1` must perform
  a real dry-run against the remote host, not a no-op.
- `make run` / `make status` — read-only: query the actual remote process table, systemd units, or
  crontab, not the local repo state. Keep `status` as an alias when a repo already exposes `run`.
- `make backup` — sync state snapshots (config overlays, data) down from the remote host before a
  risky deploy.
- Every target here queries or mutates a real remote host — treat it like `make smoke` (see
  `docs/practices/AgenticLoop.md`): safe to define, but only run when you intend the live effect.
