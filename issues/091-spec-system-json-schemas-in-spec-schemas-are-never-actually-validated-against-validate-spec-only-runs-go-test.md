# 091 — Spec system: JSON Schemas in spec/schemas/ are never actually validated against

**Status**: Open
**Priority**: P2 (Medium)
**Severity**: Design gap (spec system doesn't do what it documents)
**Category**: Spec System
**Related**: [spec/schemas/actions.schema.json](../spec/schemas/actions.schema.json), [spec/schemas/models.schema.json](../spec/schemas/models.schema.json), [spec/schemas/monitor.schema.json](../spec/schemas/monitor.schema.json), [Makefile](../Makefile) (`validate-spec` target), [docs/Spec.md](../docs/Spec.md)

---

## 1. Problem

`docs/Spec.md` states as a core principle: "JSON Schema for validation: Every
YAML file must have a companion JSON Schema (`spec/schemas/*.schema.json`)
for automated IDE validation, editor auto-completion, and **CI/pre-commit
verification**" and lists "Unvalidated YAML: Maintaining YAML files without a
corresponding JSON Schema" as an anti-pattern, implying the schema is an
enforced, load-bearing artifact.

In practice:

- `make validate-spec` (Makefile:100-101) is just `go test ./spec/...` — it
  runs the hand-written Go validation inside `parseActionSpec`,
  `parseModelSpec`, `parseMonitorSpec` (structural checks written directly in
  Go: required fields, category enums, key-collision checks, etc.), not a
  JSON Schema validator run against `spec/schemas/*.schema.json`.
- No Go module in `go.mod`/`go.sum` implements JSON Schema validation (no
  `santhosh-tekuri/jsonschema`, `xeipuuv/gojsonschema`, or similar).
- `grep -l schema spec/*_test.go` returns nothing — none of the three spec
  test files reference the schema files at all.

So the three `.schema.json` files are pure documentation/IDE-hint artifacts
(consumed only by an editor's `yaml-language-server` extension, if the user
has one configured) with **no automated enforcement** that they stay
consistent with what the Go loaders actually accept or with the YAML they
describe. A schema and a YAML file can drift from each other indefinitely —
e.g. someone loosens or tightens a loader's Go validation without updating
the matching schema property, or edits the schema without any pipeline step
noticing the loader now accepts (or rejects) something the schema disagrees
with — and neither `make check` nor `make validate-spec` would ever catch it.

## 2. Impact

- The project's own quality invariant #3, "No schema drift: every property
  used in YAML is declared in its schema," is currently unverifiable by
  tooling — it can only be maintained by manual discipline.
- New contributors (or an LLM agent) editing `spec/*.yaml` have no automated
  feedback if they add a property the schema doesn't declare, undermining
  the stated purpose of `additionalProperties: false` in all three schemas.

## 3. Suggested Fix

Either:
- Add real schema validation to `validate-spec` — a lightweight pure-Go JSON
  Schema validator (e.g. `github.com/santhosh-tekuri/jsonschema/v5`) run
  against each `spec/*.yaml` (converted to JSON) and its
  `spec/schemas/*.schema.json` sibling, as a `go test` or a small
  `validate-spec` script step; or
- If a schema-validation dependency is considered unwanted the docs should
  be corrected instead of the code — reword `docs/Spec.md` to state plainly
  that the schemas are IDE-only hints, not CI-enforced, so the invariant list
  doesn't over-promise what the pipeline actually checks.
