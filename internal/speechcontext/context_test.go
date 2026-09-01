package speechcontext

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func testOptions() Options {
	return Options{Enabled: true, PromptPrefix: "Terms:", MaxTerms: 5, MaxChars: 100, MaxTermChars: 20}
}

func TestBuildPriorityCapAndDeduplication(t *testing.T) {
	opts := testOptions()
	opts.MaxTerms = 3
	got := Build(opts, Sources{
		Explicit:   []string{"Voxi", "dotool"},
		Static:     []string{"voxi", "PipeWire"},
		Repository: []string{"models.yaml"},
	})
	if want := "Terms: Voxi, dotool, PipeWire"; got != want {
		t.Fatalf("Build() = %q, want %q", got, want)
	}
}

func TestBuildSanitizesPathsControlsAndOverlongTerms(t *testing.T) {
	got := Build(testOptions(), Sources{Explicit: []string{
		"/home/private/project/models.yaml",
		"Pipe\x00Wire",
		"C++",
		strings.Repeat("x", 21),
	}})
	if want := "Terms: models.yaml, Pipe Wire, C++"; got != want {
		t.Fatalf("Build() = %q, want %q", got, want)
	}
}

func TestBuildHonorsCharacterBudget(t *testing.T) {
	opts := testOptions()
	opts.MaxChars = utf8.RuneCountInString("Terms: Voxi, dotool")
	got := Build(opts, Sources{Explicit: []string{"Voxi", "dotool", "PipeWire"}})
	if want := "Terms: Voxi, dotool"; got != want {
		t.Fatalf("Build() = %q, want %q", got, want)
	}
	if utf8.RuneCountInString(got) > opts.MaxChars {
		t.Fatalf("prompt length %d exceeds cap %d", utf8.RuneCountInString(got), opts.MaxChars)
	}
}

func TestBuildDisabledOrEmpty(t *testing.T) {
	opts := testOptions()
	opts.Enabled = false
	if got := Build(opts, Sources{Static: []string{"Voxi"}}); got != "" {
		t.Fatalf("disabled Build() = %q, want empty", got)
	}
	opts.Enabled = true
	if got := Build(opts, Sources{}); got != "" {
		t.Fatalf("empty Build() = %q, want empty", got)
	}
}

func TestParseVocabulary(t *testing.T) {
	got := ParseVocabulary([]byte(" Voxi \n\nC#\r\ndotool\n"))
	want := []string{"Voxi", "C#", "dotool"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("ParseVocabulary() = %#v, want %#v", got, want)
	}
}

func TestVocabularyPath(t *testing.T) {
	if got := VocabularyPath("/home/test"); got != "/home/test/.config/voxi/vocabulary.txt" {
		t.Fatalf("VocabularyPath() = %q", got)
	}
	if got := VocabularyPath(""); got != "" {
		t.Fatalf("VocabularyPath(empty) = %q", got)
	}
}

func TestDiscoverRepositoryTermsUsesTrackedBasenamesByMtime(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	nested := filepath.Join(root, "internal")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	older := filepath.Join(nested, "older.go")
	newer := filepath.Join(root, "models.yaml")
	untracked := filepath.Join(root, "private-secret.txt")
	for _, path := range []string{older, newer, untracked} {
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := exec.Command("git", "-C", root, "init", "-q").Run(); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", root, "add", "internal/older.go", "models.yaml").Run(); err != nil {
		t.Fatal(err)
	}
	baseTime := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(older, baseTime, baseTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newer, baseTime.Add(time.Hour), baseTime.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	got := DiscoverRepositoryTerms(context.Background(), nested, 2)
	want := []string{filepath.Base(root), "models.yaml", "older.go"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("DiscoverRepositoryTerms() = %#v, want %#v", got, want)
	}
}
