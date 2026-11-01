<!-- harnez:variant=lite -->
# Release Pipeline (Lite)

`harnez release` + `goreleaser` + `minisign` + Forgejo (`fj`). Language-agnostic: Go is the
common case, but Python/Zig/Rust and compile-nothing projects release the same way. Full doc
carries the rationale and worked examples: `harnez docs variant gorelease full`.

## 1. Prerequisites — in `$PATH`

`git`, `goreleaser` (`go install github.com/goreleaser/goreleaser/v2@latest`, used even by
non-Go projects), `minisign`, `fj` (forgejo-cli, authenticated to Codeberg),
`harnez` (`ubunatic.com/harnez`), plus `go` for Go projects and to `go install` the tools.

## 2. Project setup checklist

### 2.1 `version.yaml` at repo root = single source of truth

```yaml
# yaml-language-server: $schema=spec/schemas/version.schema.json
version: 0.1.0
```

`harnez release` reads + bumps it, then syncs language-native version files if present:

- Go: `version.go` (`var Version = "0.1.0"`)
- Python: `__version__.py` / `version.py` / `pyproject.toml`
- Zig: `build.zig.zon` / `version.zig`
- Rust: `Cargo.toml`

DO commit `version.yaml` (and add it to `REUSE.toml` if the repo runs `reuse lint`) so the
first automated bump doesn't trip license linting. A project with none of the native files is
valid — sync is then a no-op.

Projects with a breaking-changes table can set `upgrade_doc: docs/Upgrading.md` in
`version.yaml` (default: `docs/Upgrading.md`). Release, including `--dry-run` and
`--continue`, refuses table rows marked `Unreleased`; a missing doc skips the check.
Relabel shipped rows manually: find the change's commit, inspect `git tag --contains
<commit>`, and assign the earliest containing release version. Review ambiguous history.

### 2.2 Cobra root wiring (Go), in `cmd/<binary>/main.go`

```go
root := &cobra.Command{Use: "mytool", Version: Version, Short: "..."}
```

### 2.3 Passwordless minisign key — `-W` is the non-interactive flag

```bash
minisign -G -W -f -p ~/.minisign/<project>.pub -s ~/.minisign/<project>.key
```

Auto-detected as `~/.minisign/<project>.key` by project dir name; else `--sign-key`/`-s <path>`.

### 2.4 `.goreleaser.yaml` (GoReleaser v2)

```yaml
version: 2

project_name: <project>

builds:
  - id: <project>-linux
    main: ./cmd/<project>   # or . if single main.go
    env: [CGO_ENABLED=0]
    goos: [linux]
    goarch: [amd64, arm64]
    binary: <project>
    ldflags:
      - -s -w -X main.Version={{.Version}}

archives:
  - id: default
    formats: [tar.gz]
    name_template: "<project>-{{ .Version }}-{{ if eq .Arch \"amd64\" }}x86_64{{ else if eq .Arch \"arm64\" }}aarch64{{ else }}{{ .Arch }}{{ end }}-linux"
    files: [README.md]
  - id: binary
    formats: [binary]
    name_template: "<project>-{{ if eq .Arch \"amd64\" }}x86_64{{ else if eq .Arch \"arm64\" }}aarch64{{ else }}{{ .Arch }}{{ end }}-linux"

checksum:
  name_template: SHA256SUMS
  algorithm: sha256
```

DON'T add a `signs:` block — dead config. `harnez release` runs GoReleaser with
`--skip=publish --skip=sign` and signs `dist/SHA256SUMS` itself; publishing is owned by `fj`.

### 2.5 `Makefile` target — thin, one line

```makefile
release: check ⚙️  # release the project using harnez
	harnez release
```

Depend on whatever the repo already calls its test/lint target (`check`, `test`, …) — keep its
existing name, don't add an alias. DO delete old hand-rolled
`deps`/`release-build`/`release-publish`/`release-full` targets when adopting this; `harnez
release` owns build, signing, tagging, pushing, publishing. Keep generic helpers like `clean`.

## 3. Forgejo / Codeberg remote verification

`fj release` gets `404 Not Found` when the repo's Releases feature is off. `harnez release`
auto-queries `GET /api/v1/repos/<owner>/<repo>` and auto-enables `has_releases`. Token
resolution (`GetForgeToken`, `internal/release/forge.go`): `$CODEBERG_TOKEN` →
`$FORGEJO_TOKEN`/`$CODEBERG_TOKEN`/`$FJ_TOKEN`/`$GITEA_TOKEN` → fallback read of `fj`'s own
credential store `~/.local/share/forgejo-cli/keys.json`. With no env vars set this rides
entirely on whatever `fj auth login`/`fj auth add-token` last wrote — not a separate credential.

Manual enable:

```bash
curl -X PATCH -H "Authorization: token $CODEBERG_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"has_releases": true}' \
     https://codeberg.org/api/v1/repos/<owner>/<repo>
