# Quota-1 Guardrails

- **Single-Test Boundary**: Under Quota-1 rules, the agent may only run the test suite once per step/turn.
- **Code Modification Required**: If tests fail or complete, you MUST modify repository source files before running tests again. Repeated test runs without intermediate code modifications are blocked.
- **Clean Tree First**: Before the quota run, check `git status` for changes that are not yours. If there are any, wait or report; do not run.
- **Enforced Test Target**: Execute tests via `make test-q1` (or `harnez exec --quota-1 -- <test-cmd>`).
- **Unauthorized Bypass Forbidden**: Bypassing guardrails via `QUOTA_BYPASS=1` or `HARNEZ_QUOTA_BYPASS=1` is strictly reserved for human developers and CI environments. Agent loops must not set or pass bypass flags.
- **Media & Demo Verification Gate**: Get explicit user confirmation before publishing recordings or screenshots.

## Quota-1 Guardrails

- **Single-Test Boundary**: Under Quota-1 rules, the agent may only run the test suite once per step/turn.
- **Code Modification Required**: If tests fail or complete, you MUST modify repository source files before running tests again. Repeated test runs without intermediate code modifications are blocked.
- **Clean Tree First**: Before the quota run, check `git status` for changes that are not yours. If there are any, wait or report; do not run.
- **Enforced Test Target**: Execute tests via `make test-q1` (or `harnez exec --quota-1 -- <test-cmd>`).
- **Unauthorized Bypass Forbidden**: Bypassing guardrails via `QUOTA_BYPASS=1` or `HARNEZ_QUOTA_BYPASS=1` is strictly reserved for human developers and CI environments. Agent loops must not set or pass bypass flags.
