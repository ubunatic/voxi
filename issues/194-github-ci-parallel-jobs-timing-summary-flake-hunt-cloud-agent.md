# GitHub CI: parallel jobs, timing summary, flake hunt

Status: Closed — single job kept (split was slower), race step reports through gotest-report (run 39027438240); fast-forwarded to main

Scope: .github/ only — same design as harnez issue 809 (no flake workflow:
the sprint scopes the flake hunt to harnez and loom).

## Changes
- `.github/gotest-report.go` (shared verbatim with harnez, loom, cati):
  failures, test time, slowest 10 packages per job summary.
- `ci.yaml`: one job (parallel jobs made the seconds-long suite slower, 0.6
  to 1.4 min). `go test -race -json` feeds the summary step; actions moved
  off the deprecated Node 20 majors; `contents: read`, 15 min timeout.

## Verification
- actionlint clean; workflow_dispatch run on the ccr branch.
