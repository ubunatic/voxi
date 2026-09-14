package main

import (
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	"github.com/spf13/pflag"
	"ubunatic.com/voxi"
)

// newManCmd builds the `voxi man` command: print the generated roff manual
// to stdout, install the full man tree with --install, or render the whole
// tree as one static HTML page with --html (see docs/ManPages.md for the
// roff/install pattern this follows; --html is voxi's own extension, used to
// publish an always-in-sync manual subpage on the project website instead of
// a hand-maintained HTML summary).
func newManCmd(rootCmd *cobra.Command) *cobra.Command {
	var (
		install  bool
		dir      string
		htmlMode bool
	)
	cmd := &cobra.Command{
		Use:   "man",
		Short: "generate and print or install roff man pages",
		Long: "man generates standard roff/troff man pages from voxi's command tree.\n\n" +
			"Without flags, it prints the roff man page to stdout (e.g. voxi man | man -l -).\n" +
			"With --install, it writes .1 man pages to ~/.local/share/man/man1\n" +
			"(or /usr/local/share/man/man1 if root).\n" +
			"With --html, it prints one static HTML page covering every command to stdout.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if htmlMode {
				return writeHTMLManual(rootCmd, cmd.OutOrStdout())
			}
			if install || dir != "" {
				return installManPages(rootCmd, dir, cmd.OutOrStdout())
			}
			return doc.GenMan(rootCmd, manHeader(), cmd.OutOrStdout())
		},
	}
	// Long-form only: monitor's -i/--interval flag makes -i ambiguous at the
	// root level if this ever gained a persistent sibling flag (see
	// docs/ManPages.md §5, Flag Inheritance Collisions).
	cmd.Flags().BoolVar(&install, "install", false, "install man pages into standard system or user man directory")
	cmd.Flags().StringVarP(&dir, "dir", "d", "", "custom target directory for man pages (implies --install)")
	cmd.Flags().BoolVar(&htmlMode, "html", false, "print one static HTML page covering every command to stdout")
	return cmd
}

func manHeader() *doc.GenManHeader {
	return &doc.GenManHeader{
		Title:   "VOXI",
		Section: "1",
		Source:  "voxi " + voxi.Version,
		Manual:  "General Commands Manual",
	}
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
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("creating man directory %s: %w", targetDir, err)
	}
	if err := doc.GenManTree(rootCmd, manHeader(), targetDir); err != nil {
		return fmt.Errorf("generating man pages in %s: %w", targetDir, err)
	}
	fmt.Fprintf(out, "Installed man pages to %s\nRun 'man voxi' to view.\n", targetDir)
	return nil
}

// slug turns a command's full path ("voxi config import") into the anchor
// and roff-filename-style id cobra/doc's GenManTree already uses
// ("voxi-config-import"), so cross-links match `man voxi-config-import`.
func slug(cmd *cobra.Command) string {
	return strings.ReplaceAll(cmd.CommandPath(), " ", "-")
}

// availableSubcommands mirrors cobra/doc's own traversal filter (see
// GenManTreeFromOpts) so the HTML manual covers exactly the same commands as
// the roff tree -- no hidden commands, no bare help topics.
func availableSubcommands(cmd *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() || c.IsAdditionalHelpTopicCommand() {
			continue
		}
		out = append(out, c)
	}
	return out
}

// writeHTMLManual renders root and every available descendant as one static
// HTML page, generated straight from the live Cobra tree so it can never
// drift the way a hand-maintained website summary can (see man --html and
// docs/ManPages.md).
func writeHTMLManual(root *cobra.Command, out io.Writer) error {
	fmt.Fprint(out, htmlManualHead)
	fmt.Fprintln(out, `      <nav class="manual-toc" aria-label="Command index">`)
	fmt.Fprintln(out, `        <h2>Commands</h2>`)
	writeTOC(out, root)
	fmt.Fprintln(out, `      </nav>`)
	fmt.Fprintln(out, `      <div class="manual-entries">`)
	writeEntries(out, root)
	fmt.Fprintln(out, `      </div>`)
	fmt.Fprint(out, htmlManualFoot)
	return nil
}

func writeTOC(out io.Writer, cmd *cobra.Command) {
	kids := availableSubcommands(cmd)
	fmt.Fprintf(out, "<ul><li><a href=\"#%s\"><code>%s</code></a>", slug(cmd), html.EscapeString(cmd.CommandPath()))
	if len(kids) > 0 {
		for _, k := range kids {
			writeTOC(out, k)
		}
	}
	fmt.Fprint(out, "</li></ul>")
}