```

PITFALL — expired token = non-fatal 401, not a broken release. A stale `keys.json` token emits
`[forge] Warning: failed to check 'has_releases' unit: ... status 401 ... token is expired`
mid-run, but the release usually still completes: the tag push is plain `git push` (SSH, own
credentials) and the publish is a separate `fj release` call whose token can still be valid,
since tokens issued at different times expire independently. Confirmed 2026-08-29. Treat the
warning as informational unless the run fails outright — check for
`Release vX.Y.Z completed successfully!` at the end before assuming breakage. `fj auth login`
then re-running clears the warning next release.

## 4. Releasing & recovery

### When to release, which bump

Release only when the user or active `/goal` asks; otherwise recommend the bump and wait. First ensure a clean tree, committed/indexed docs and tickets, and passing `make lint` plus `make test-q1` (`make release` depends on `check`: gofmt, vet, and Go tests). Patch: compatible fixes/docs/refactors; minor: backward-compatible capabilities; major: breaking public APIs. A release pushes and publishes.

```bash
make release                     # or: harnez release   (bumps patch by default)
harnez release --bump patch      # or minor / major / 1.0.0
harnez release --dry-run         # preview execution
harnez release --continue        # resume a failed/interrupted publish
```

DON'T delete or move tags when GoReleaser succeeded or tags were pushed but publishing hit a
network/forge error — `--continue` resumes safely.

`--continue` skips build and signing when `dist/SHA256SUMS`, its `.minisig`, and all
listed archives for this version exist and their hashes verify. Missing files rebuild;
checksum corruption fails. First publish and continue use one resolver: only versioned
archives and packages for the release, `SHA256SUMS`, and its `.minisig` are uploaded.
Bare binaries and artifacts for another version are excluded, including entries in
`dist/artifacts.json`. Before upload, fj lists existing assets and downloads published
`SHA256SUMS`: existing names print `exists, skipped`; differing checksum sets fail for
manual recovery without deleting remote assets. Uploads reporting that a new release is
not visible retry according to `upload_retry_attempts` and `upload_retry_delay` in the
embedded `spec/release.yaml`. If listing fails, Harnez tries every selected upload and
treats `already exists` as success. A nonempty release without `SHA256SUMS` also stops.
Runtime errors suppress usage; auth failures suggest `fj auth login`, followed by
`harnez release --continue`.

### Protected main (`git.protected_main`)

A plain `harnez release` still runs on the default branch, and also pushes any other unpushed
commits there. Harnez does not check who runs it; the forge's branch protection decides whether the
push is accepted. After a rejection, `harnez release --continue` finishes once the push is allowed.

If you cannot push the default branch, release from `release/vX.Y.Z`; the bump commit and tag are
made there and both are pushed. Open the PR and rebase-merge or fast-forward it so the tag stays on
`main`'s history (a merge or squash commit leaves the tag off `main`). Harnez does not tag after
the merge.

To finish a rejected release through a PR, keep its local bump commit and tag: run
`git switch -c release/vX.Y.Z`, `harnez release --continue`, then `git branch -f main origin/main`.

### 4.1 `go.work` and local co-development

Build step forces `GOWORK=off` by default: an enclosing `go.work` silently substitutes an
untagged local checkout for a pinned `go.mod`/`go.sum` dep, so a release could ship built
against uncommitted local source. Pass `--allow-workspace` only when that substitution is
genuinely intended (e.g. cutting a release to validate an in-flight cross-repo change before
either side is tagged).

DO repin after every dependency release, don't let it drift. Outside `harnez release`, plain
`go build`/`go test` in the workspace still use the substitution by design — so an API change
in a local sibling module can build clean in the consumer indefinitely while the consumer's
`go.mod` still pins an older incompatible tag, surfacing only outside the workspace (fresh
clone, CI, `GOWORK=off go build ./...`). Real case: voxi's `audiolevel.Spec` gained a 4th
return value (voxi commit `7b76138`); harnez built fine under `go.work` for 32 commits of
drift. Whenever a sibling's exported API changes: tag + push the sibling first, then
immediately `GOFLAGS=-mod=mod go get <module>@vX.Y.Z` in the consumer — same session, not
optional cleanup.

Automatic drift catch: the `check` target (this project's `Makefile` and
`docs/templates/Makefile` via `harnez init`) runs `go vet`/`go test` with `GOWORK=off` forced —
harmless with no `go.work` active, catches this bug at `make check` time instead of release
time. `check-fast` intentionally does NOT force it, so the inner loop can still test against an
uncommitted sibling checkout.

**Pitfall — `@latest` lags right after a release:** the Go proxy caches `@latest` for a while
after a new tag. Right after a release, `go install <module>@vX.Y.Z` with the exact version.

## 5. Non-Go / scripted projects

Nothing requires a compiler. Same checklist; only the version file and packaging differ.

- **Python version files**: `harnez release` writes `version.yaml`, then syncs if present
  `__version__.py`, `version.py` (`__version__ = "0.1.0"` / `VERSION = ...` / `version = ...`),
  and `pyproject.toml`'s `[project] version`. Single-file scripts with no version constant need
  none — `version.yaml` is enough.
- **Packaging with no build**: GoReleaser OSS has no prebuilt-binary builder, so skip building
  and ship a `meta:` archive. Stage the payload in `before.hooks` OUTSIDE `dist/` (GoReleaser
  empties `dist/` for itself). Add `.goreleaser-src/` to `.gitignore` next to `dist/`.

```yaml
version: 2
project_name: <project>

