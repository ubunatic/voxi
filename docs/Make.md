<!-- harnez:variant=lite -->
# Make Rules (Lite)

Language assumed: Go. Adapt to others.

## Structure

- 1st target = default goal = `help` (bare `make` prints usage)
- every target declared phony via the `⚙️`/`🤖` sentinel prereq — never per-target `.PHONY` names
- 1 blank line between targets
- every help-visible target carries `  # description` on its rule header line

## Variables

```makefile
BINARY  := harnez              # output binary name
CONFIG  := config.yaml         # default config file
TARGET  := $(HOME)/.claude     # install target dir
PROJECT := .                   # project root (tool's -p)
PREFIX  ?= /usr/local          # ?= env-overridable; := immediate, use for all others

export MYAPP_SOME_FEATURE=1
# align the = signs
```

## Sentinels

```makefile
.PHONY: ⚙️ 🤖
```

- `build: ⚙️  # ...` / `help: 🤖  # ...` — sentinel as prereq on EVERY target makes Make treat
  all as phony without naming each twice
- `🤖` = managed by harnez (reconciled/updated, e.g. `help`)
- `⚙️` = manual/generated once (`build`, `test`, `release`); harnez never overwrites

## help

```makefile
_prim := \033[36m
_rst  := \033[0m

help: 🤖  # show this help
	@grep -E '^[a-zA-Z_-]+:.*[⚙🤖].*#+' $(MAKEFILE_LIST) | \
	awk 'BEGIN {FS = ":.*#+ "}; {printf "    $(_prim)%-15s$(_rst) %s\n", $$1, $$2}'
```

## build + action targets

```makefile
build: ⚙️  # build the binary
	go build -o $(BINARY) .

apply: ⚙️ build  # apply config.yaml to the Claude Code config dir
	./$(BINARY) apply -c $(CONFIG) -t $(TARGET) -p $(PROJECT)
```

- action targets depend on `build` -> binary always fresh; `build` rebuilds only on source change
- always run `./$(BINARY)` (locally built), never `$PATH`'s; user may override this rule

## install (Go) — do local + try global

```makefile
install: ⚙️ build  # install the binary to PREFIX/bin (default: /usr/local/bin)
	go install .
	@sudo install -m 0755 $(BINARY) $(PREFIX)/bin/$(BINARY) && \
	  echo "✅ Installed for all users" || echo "⚠️ System install failed"
```

`go install` -> `$(GOPATH)/bin` (user-local); `sudo install -m 0755` -> `$(PREFIX)/bin`
(system-wide); `|| echo` degrades gracefully when sudo is unavailable.

## check

```makefile
check: ⚙️  # run static analysis and tests
	go vet ./...
	go test ./...

check-fast: ⚙️  # fast local feedback loop
	go test ./...

test: ⚙️ check  # alias for check
```

- `check` = standard verification target of the dev-flow docs; `go vet` ALWAYS before `go test`
  (vet catches what tests miss)
- keep `test` as compat alias where a repo already exposes it
- add `check-fast` when full checks are slow: broad coverage, cheap settings (e.g. trafficsim
  runs one focused model via `MODEL=...`)

## deployment parity

Any project with mutating provisioners (pushes binary/config/schedule to a remote host) MUST
expose these four, so deploy/verify is never ad-hoc SSH one-liners. See the optional
`deployment-transparency` practice for why `run`/`status` must probe the live host instead of
inferring success from a completed `deploy`.

```makefile
deploy: ⚙️ build  # deploy binary, configs, and cron schedules (DRY=1 for dry-run)
	@scripts/deploy.sh $(if $(DRY),--dry-run)

run: ⚙️  # query live deployment health (process, service, or job status)
	@scripts/deploy.sh --status

status: ⚙️ run  # alias for run

backup: ⚙️  # sync state snapshots from the remote host
	@scripts/backup.sh
```

- `deploy [DRY=1]` — ships binary, configs, cron/systemd schedules; `DRY=1` = REAL dry-run
  against the remote host, not a no-op
- `run`/`status` — read-only: query real remote process table / systemd units / crontab, never
  local repo state; keep `status` as alias where `run` already exists
- `backup` — pull state snapshots (config overlays, data) down before a risky deploy
- all four touch a real remote host — like `make smoke` (@docs/AgenticLoop.md): safe to define,
  run only when you intend the live effect

<!-- harnez:stop -->
