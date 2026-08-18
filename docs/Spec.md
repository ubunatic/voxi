---
title: Spec System Reference
weight: 30
---

# Spec-Driven Architecture — Authoritative Reference

The `spec/` directory is **application code**, not runtime user configuration. Treat spec files with the same rigour as source code: every change must be intentional, every object must be consumed, and specs must always be validated against formal schemas.

---

## 1. Core Principles

> **Spec files are the single source of truth. Application code must not duplicate or shadow spec values.**

### Standard Format: YAML + JSON Schema
- **YAML for authoring**: YAML provides concise, comment-friendly, and human/LLM-readable specification definitions without syntax noise or string escaping friction. Plain JSON files are **not** promoted for authoring due to lack of comments and strict syntax friction.
- **JSON Schema for validation**: Every YAML file must have a companion JSON Schema (`spec/schemas/*.schema.json`) for automated IDE validation, editor auto-completion, and CI/pre-commit verification.
- **Embedded application code**: In compiled languages, embed the `spec/` directory directly into the binary (e.g. Go `//go:embed`, Rust `include_str!`). The spec is part of the application artifact, not external config.

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

---

## 7. Change Checklist

When modifying spec-driven features:

- [ ] Updated schema (`spec/schemas/*.schema.json`) for any new property or enum value.
- [ ] Updated YAML specification (`spec/*.yaml`).
- [ ] Added or updated code handler/dispatcher for new actions or fields.
- [ ] Validated schema: `make validate-spec` (or language test equivalent).
- [ ] Verified integrity tests pass: `go test ./...` / `cargo test` / `pytest`.
- [ ] Cleaned up obsolete definitions, orphaned actions, and schema enums.
