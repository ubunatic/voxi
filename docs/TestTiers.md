# Test Tiers

<!-- harnez:bundled -->

One shared test vocabulary for every repo. Run the smallest tier that exercises
the change, then widen before you commit. Each tier is a Makefile target, so
agents and CI call the same commands everywhere.

## Tiers

| Target | Scope | When |
|---|---|---|
| `make lint` | gofmt check, `go vet`, `harnez lint --check` on tracked files | before every commit; first CI step |
| `make test-pkg PKG=./path/...` | one package, `-race -count=1` | inner loop while editing that package |
| `make test-short` | `go test -short ./...`: fast unit tier | before a commit that touches several packages |
| `make test` | full suite: gofmt, vet, all tests incl. PTY/media/integration | before the last commit of a task |
| `make container-test` | `make lint check` offline in a clean Linux container (podman or docker) | before the last commit; matches CI |
| CI | `make lint`, full tests, `go test -race` | every push |

Non-Go repos keep the same target names and map them to their toolchain.

## Writing Tests for the Right Tier

- **Short tier** — pure logic, in-memory fakes, temp dirs. Guard anything slow
  or environment-bound with `if testing.Short() { t.Skip("...: full tier") }`:
  real PTYs, media decoding, subprocess binaries, network listeners.
- **Package tests** — every package has `_test.go` coverage beside its code.
  `harnez assess --hotspots` lists packages without tests.
- **Terminal output** — test rendering as plain strings at a fixed width and
  height; assert visible width with a display-width helper, not `len`. Reserve
  real PTY tests for terminal lifecycle (raw mode, resize, restore) and put
  them in the full tier.
- **Containers** — tests must pass offline as a non-root user in a fresh git
  checkout; never depend on `$HOME` state, installed binaries, or the network.
  Isolate state with `t.Setenv("HOME", t.TempDir())`.

## Timing Rules

- No wall-clock sleeps to wait for work. Synchronise with channels, barriers,
  `sync.WaitGroup`, or an injected fake clock.
- Assert outcomes, not elapsed time. When a latency budget is the behaviour
  under test, scale it under the race detector: define `const raceFactor` as
  `5` in a `//go:build race` test file and as `1` in a `//go:build !race` one,
  then use `budget := 50 * time.Millisecond * raceFactor`.

- Tests must pass with `-count=1` repeated and in any order; no shared global
  state between tests without reset.

## Quality Gates

- `harnez lint --check` fails on errors; `--check=warning` also fails on
  warnings such as Go size limits (`spec/lint.yaml` in harnez). Repo-wide runs
  list every error but only summarise warnings; `--verbose` lists them all.
- Keep files and functions under the warning limits when you touch them; split
  files by concern with move-only commits that keep the suite green.

## Containers and CI

- `make container-test` mounts the source read-only: a test that writes into
  the checkout (fixtures, golden files) fails there. Write to `t.TempDir()`;
  regenerate fixtures only behind an explicit `-update` flag.
- Tests needing a TTY, audio, GPU or network skip when the repo's container
  flag is set (e.g. `LOOM_CONTAINER_TEST=1`) and list themselves in the CI ticket.
- CI runs `-race` on 2-CPU runners: any wall-clock budget not scaled by
  `raceFactor` fails there first. Fix the budget or prove the property with a
  barrier, never retry.
- CI reports failing tests as annotations, which `gh run view` shows without log
  download rights; keep that step when changing workflows.

<!-- harnez:stop -->