func writeEntries(out io.Writer, cmd *cobra.Command) {
	fmt.Fprintf(out, "<article class=\"manpage manual-entry\" id=\"%s\">\n", slug(cmd))
	fmt.Fprintf(out, "  <div class=\"mp-bar\"><span>%s</span><span>voxi %s</span></div>\n",
		html.EscapeString(strings.ToUpper(slug(cmd))), html.EscapeString(voxi.Version))
	fmt.Fprintf(out, "  <h3 class=\"mp-sh\"><code>%s</code></h3>\n", html.EscapeString(cmd.UseLine()))
	if cmd.Short != "" {
		fmt.Fprintf(out, "  <p class=\"mp-body\">%s</p>\n", html.EscapeString(cmd.Short))
	}
	if cmd.Long != "" && cmd.Long != cmd.Short {
		fmt.Fprintf(out, "  <p class=\"mp-body\">%s</p>\n",
			strings.ReplaceAll(html.EscapeString(cmd.Long), "\n", "<br>"))
	}

	var flagLines []string
	cmd.NonInheritedFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		name := "--" + f.Name
		if f.Shorthand != "" {
			name = "-" + f.Shorthand + ", " + name
		}
		flagLines = append(flagLines, fmt.Sprintf("  <dt><code>%s</code></dt><dd>%s%s</dd>",
			html.EscapeString(name), html.EscapeString(f.Usage), defaultSuffix(f)))
	})
	if len(flagLines) > 0 {
		fmt.Fprintln(out, "  <dl class=\"mp-opts\">")
		for _, l := range flagLines {
			fmt.Fprintln(out, l)
		}
		fmt.Fprintln(out, "  </dl>")
	}
	fmt.Fprintln(out, "</article>")

	for _, k := range availableSubcommands(cmd) {
		writeEntries(out, k)
	}
}

func defaultSuffix(f *pflag.Flag) string {
	if f.DefValue == "" || f.DefValue == "false" || f.DefValue == "[]" {
		return ""
	}
	return fmt.Sprintf(" (default: <code>%s</code>)", html.EscapeString(f.DefValue))
}

const htmlManualHead = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>voxi &mdash; Full Command Reference</title>
  <meta name="description" content="Generated, always-in-sync voxi command reference, produced directly from the CLI's own Cobra command tree.">

  <link rel="icon" type="image/svg+xml" href="../logo.svg">
  <link rel="stylesheet" href="../index.css">
  <script src="../index.js" defer></script>
  <style>
    .manual-toc ul { list-style: none; margin: 0; padding-left: 1.1rem; }
    .manual-toc > ul { padding-left: 0; }
    .manual-toc a { font-family: var(--mono); font-size: 0.86rem; }
    .manual-entry { margin-bottom: 1.75rem; }
  </style>
</head>
<body>
  <header class="site-head">
    <nav class="wrap nav" aria-label="Main navigation">
      <a class="brand" href="../index.html#top">
        <img class="brand-icon" src="../logo.svg" alt="voxi logo" width="28" height="28">
        <span>voxi</span>
      </a>
      <div class="nav-links">
        <a href="../index.html#top">Home</a>
        <a href="../dev/index.html">Developers</a>
        <a class="btn btn-outline" href="https://codeberg.org/ubunatic/voxi" rel="noopener" target="_blank">Codeberg</a>
      </div>
    </nav>
  </header>

  <main id="top">
    <div class="wrap hero-shell">
      <section class="hero-intro" aria-label="voxi full command reference overview">
        <h1><span class="name">voxi</span> full command reference.</h1>
        <p class="lede">
          Generated directly from voxi's own Cobra command tree (<code>voxi man --html</code>)
          &mdash; the same source that produces <code>voxi man --install</code>'s roff pages, so
          this page can't drift from the real CLI the way a hand-written summary can. For a
          curated tour of the everyday commands, see the <a href="../index.html#manual">main
          page's Command-Line Reference</a> instead.
        </p>
      </section>
    </div>

    <section class="wrap section-wrap" aria-label="Full command reference">
`

const htmlManualFoot = `    </section>
  </main>

  <footer class="site-foot">
    <div class="wrap foot-row">
      <div class="foot-brand">
        <img class="brand-icon" src="../logo.svg" alt="voxi logo" width="20" height="20">
        <span>voxi &mdash; &copy; <span id="year"></span> Uwe Jugel &lt;uwe@ubunatic.com&gt; &middot; AGPL-3.0-or-later</span>
      </div>
      <div class="foot-links">
        <a href="https://codeberg.org/ubunatic/voxi" rel="noopener" target="_blank">Codeberg Repository</a>
        <a href="../index.html#top">Back to voxi &uarr;</a>
      </div>
    </div>
  </footer>
</body>
</html>
`
