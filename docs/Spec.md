---
title: Spec System Reference
weight: 30
---

<!-- harnez:bundled -->
# Spec-Driven Architecture — Authoritative Reference

The `spec/` directory is **application code**, not runtime user configuration. Treat spec files with the same rigour as source code: every change must be intentional, every object must be consumed, and specs must always be validated against formal schemas.

---

## 1. Core Principles

> **Spec files are the single source of truth. Application code must not duplicate or shadow spec values.**

### Standard Format: YAML + JSON Schema
- **YAML for authoring**: YAML provides concise, comment-friendly, and human/LLM-readable specification definitions without syntax noise or string escaping friction. Plain JSON files are **not** promoted for authoring due to lack of comments and strict syntax friction.
- **JSON Schema for validation**: Every YAML file must have a companion JSON Schema (`spec/schemas/*.schema.json`) for automated IDE validation, editor auto-completion, and CI/pre-commit verification.
- **Embedded application code**: In compiled languages, embed the `spec/` directory directly into the binary (e.g. Go `//go:embed`, Rust `include_str!`). The spec is part of the application artifact, not external config.
- `spec/glossary.yaml` (`spec/schemas/glossary.schema.json`) — machine-readable ubiquitous-language glossary, canonical terms, and forbidden wording patterns

### Project-local Harnez configuration
Harnez repositories may also contain `.harnez/repo.yaml`, validated by
`spec/schemas/repo.schema.json`. This is runtime project configuration, not an
application spec: it records the project's detected or explicitly selected Harnez
features, while `config.yaml` remains the embedded/global definition used by
`apply`. The repository file has a required `version: 1`, strict feature-area
objects, and rejects unknown keys.

```yaml
version: 1
issues:
  tracker: harnez   # harnez | none
  merge_driver: true
docs:
  studies: true
  copies: [golang, bash]
rules:
  quota: false
make:
  scaffold: true
  container_test: false
```

`init` writes the file on first initialization after detecting the repo's issue
format, existing studies directory, copied language docs, quota section or
`test-q1` target, existing `container-test` target, and existing Makefile. It validates
later reads against the schema and leaves an existing file unchanged unless a setup
flag records an override. `docs.copies` records optional detected or explicitly selected
docs; later runs copy only recorded choices and print an `init --docs <name>` hint for
newly detected optional docs. Docs marked `default: true` in `config.yaml` remain part of
the baseline. `make.scaffold` gates Makefile creation and Harnez target injection. It is
optional in the schema so existing repo.yaml files stay valid; when omitted it defaults
to true, and initial detection sets it true when a Makefile exists. `issues.tracker` keeps
the file-first Harnez tracker as the recommended default; `none` records that this repo
uses another approach or has
no file-based tracker. `issues.active_cap` optionally sets this repository's active
ticket warning threshold; when omitted, the value in `spec/issues.yaml` applies.
`make.container_test` and `rules.quota` gate the respective init behavior. When the tracker is `none` or `docs.studies` is false, init writes
a reminder into `.harnez/rules/Index.md`, which the generated AGENTS.md rules
header directs agents to read.

### Anti-Patterns to Avoid
- ❌ **Shadowing / hardcoded fallbacks**: Hardcoding fallback maps or default string tables in source code that mirror spec content.
- ❌ **Undocumented actions/keys**: Handling IDs, keys, or action names in code that are not defined in the spec.
- ❌ **Unvalidated YAML**: Maintaining YAML files without a corresponding JSON Schema.
- ❌ **Silent drift**: Adding properties to YAML that are not declared in the JSON Schema.

---

## 2. Standard Layout

```
spec/
├── schemas/
│   ├── actions.schema.json     # Schema for action registry
│   ├── layout.schema.json      # Schema for UI/view layout
│   └── config.schema.json      # Schema for defaults
├── actions.yaml                # Actions, buttons, hotkeys, commands
├── layout.yaml                 # Structural layout, views, ordering
└── strings.yaml                # Labels, hints, messages, titles
```

Every YAML file references its schema at the top:
```yaml
# yaml-language-server: $schema=schemas/actions.schema.json
```

`spec/telemetry.yaml` is a Harnez-specific non-UI example: it owns embedded SQL
statements and DDL, while its JSON Schema validates the YAML structure rather
than SQL syntax. In another project, substitute an existing spec file or omit
this example when absent.

For the model catalog contract, legacy `spec/agent.yaml` model `cost` values map
to `relative_cost` facts when migrating. This is a relative estimate, not a
currency price. `relative_cost` must be positive or null. API prices and
subscription quota are nonnegative or null: zero means a measured free rate or
zero quota, while null means unknown. In policy patches, null explicitly clears
the field. A `version_pin` must equal the version in the same entry's identity
when both are present; JSON Schema cannot compare those sibling fields, so
`internal/modelcatalog.ValidatePolicyPins` checks the relationship after schema
validation.

Each fact may carry `disputed: true` when its evidence producer identifies a
conflict with another credible observation. The marker belongs to that fact in
the facts array; all observations remain available, and readers must surface an
unresolved conflict rather than select a value by array order. A policy override
is an explicit user choice, not a resolution of the underlying evidence.