before:
  hooks:
    - make pack                       # writes dist/<artifact>
    - mkdir -p .goreleaser-src
    - cp dist/<artifact> .goreleaser-src/<project>
    - rm -rf dist

builds:
  - skip: true

archives:
  - id: default
    meta: true                        # pack files without a build
    formats: [tar.gz]
    name_template: "<project>-{{ .Version }}"
    files:
      - src: .goreleaser-src/<project>
        dst: <project>
      - LICENSES/*
      - README.md

checksum:
  name_template: SHA256SUMS
  algorithm: sha256
```

No GoReleaser at all — point at any command that fills `dist/`:

```bash
harnez release --build-cmd "make dist"
```

- **Signing is language-agnostic**: happens outside GoReleaser, after the build step —
  `minisign -S -s <key> -m dist/SHA256SUMS -t "<project> <version>"`. Key MUST be passwordless
  (`-W`, §2.3) because nothing feeds it a password. Resolution order: `--sign-key` →
  `~/.minisign/<project>.key` → repo-local `.minisign.key` / `minisign.key` / `<project>.key` →
  `~/.minisign/minisign.key`. Prefer a dedicated per-project key over the shared fallback;
  rotating from shared to per-project leaves already-published releases verifiable only with
  the old public key — commit the new `minisign.pub` and say so in the commit message.
- **Published assets**: first publish and `--continue` attach the same set: versioned archives
  and packages for the release, `SHA256SUMS`, and `SHA256SUMS.minisig`. Bare binaries and
  artifacts for another version are excluded. A scripted project with a tarball ends up with
  three assets: tarball, `SHA256SUMS`, and `SHA256SUMS.minisig`.
- **Pitfall**: publish still fails when `dist/` is missing or has no matching file, so a
  repo without artifacts cannot be released yet (harnez issue 734).

## 6. Exit criteria — assert link liveness, not link presence

A legal/compliance check that only greps for a string proves the string exists in a source
file, not that the release is safe to announce. `smarthome`'s `make legal-check` verified an
Impressum link and an AGPL source link were *present*, passed clean, and still shipped a public
release whose Privacy Policy link 404s in production — no `website/privacy/` page was ever
generated. False confidence is worse than no check.

RULE: before announcing a release, probe every externally-linked page reachable from the
published site or app (Impressum, privacy policy, source repo, license file) live and require
200 — not a grep. Make it an explicit step in the release/legal-check target, after publish and
before announcing done.

```bash
curl -o /dev/null -sw '%{http_code}' https://example.com/privacy/
```

<!-- harnez:stop -->
