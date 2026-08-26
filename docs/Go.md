---
title: Go Conventions
weight: 60
---

<!-- harnez:bundled -->
# Go Conventions

> **Who this is for** — anyone writing or reviewing Go in these repositories. Reference material: grep it, don't read it.
>
> **Read this if** — you are about to add a dependency, a global, or a test framework.
>
> **Takeaways**
> 1. Standard library first; every dependency needs a reason and approval (`cobra` for CLI, `yaml.v3` for config).
> 2. No package-level mutable state and no `panic()` outside boot.
> 3. Table-driven tests in the same package, standard `testing` only.

---

## Language & Deps
- **Modern Go**: Leverage current features like `any` and generics, but only where they explicitly reduce boilerplate/noise.
- **Minimise external deps**: Default to the Go Standard Library. If you need an external library, get approval first.
- **Allowed CLI & Config Libraries**: `github.com/spf13/cobra` for CLI tools, `gopkg.in/yaml.v3` for config files.
- **No Heavy Frameworks**: No ORMs (write raw SQL), no logging frameworks (use standard `log`), no Dependency Injection (DI) containers (pass dependencies explicitly).

## Project Layout
- **Small**: Root `main.go` with features split by concern (`apply.go`, `config.go`, `status.go`).
- **Med**: Root `main.go` with `internal/` sub-packages; no `pkg/`.
- **Large/Multi**: Tools in `cmd/<name>/main.go` with features in split files or `internal/`.
- **Embed static assets & specs**: Use `//go:embed`.
- **Never commit binaries**: `go build` drops binaries in the repo root.
  Add it to `.gitignore` at project setup time:
  ```
  # ignore Go binaries
  /mybinary
  ```
  Use the `BINARY` variable from the Makefile as the canonical name so the `.gitignore` entry and the build output always match.

## Spec-Driven Apps
- Avoid hard-coding application configuration, UI labels, controls, text, icons, or layout variables in Go code.
- Define them in YAML specs under `spec/` (e.g. `spec/strings.yaml`, `spec/layout.yaml`, `spec/controls.yaml`) with `$schema` in `spec/schemas/`.
- Embed `spec/` into the Go binary (`//go:embed`); it **IS** part of the code!
- Write compiler/unit test assertions to verify Go structs match specs.

## CLI
- Use Cobra; one `*cobra.Command` per verb, flags defined on that command.
- Use `RunE` instead of `Run` — return errors, don't `os.Exit` inside commands.
- Set `SilenceUsage: true` on commands where error is not a usage mistake.

## State Management
- **No package-level mutable variables**: Pass state explicitly via function arguments or state structs (e.g. `type App struct { client *http.Client }`). Mutable global state creates hidden coupling and breaks tests.
- **Safe `init()` usage**: Use `init()` only for static, side-effect-free registrations (e.g. `flag.Var`). Never connect to databases, services, or load files during initialization.

## Error Handling
- **Wrap errors with context**: Use `fmt.Errorf("module: %w", err)` to preserve the original error while giving readable context.
- **No panic**: Never use `panic()` except for truly unrecoverable boot-time configurations.
- **Return errors**: Let errors bubble up to `main()` where they are printed and exited.

## Types & Style
- Unexported types for internal results; exported only when crossing package boundary.
- Pointer fields (`*bool`, `*int`) for optional struct values; add `boolPtr`/`intPtr` helpers.
- Section banners: `// ── Section name ────────────────────────────────────────────`
- Doc comments on all exported symbols.

## Output Discipline
- Functions return values; callers own printing.
- Indent output messages for visual hierarchy:
  - Print changed items with two spaces: `  wrote README.md`
  - Print sub-details with four spaces: `    size: 1.2 KB`

## Unit Tests
- Use standard Go table-driven tests (`t.Run`).
- Put tests in the same package (e.g. `package main`) to test unexported components easily.
- Helpers: `t.Helper()`, `t.Fatalf` for setup failures, `t.Errorf` for assertion failures.
- Standard library `testing` only — no external testing frameworks or mock generators.