Policy patch schema fixtures prove accepted document shapes only. The 941 loader
enforces patch behavior: omitted fields inherit, arrays replace, and null clears.
Policy `fields` currently accepts only `aliases`, `purpose`, and `tier`; schema
and loader reject fields with no merged-catalog behavior. Access observations
carry their own `observed_at` timestamp in addition to the file-level timestamp.
For enablement, a model entry's explicit `enabled` value takes precedence; absent
that, membership in `enabled` decides, then `default_enabled` supplies the
fallback. Snapshots use opaque non-empty IDs. Change report `before` and `after`
refer to snapshot IDs; row `before`/`after` contain field values, and `field` is a
dotted path relative to its model entry, such as `facts.api_cost`.

Policy IDs keep a lowercase harness prefix (`pi`, for example), followed by a
case-sensitive model ID. The model portion may include uppercase letters,
slashes, dots, underscores, and hyphens so exact Pi IDs can be enabled; an
optional final `:low`, `:med`, or `:high` selects a tier. Whitespace and an empty
model ID are invalid. Runtime policy matching preserves model-ID case while
comparing harness names and tiers without case sensitivity.

---

## 3. Architecture & Data Flow

```
┌───────────────────────────┐      ┌─────────────────────────────┐
│  spec/schemas/*.json      │      │  spec/*.yaml                │
│  (JSON Schema definitions)│      │  (YAML Single Source Truth) │
└─────────────┬─────────────┘      └──────────────┬──────────────┘
              │                                   │
              │  Schema Validation (CI/Test)      │  Embedded in binary
              ▼                                   ▼  (//go:embed, include_str!)
┌───────────────────────────┐      ┌─────────────────────────────┐
│  make validate-spec       │      │  Application Runtime / Code │
│  (pre-commit validation)  │      │  (zero hardcoded shadows)   │
└───────────────────────────┘      └──────────────┬──────────────┘
                                                  │
                                                  ▼
                                   ┌─────────────────────────────┐
                                   │  Integrity Test Suite       │
                                   │  (assert all actions wired) │
                                   └─────────────────────────────┘
```

---

## 4. Quality Invariants — "The Spec Compiles"

A specification-driven codebase is considered **clean** when all of the following hold:

1. **No stale actions / identifiers**: Every action, command, or token defined in the spec appears in the schema enum AND has a registered handler in code.
2. **No orphaned definitions**: Every entry declared in the spec is referenced and actively consumed by at least one view, pipeline, or layout component.
3. **No schema drift**: Every property used in YAML is declared in its schema (`additionalProperties: false` recommended where strictness is needed).
4. **No code shadowing**: Loader functions load directly from the embedded spec. They do not maintain hardcoded fallback tables.
5. **Complete labels & strings**: Every label key referenced by layouts or templates exists in the corresponding string/label spec.

---

## 5. Illustrative Pattern

### YAML Definition (`spec/actions.yaml`)
```yaml
# yaml-language-server: $schema=schemas/actions.schema.json
actions:
  refresh:
    title: "Refresh Data"
    description: "Reload all workspace entities from remote"
    keys: ["r", "<f5>"]
    category: "navigation"
  quit:
    title: "Quit Application"
    keys: ["q", "<c-c>"]
    category: "system"
```

### JSON Schema (`spec/schemas/actions.schema.json`)
```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["actions"],
  "properties": {
    "actions": {
      "type": "object",
      "additionalProperties": {
        "type": "object",
        "required": ["title", "keys", "category"],
        "properties": {
          "title": { "type": "string" },
          "description": { "type": "string" },
          "keys": {
            "type": "array",
            "items": { "type": "string" }
          },
          "category": {
            "type": "string",
            "enum": ["navigation", "editing", "system"]
          }
        },
        "additionalProperties": false
      }
    }
  },
  "additionalProperties": false
}
```

---

## 6. Testing Spec Integrity

Integrity tests ensure that code and specifications remain in lockstep.

### Required Test Coverage
- **Schema conformance**: Validate all YAML files against their schemas using a standard validator.
- **Handler completeness**: Assert that every action identifier present in the spec maps to an implemented code branch/handler.
- **No unused spec entries**: Assert that every defined item is referenced in the active configuration or layout.
- **Loader fidelity**: Verify that loaders read directly from the embedded spec without falling back to hidden default maps.
- **Read values, don't copy them**: Tests that need a changeable spec value (a default, a model name) should read it from the embedded spec. A literal copy in a test breaks on every deliberate spec edit.

### Spec Modifications & Agent Test Alignment
- **Spec is Intent**: If a spec file in `spec/*.yaml` has been modified (check `git log` and file modification time), agents must assume the spec reflects deliberate user intent.
- **Update Tests to Match Spec**: When a test fails because it asserted against a former spec default or value, update the test to match the new spec. Do **not** reflexively revert or weaken spec changes.
- **Confirm Before Reverting**: If genuinely ambiguous or uncertain whether a spec edit was intentional, ask the user before reverting any spec modification.

---

## 7. Change Checklist

When modifying spec-driven features:

- [ ] Updated schema (`spec/schemas/*.schema.json`) for any new property or enum value.
- [ ] Updated YAML specification (`spec/*.yaml`).
- [ ] Added or updated code handler/dispatcher for new actions or fields.
- [ ] Validated schema: `make validate-spec` (or language test equivalent).
- [ ] Verified integrity tests pass: `go test ./...` / `cargo test` / `pytest`.
- [ ] Cleaned up obsolete definitions, orphaned actions, and schema enums.

<!-- harnez:stop -->
