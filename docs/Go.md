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
- **Allowed & Recommended Libraries**: `github.com/spf13/cobra` for CLI tools, `gopkg.in/yaml.v3` for YAML config/spec files, and `github.com/google/jsonschema-go/jsonschema` for JSON Schema validation and schema-aware tooling. Use the latter with `yaml.v3` when validating YAML documents against JSON Schemas.
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

## Workspace Isolation

- Go automatically applies the nearest enclosing `go.work` to every descendant directory. A
  persistent workspace for sibling-module development can therefore break an unrelated nested
  repository whose modules are absent from that workspace.
- `harnez init` probes `go env GOWORK` and the active workspace's parsed module list. When an
  enclosing workspace omits any Go module found in the target repository, init creates a minimal
  project-local `go.work` that uses all of the repository's modules. Existing local workspaces are
  always preserved, and init creates nothing when there is no module or no demonstrated hazard.
- A project-local workspace is the normal isolation boundary. To deliberately use a different
  cross-repository workspace for one command, select it explicitly, for example
  `GOWORK=/path/to/go.work go test ./...`. Release builds still use their separate
  `GOWORK=off` policy unless `harnez release --allow-workspace` is passed.
- When a module has a local `replace` pointing at a sibling module, include both modules in the
  project-local workspace (for example, `use ( ./ ../loom )`). A workspace does not make a
  relative replacement path portable: copied projects still need the sibling directory, or a
  published module version. Keep the `replace` in `go.mod` when `GOWORK=off` builds still need the
  local fallback.
- After copying a project, validate every relative replacement with `go list ./...`. A missing
  sibling replacement can make `gopls` repeatedly diagnose the module and make editor saves
  appear to hang while the workspace is loading.

## Spec-Driven Apps
- Avoid hard-coding application configuration, UI labels, controls, text, icons, or layout variables in Go code.
- Define them in YAML specs under `spec/` (e.g. `spec/strings.yaml`, `spec/layout.yaml`, `spec/controls.yaml`) with `$schema` in `spec/schemas/`.
- Embed `spec/` into the Go binary (`//go:embed`); it **IS** part of the code!
- Write compiler/unit test assertions to verify Go structs match specs.

## CLI & Releases
- Use Cobra; one `*cobra.Command` per verb, flags defined on that command.
- Use `RunE` instead of `Run` — return errors, don't `os.Exit` inside commands.
- Set `SilenceUsage: true` on commands where error is not a usage mistake.
- **Version Wiring**: Keep `var Version = "..."` in `version.go` (synced automatically by `harnez release` from `version.yaml`) and wire `rootCmd.Version = Version`.
- **Man Pages**: Provide `<tool> man` (stdout roff) and `<tool> man --install` (writes `.1` files to `~/.local/share/man/man1` or `/usr/local/share/man/man1`) via `cobra/doc` so `go install` users get man pages. Only read `@docs/ManPages.md` when first implementing or troubleshooting man page setup.
- **Releases**: Provide a thin `release: check ⚙️` recipe that delegates to `harnez release`. See `@docs/GoRelease.md`.

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

## Strings, Runes & Terminal Width
- Never index or slice a string by byte offset (`s[i:j]`) for human-readable text or aligned output.
- Never use `len(s)` as a visual column count: it measures bytes, not terminal cells.
- Use `[]rune(s)` for character-count logic and `runewidth.StringWidth` for terminal width.
- Strip ANSI escapes before measuring width; test rendered cell width, not rune count.
- The existing `internal/usage` width assertions are the canonical pattern for TUI output.
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
