package feedback

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ubunatic.com/voxi/internal/deps"
)

func TestApplyReplacements(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		rules []Replacement
		want  string
	}{
		{"word and punctuation", "voxy, then voxy!", []Replacement{{From: "voxy", To: "voxi"}}, "voxi, then voxi!"},
		{"embedded word rejected", "Voxyology myVoxy Voxy_2", []Replacement{{From: "Voxy", To: "voxi"}}, "Voxyology myVoxy Voxy_2"},
		{"adapts to heard case", "boxie Boxie BOXIE", []Replacement{{From: "boxie", To: "voxi"}}, "voxi Voxi VOXI"},
		{"unicode boundaries", "überVoxy Voxy—Voxy Voxy猫", []Replacement{{From: "Voxy", To: "voxi"}}, "überVoxy Voxi—Voxi Voxy猫"},
		{"phrase mixed case falls back to stored casing", "Voxy\tproject and Voxy\nproject", []Replacement{{From: "Voxy project", To: "voxi project"}}, "voxi project and voxi project"},
		{"all aliases, title case heard", "Voxy and Voxie", []Replacement{{From: "Voxy", To: "voxi"}, {From: "Voxie", To: "voxi"}}, "Voxi and Voxi"},
		{"longest overlap, mixed case falls back", "Voxy project", []Replacement{{From: "Voxy", To: "wrong"}, {From: "Voxy project", To: "voxi project"}}, "voxi project"},
		{"non cascading, second match re-cased on its own heard form", "Voxy Voxi", []Replacement{{From: "Voxy", To: "Voxi"}, {From: "Voxi", To: "changed"}}, "Voxi Changed"},
		{"title case with trailing symbol", "Voxy", []Replacement{{From: "Voxy", To: "voxi™"}}, "Voxi™"},
		{"unclassifiable heard case falls back to stored casing", "VoXy", []Replacement{{From: "voxy", To: "Voxi"}}, "Voxi"},
		{"fixed case ignores heard casing", "Ubonatic Dot Com and ubonatic dot com", []Replacement{{From: "ubonatic dot com", To: "ubunatic.com", FixedCase: true}}, "ubunatic.com and ubunatic.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ApplyReplacements(tt.text, tt.rules); got != tt.want {
				t.Fatalf("ApplyReplacements() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReplacementPersistenceValidationAndCommands(t *testing.T) {
	home := t.TempDir()
	path := ReplacementPath(home)
	var out bytes.Buffer
	run := func(args ...string) error {
		cmd := NewCommand(&out, home, rules, 64, nil, deps.Dependencies{}, nil)
		cmd.SetArgs(args)
		return cmd.Execute()
	}
	if err := run("replacement", "add", " Voxy   project ", "voxi project"); err != nil {
		t.Fatal(err)
	}
	if err := run("replacement", "add", "Voxy", "voxi"); err != nil {
		t.Fatal(err)
	}
	if err := run("replacement", "add", "Voxy", "other"); err == nil || !strings.Contains(err.Error(), "already mapped") {
		t.Fatalf("duplicate error = %v", err)
	}
	if err := run("replacement", "add", "VOXY", "other"); err == nil || !strings.Contains(err.Error(), "already mapped") {
		t.Fatalf("case-insensitive duplicate error = %v", err)
	}
	if err := run("replacement", "add", "ubonatic dot com", "ubunatic.com", "--fixed-case"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("mode = %o, want 600", got)
	}
	out.Reset()
	if err := run("replacement", "list"); err != nil {
		t.Fatal(err)
	}
	want := "Voxy\tvoxi\nVoxy project\tvoxi project\nubonatic dot com\tubunatic.com\t[fixed-case]\n"
	if got := out.String(); got != want {
		t.Fatalf("list = %q, want %q", got, want)
	}
	if err := run("replacement", "remove", "voxy"); err != nil {
		t.Fatalf("case-insensitive remove error = %v", err)
	}
	loaded, err := LoadReplacements(path)
	if err != nil || len(loaded) != 2 {
		t.Fatalf("loaded = %#v, err = %v", loaded, err)
	}
	if _, _, err := AddReplacement(nil, "", "x", false); err == nil {
		t.Error("empty source accepted")
	}
	if _, _, err := AddReplacement(nil, "x", " ", false); err == nil {
		t.Error("empty target accepted")
	}
	if _, _, err := AddReplacement(nil, "x\n", "y", false); err != nil {
		t.Errorf("ordinary source whitespace should normalize: %v", err)
	}
	if _, _, err := AddReplacement(nil, "x", "y\x00", false); err == nil {
		t.Error("control target accepted")
	}

	bad := filepath.Join(t.TempDir(), "replacements.json")
	if err := os.WriteFile(bad, []byte(`[{"from":"","to":"x"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadReplacements(bad); err == nil {
		t.Error("invalid stored rule accepted")
	}
}

func TestDedupeReplacements(t *testing.T) {
	kept, dropped := DedupeReplacements([]Replacement{
		{From: "Voxy", To: "voxi"},
		{From: "voxy", To: "voxi"},
		{From: "VOXY", To: "voxi2"},
		{From: "Codeberg", To: "Codeberg"},
	})
	if len(kept) != 2 || kept[0].From != "Codeberg" || kept[1].From != "Voxy" {
		t.Fatalf("kept = %#v", kept)
	}
	if len(dropped) != 2 {
		t.Fatalf("dropped = %#v", dropped)
	}
	if dropped[0].Dropped.From != "VOXY" || !dropped[0].Conflict {
		t.Errorf("dropped[0] = %#v, want conflicting VOXY", dropped[0])
	}
	if dropped[1].Dropped.From != "voxy" || dropped[1].Conflict {
		t.Errorf("dropped[1] = %#v, want benign voxy", dropped[1])
	}
	if kept2, dropped2 := DedupeReplacements(nil); len(kept2) != 0 || len(dropped2) != 0 {
		t.Errorf("empty input produced kept=%#v dropped=%#v", kept2, dropped2)
	}

	// Pure case variants that agree on the target modulo casing are now
	// redundant under adaptive casing: they collapse with no Conflict, and
	// the Title-Case form is preferred as the canonical survivor.
	kept3, dropped3 := DedupeReplacements([]Replacement{
		{From: "Boxie", To: "Voxi"},
		{From: "boxie", To: "voxi"},
	})
	if len(kept3) != 1 || kept3[0].From != "Boxie" || kept3[0].To != "Voxi" {
		t.Fatalf("kept3 = %#v", kept3)
	}
	if len(dropped3) != 1 || dropped3[0].Conflict {
		t.Fatalf("dropped3 = %#v, want one benign drop", dropped3)
	}
}

func TestReplacementCleanupCommand(t *testing.T) {
	home := t.TempDir()
	path := ReplacementPath(home)
	var out bytes.Buffer
	run := func(args ...string) error {
		out.Reset()
		cmd := NewCommand(&out, home, rules, 64, nil, deps.Dependencies{}, nil)
		cmd.SetArgs(args)
		return cmd.Execute()
	}
	if err := run("replacement", "cleanup"); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "No duplicate replacements found.\n" {
		t.Fatalf("cleanup on empty file = %q", got)
	}

	// Simulate a dictionary built before case-insensitive matching, which
	// AddReplacement can no longer produce directly.
	if err := SaveReplacements(path, []Replacement{
		{From: "Voxy", To: "voxi"},
		{From: "voxy", To: "voxi"},
		{From: "VOXY", To: "voxi2"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := run("replacement", "cleanup", "--dry-run"); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "Dry run: 2 of 3 replacements would be removed") {
		t.Fatalf("dry-run summary = %q", got)
	}
	if !strings.Contains(got, "conflicts with kept") {
		t.Fatalf("dry-run output missing conflict note = %q", got)
	}
	loaded, err := LoadReplacements(path)
	if err != nil || len(loaded) != 3 {
		t.Fatalf("dry-run must not write: loaded = %#v, err = %v", loaded, err)
	}

	if err := run("replacement", "cleanup"); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "Removed 2 duplicate replacement(s); 1 remain.") {
		t.Fatalf("cleanup summary = %q", got)
	}
	loaded, err = LoadReplacements(path)
	if err != nil || len(loaded) != 1 || loaded[0].From != "Voxy" || loaded[0].To != "voxi" {
		t.Fatalf("loaded after cleanup = %#v, err = %v", loaded, err)
	}
}
