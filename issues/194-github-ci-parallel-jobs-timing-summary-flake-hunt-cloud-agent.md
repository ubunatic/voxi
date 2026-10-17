# GitHub CI: parallel jobs, timing summary, flake hunt

Status: In Progress — owned by the Claude cloud agent (ccr branch)

Scope: .github/ only — same design as harnez issue 809 (no flake workflow:
the sprint scopes the flake hunt to harnez and loom).

## Changes
- `.github/gotest-report.go` (shared verbatim with harnez, loom, cati):
  failures, test time, slowest 10 packages per job summary.
- `ci.yaml`: parallel `lint` (gofmt, vet, build), `unit` (`go test -short`),
  `race`; `report` writes the CI wall time. The separate `go test ./spec/...`
  step is dropped: `go test ./...` already runs it. Per-job module and build
  caches; actions moved off the deprecated Node 20 majors; `contents: read`.

## Verification
- actionlint clean; workflow_dispatch run on the ccr branch.
