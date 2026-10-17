// Command gotest-report turns `go test -json` output into a GitHub job
// summary: failing tests with their output tail (also as ::error annotations,
// readable with `gh run view` when raw logs are not), the slowest packages and
// the test time. With -flaky FILE it lists tests that both passed and failed
// across -count runs, one "pkg Test" per line, for the flake workflow.
//
// Shared verbatim by harnez, loom, cati and voxi CI; stdlib only. Run it as
// `go run .github/gotest-report.go [flags] test.json...`. The exit status is 0
// whether or not tests failed; the go test step carries the failure.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"
)

type event struct {
	Time       time.Time
	Action     string
	Package    string
	Test       string
	Output     string
	Elapsed    float64
	ImportPath string
}

type report struct {
	output      map[string][]string // "pkg Test" or "pkg" -> output lines
	pass, fail  map[string]int      // per test or package, counted across -count runs
	failOrder   []string
	pkgElapsed  map[string]float64
	first, last time.Time
	other       []string // non-JSON lines (gofmt, vet, build errors)
}

func key(pkg, test string) string {
	if test == "" {
		return pkg
	}
	return pkg + " " + test
}

func (r *report) read(in io.Reader) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	for sc.Scan() {
		line := sc.Text()
		var e event
		if !strings.HasPrefix(line, "{") || json.Unmarshal([]byte(line), &e) != nil || e.Action == "" {
			r.other = append(r.other, line)
			continue
		}
		if !e.Time.IsZero() {
			if r.first.IsZero() || e.Time.Before(r.first) {
				r.first = e.Time
			}
			if e.Time.After(r.last) {
				r.last = e.Time
			}
		}
		k := key(e.Package, e.Test)
		switch e.Action {
		case "output":
			r.output[k] = append(r.output[k], strings.TrimRight(e.Output, "\n"))
		case "build-output":
			pkg, _, _ := strings.Cut(e.ImportPath, " [")
			r.output[pkg] = append(r.output[pkg], strings.TrimRight(e.Output, "\n"))
		case "pass":
			r.pass[k]++
		case "fail":
			if r.fail[k] == 0 {
				r.failOrder = append(r.failOrder, k)
			}
			r.fail[k]++
		}
		if e.Test == "" && (e.Action == "pass" || e.Action == "fail") {
			r.pkgElapsed[e.Package] += e.Elapsed
		}
	}
	return sc.Err()
}

// failures lists failing tests; a package is listed only when none of its
// tests failed (build errors, TestMain, panics outside tests).
func (r *report) failures() []string {
	withTests := map[string]bool{}
	for _, k := range r.failOrder {
		if pkg, _, ok := strings.Cut(k, " "); ok {
			withTests[pkg] = true
		}
	}
	var out []string
	for _, k := range r.failOrder {
		if strings.Contains(k, " ") || !withTests[k] {
			out = append(out, k)
		}
	}
	return out
}

// flaky lists tests that passed in some runs and failed in others. Subtests
// are left out when their parent is listed.
func (r *report) flaky() []string {
	var out []string
	for _, k := range r.failOrder {
		if !strings.Contains(k, " ") || r.pass[k] == 0 {
			continue
		}
		if i := strings.LastIndex(k, "/"); i > 0 && r.pass[k[:i]] > 0 && r.fail[k[:i]] > 0 {
			continue
		}
		out = append(out, k)
	}
	return out
}

func tail(lines []string, n int) []string {
	return lines[max(0, len(lines)-n):]
}

func (r *report) markdown(w io.Writer, title string, slowest, n int) {
	fmt.Fprintf(w, "### %s\n\n", title)
	if !r.first.IsZero() {
		fmt.Fprintf(w, "Test time: %s\n\n", r.last.Sub(r.first).Round(time.Second))
	}
	if fails := r.failures(); len(fails) > 0 {
		fmt.Fprintf(w, "**%d failing**\n\n", len(fails))
		for _, k := range fails {
			fmt.Fprintf(w, "<details><summary><code>%s</code> (failed %d, passed %d)</summary>\n\n```\n%s\n```\n</details>\n\n",
				k, r.fail[k], r.pass[k], strings.Join(tail(r.output[k], n), "\n"))
		}
	} else if len(r.other) > 0 && len(r.pass) == 0 {
		fmt.Fprintf(w, "No test events; output tail:\n\n```\n%s\n```\n\n", strings.Join(tail(r.other, n), "\n"))
	}
	if flaky := r.flaky(); len(flaky) > 0 {
		fmt.Fprintf(w, "**%d flaky** (passed and failed across runs):\n\n", len(flaky))
		for _, k := range flaky {
			fmt.Fprintf(w, "- `%s` failed %d of %d\n", k, r.fail[k], r.fail[k]+r.pass[k])
		}
		fmt.Fprintln(w)
	}
	pkgs := make([]string, 0, len(r.pkgElapsed))
	for p := range r.pkgElapsed {
		pkgs = append(pkgs, p)
	}
	slices.SortFunc(pkgs, func(a, b string) int {
		if d := r.pkgElapsed[b] - r.pkgElapsed[a]; d != 0 {
			if d > 0 {
				return 1
			}
			return -1
		}
		return strings.Compare(a, b)
	})
	if len(pkgs) > 0 {
		fmt.Fprintf(w, "<details><summary>Slowest %d of %d packages</summary>\n\n| package | seconds |\n|---|---:|\n", min(slowest, len(pkgs)), len(pkgs))
		for _, p := range pkgs[:min(slowest, len(pkgs))] {
			fmt.Fprintf(w, "| `%s` | %.1f |\n", p, r.pkgElapsed[p])
		}
		fmt.Fprint(w, "\n</details>\n\n")
	}
}

func main() {
	title := flag.String("title", "go test", "summary heading")
	slowest := flag.Int("slowest", 10, "number of slowest packages to list")
	n := flag.Int("tail", 40, "output lines per failure")
	flakyOut := flag.String("flaky", "", "write flaky tests to this file, one per line")
	flag.Parse()
	r := &report{output: map[string][]string{}, pass: map[string]int{}, fail: map[string]int{}, pkgElapsed: map[string]float64{}}
	for _, name := range flag.Args() {
		f, err := os.Open(name)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue // a missing file means its step did not run
		}
		if err := r.read(f); err != nil {
			fmt.Fprintln(os.Stderr, name, err)
		}
		f.Close()
	}
	out := io.Writer(os.Stdout)
	if path := os.Getenv("GITHUB_STEP_SUMMARY"); path != "" {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			defer f.Close()
			out = f
		}
	}
	r.markdown(out, *title, *slowest, *n)
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		for _, k := range r.failures() {
			msg := strings.Join(tail(r.output[k], 10), "%0A")
			fmt.Printf("::error title=%s::%s\n", k, msg)
		}
	}
	if *flakyOut != "" {
		if err := os.WriteFile(*flakyOut, []byte(strings.Join(r.flaky(), "\n")), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
