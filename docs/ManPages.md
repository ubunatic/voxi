---
title: Man Pages for Go CLIs
weight: 65
---

<!-- harnez:bundled -->
# Man Pages for Go CLIs

> **Who this is for** — developers creating or maintaining CLI tools in Go who want first-class manual page support across `go install`, pre-built binaries, release tarballs, and system packages.
>
> **Takeaways**
> 1. `go install` only installs the binary into `$GOBIN` — giving the binary a self-installing `<cmd> man --install` subcommand bridges the gap.
> 2. Generate roff man pages directly from Cobra commands (`cobra/doc`) to eliminate doc drift.
> 3. Target XDG `~/.local/share/man/man1` for unprivileged users and `/usr/local/share/man/man1` for root.

---

## 1. Problem: The `go install` Delivery Gap

`go install` compiles and places only the executable binary into `$GOBIN` (or `~/go/bin`). It provides no mechanism to install auxiliary assets like roff man pages, shell completions, or systemd units.

As a result:
- Standalone roff files (e.g. `cmd.1`) checked into the repo frequently drift out of sync with code changes and flags.
- Users installing via `go install` or downloading standalone binaries get working commands but missing `man <cmd>` pages.

---

## 2. Architecture: The 3-Tiered Man Page Pattern

```
┌────────────────────────────────────────────────────────┐
│ Cobra Command Tree (Single Source of Truth)            │
│ github.com/spf13/cobra/doc.GenMan / GenManTree         │
└───────────┬──────────────────┬─────────────────┬───────┘
            │                  │                 │
            ▼                  ▼                 ▼
   ┌─────────────────┐ ┌───────────────┐ ┌───────────────┐
   │ 1. CLI Command  │ │ 2. Makefile   │ │ 3. Packaging  │
   │  `<tool> man`   │ │`make install` │ │ GoReleaser /  │
   │ `<tool> man -i` │ │  `make man`   │ │  NFPM / DEB   │
   └─────────────────┘ └───────────────┘ └───────────────┘
```

By generating the roff representation dynamically from Cobra's command hierarchy, flag definitions and documentation remain strictly in sync with the codebase.

---

## 3. Subcommand Design: `<cmd> man`

Every CLI tool should provide a `man` subcommand with two primary workflows:

### A. Stdout Pipe (Zero-Install Inspection)
Prints the generated roff manual to standard output:
```bash
# View without installing
psync man | man -l -

# Redirect to arbitrary destination
psync man > /tmp/psync.1
```

### B. Self-Installation (`--install`)
Generates the entire man tree (including subcommands) and writes `.1` files directly into the platform's standard manpath:
```bash
psync man --install
man psync
```

### Standard Path Resolution:
- **Unprivileged User**: `$XDG_DATA_HOME/man/man1` (defaults to `~/.local/share/man/man1`). Modern Linux distributions include `~/.local/share/man` in default `man` search paths automatically.
- **Root / Sudo** (`os.Geteuid() == 0`): `/usr/local/share/man/man1`.
- **Custom Override**: `--dir <path>` for custom packaging setups.

---

## 4. Go Implementation Blueprint

Create a dedicated `man.go` file in the root CLI package:

```go
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func newManCmd(rootCmd *cobra.Command) *cobra.Command {
	var (
		install bool
		dir     string
	)
	cmd := &cobra.Command{
		Use:   "man",
		Short: "generate and print or install roff man pages",
		Long: `man generates standard roff/troff man pages.

Without flags, it prints the roff man page to stdout (e.g. tool man | man -l -).
With --install, it writes .1 man pages to ~/.local/share/man/man1 (or /usr/local/share/man/man1 if root).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if install || dir != "" {
				return installManPages(rootCmd, dir, cmd.OutOrStdout())
			}
			header := &doc.GenManHeader{
				Title:   "TOOLNAME",
				Section: "1",
				Source:  "toolname " + Version,
				Manual:  "General Commands Manual",
			}
			return doc.GenMan(rootCmd, header, cmd.OutOrStdout())
		},
	}
	// Note: Avoid shorthand '-i' if the root command has a persistent '-i' (e.g. --interval)
	cmd.Flags().BoolVar(&install, "install", false, "install man pages into standard system or user man directory")
	cmd.Flags().StringVarP(&dir, "dir", "d", "", "custom target directory for man pages (implies --install)")
	return cmd
}

func defaultManDir() string {
	if os.Geteuid() == 0 {
		return "/usr/local/share/man/man1"
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "man", "man1")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".local", "share", "man", "man1")
	}
	return "/usr/local/share/man/man1"
}

func installManPages(rootCmd *cobra.Command, targetDir string, out io.Writer) error {
	if targetDir == "" {
		targetDir = defaultManDir()
	}
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("creating man directory %s: %w", targetDir, err)
	}
	header := &doc.GenManHeader{
		Title:   "TOOLNAME",
		Section: "1",
		Source:  "toolname " + Version,
		Manual:  "General Commands Manual",
	}
	if err := doc.GenManTree(rootCmd, header, targetDir); err != nil {
		return fmt.Errorf("generating man pages in %s: %w", targetDir, err)
	}
	fmt.Fprintf(out, "Installed man pages to %s\nRun 'man toolname' to view.\n", targetDir)
	return nil
}
```

---

## 5. Gotchas & Lessons Learned

### Flag Inheritance Collisions
Cobra propagates `PersistentFlags()` down to all child subcommands. If the root command registers a persistent flag with shorthand `-i` (e.g. `-i, --interval`), registering `-i, --install` on `newManCmd` causes a runtime panic on initialization:
```
panic: unable to redefine 'i' shorthand in "man" flagset: it's already used for "interval" flag
```
**Rule**: Use long-form `--install` without shorthand `-i` on subcommands whenever the root command reserves persistent flags.

### Dependencies in `go.mod`
Importing `github.com/spf13/cobra/doc` introduces transitive dependencies (`github.com/cpuguy83/go-md2man/v2` and `gopkg.in/yaml.v3`). Run `go mod tidy` after wiring `cobra/doc`.

---

## 6. Build & Packaging Integration

### Makefile
Wire man page generation into standard development workflows:

```makefile
man: ⚙️ build  # generate roff man page
	./$(BINARY) man > $(BINARY).1

install: ⚙️ build  # install binary to GOBIN and install man page
	go install .
	./$(BINARY) man --install

clean: ⚙️
	rm -f $(BINARY) $(BINARY).1
```

### `.gitignore`
Ignore generated roff files in the repo root:
```
/*.1
```

### GoReleaser & NFPM Packages
In `.goreleaser.yaml`, ensure man pages are built before packaging or included in distribution artifacts:
```yaml
nfpms:
  - id: packages
    package_name: toolname
    contents:
      - src: dist/man/toolname.1
        dst: /usr/share/man/man1/toolname.1
```
