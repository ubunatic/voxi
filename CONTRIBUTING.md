# Contributing to Voxi

Thank you for your interest in contributing to `voxi`! This document outlines the development workflow, coding standards, and repository conventions.

---

## 1. Development Workflow & Git Conventions

- **Branching Strategy**: Work on the repository's default branch (`main`).
- **Commit Messages**: Follow the [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) specification:
  - `feat(scope)`: New feature or capability
  - `fix(scope)`: Bug fix
  - `docs(scope)`: Documentation changes
  - `refactor(scope)`: Code refactoring without behavioral change
  - `test(scope)`: Adding or updating tests
  - `chore(scope)`: Build, packaging, or repository maintenance
- **Issue & Feature Process**:
  - We do not use web issue trackers for general triage.
  - Proposals, bug reports, and architectural changes are managed as Markdown files under `issues/` (e.g. `issues/NNN-topic-name.md`) and submitted via Pull/Merge Requests.
- **Hygiene**:
  - Never commit secrets, credentials, or personal data.
  - Keep test audio files and binaries out of git tracking (handled by `.gitignore`).
  - Always ensure `make test` and `make check` pass cleanly before committing.

---

## 2. Go Standards & Architecture

- **Go Version & Style**:
  - Write modern, idiomatic Go.
  - Default to the Go Standard Library. External dependencies require explicit rationale.
  - Standard allowed dependencies: `github.com/spf13/cobra` (CLI) and `gopkg.in/yaml.v3` (YAML parsing).
- **Error Handling & State**:
  - Do not use `panic()` outside unrecoverable startup initialization.
  - Return explicit errors wrapped with context: `fmt.Errorf("context: %w", err)`.
  - Avoid package-level mutable state; pass dependencies and state explicitly via structs and constructors.
- **Testing**:
  - Use standard library `testing` package with table-driven tests (`t.Run(...)`).
  - Keep tests deterministic, fast, and hermetic.

---

## 3. Spec-Driven System

- Configuration, model registries, and constants live in YAML specification files under `spec/` (e.g., `spec/models.yaml`).
- Schemas are validated with JSON schemas located in `spec/schemas/`.
- Specifications are embedded directly into the Go binary via `//go:embed`.
- Do not duplicate spec values in Go code; always parse and load from embedded specs.
- Verify spec integrity with `make validate-spec`.

---

## 4. Makefile Commands

Common development targets:

```bash
make help                 # Show available make targets
make build                # Build the voxi binary
make build-modifierd      # Build the voxi-modifierd daemon
make test                 # Run go vet and go test across all packages
make validate-spec        # Validate spec/*.yaml against embedded JSON schemas
make check                # Run full test suite and spec validations
make install              # Install voxi to ~/go/bin
make install-user-services# Install systemd user service units
make format               # Format Go source code (go fmt ./...)
make clean                # Remove build artifacts
```

---

## 5. Security & Hotkey Safety Guidelines

When modifying input injection or evdev handling:
- Maintain physical modifier release gating (`voxi-modifierd`) to prevent accidental hotkey combinations when typing begins.
- Never log, buffer, or persist non-modifier keystrokes.
- Keep dictation history local to `~/.local/share/voxi/history.jsonl` with restricted file permissions (`0600`).
