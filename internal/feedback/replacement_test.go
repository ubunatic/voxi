package feedback

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		cmd := NewCommand(&out, home, rules, 64, nil)
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
		cmd := NewCommand(&out, home, rules, 64, nil)
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

func TestApplyReplacementsWithAudit(t *testing.T) {
	rules := []Replacement{
		{From: "Voxy", To: "voxi"},
		{From: "AI assistant", To: "assistant"},
		{From: "not applied", To: "other"},
	}

	text := "Testing Voxy with an AI assistant today."
	gotText, applied := ApplyReplacementsWithAudit(text, rules)

	wantText := "Testing Voxi with an assistant today."
	if gotText != wantText {
		t.Errorf("got text %q, want %q", gotText, wantText)
	}

	if len(applied) != 2 {
		t.Fatalf("expected 2 applied rules, got %d: %+v", len(applied), applied)
	}
	if applied[0].From != "Voxy" || applied[1].From != "AI assistant" {
		t.Errorf("unexpected applied rules: %+v", applied)
	}

	noMatchText, noApplied := ApplyReplacementsWithAudit("Just a normal sentence.", rules)
	if noMatchText != "Just a normal sentence." || len(noApplied) != 0 {
		t.Errorf("expected no applied rules on unmatched text, got %v (%+v)", noMatchText, noApplied)
	}
}

func TestLegacyReplacementJSONLoadsUncategorized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "replacements.json")
	if err := os.WriteFile(path, []byte(`[{"from":"Voxy","to":"voxi","fixed_case":true}]`), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadReplacements(path)
	if err != nil || len(loaded) != 1 {
		t.Fatalf("loaded = %#v, err = %v", loaded, err)
	}
	if loaded[0].Category != "" || !loaded[0].FixedCase || loaded[0].To != "voxi" {
		t.Fatalf("legacy rule = %#v", loaded[0])
	}
	if err := SaveReplacements(path, loaded); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"category"`) {
		t.Fatalf("uncategorized rule must omit category key: %s", b)
	}
}

func TestLoadReplacementsCategoryNormalizedAndValidated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "replacements.json")
	if err := os.WriteFile(path, []byte(`[{"from":"taia","to":"taia","category":" Jargon "}]`), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadReplacements(path)
	if err != nil || len(loaded) != 1 || loaded[0].Category != CategoryJargon {
		t.Fatalf("loaded = %#v, err = %v", loaded, err)
	}
	if err := os.WriteFile(path, []byte(`[{"from":"x","to":"y","category":"typo"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadReplacements(path); err == nil || !strings.Contains(err.Error(), "invalid stored replacement 1") {
		t.Fatalf("invalid stored category error = %v", err)
	}
}

func TestNormalizeCategory(t *testing.T) {
	tests := []struct {
		in, want string
		wantErr  bool
	}{
		{"", "", false},
		{"   ", "", false},
		{"jargon", "jargon", false},
		{"repair", "repair", false},
		{" JARGON\t", "jargon", false},
		{"Repair", "repair", false},
		{"typo", "", true},
		{"jargon repair", "", true},
	}
	for _, tt := range tests {
		got, err := NormalizeCategory(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("NormalizeCategory(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
		}
		if got != tt.want {
			t.Errorf("NormalizeCategory(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestAddReplacementWithCategory(t *testing.T) {
	rules, rule, err := AddReplacementWithCategory(nil, "taia", "taia", false, " Jargon ")
	if err != nil || rule.Category != CategoryJargon || len(rules) != 1 || rules[0].Category != CategoryJargon {
		t.Fatalf("add jargon: rules = %#v, rule = %#v, err = %v", rules, rule, err)
	}
	if _, _, err := AddReplacementWithCategory(rules, "heiko", "haiku", false, "typo"); err == nil {
		t.Error("invalid category accepted")
	}
	if _, _, err := AddReplacementWithCategory(rules, "TAIA", "other", false, "repair"); err == nil || !strings.Contains(err.Error(), "already mapped") {
		t.Errorf("duplicate with category error = %v", err)
	}
	// AddReplacement keeps its signature and stays uncategorized.
	plain, plainRule, err := AddReplacement(nil, "jules", "jules", false)
	if err != nil || plainRule.Category != "" || plain[0].Category != "" {
		t.Fatalf("AddReplacement category = %#v, err = %v", plain, err)
	}
}

func TestSetReplacementCategory(t *testing.T) {
	rules := []Replacement{{From: "Heiko", To: "haiku"}, {From: "quan", To: "qwen", Category: CategoryJargon}}
	updated, rule, err := SetReplacementCategory(rules, " HEIKO ", "Repair")
	if err != nil || rule.Category != CategoryRepair || rule.From != "Heiko" {
		t.Fatalf("set repair: rule = %#v, err = %v", rule, err)
	}
	if updated[0].Category != CategoryRepair || updated[1].Category != CategoryJargon {
		t.Fatalf("updated = %#v", updated)
	}
	if rules[0].Category != "" {
		t.Fatalf("input slice mutated: %#v", rules)
	}
	if _, _, err := SetReplacementCategory(updated, "missing", CategoryJargon); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("nonexistent rule error = %v", err)
	}
	if _, _, err := SetReplacementCategory(updated, "quan", ""); err == nil {
		t.Error("empty category accepted")
	}
	if _, _, err := SetReplacementCategory(updated, "quan", "typo"); err == nil {
		t.Error("invalid category accepted")
	}
}

func TestFilterAndApplyReplacementsByCategory(t *testing.T) {
	rules := []Replacement{
		{From: "taya", To: "taia", Category: CategoryJargon},
		{From: "heiko", To: "haiku", Category: CategoryRepair},
		{From: "jewel", To: "jules"},
	}
	if got := FilterReplacementsByCategory(rules, CategoryJargon); len(got) != 1 || got[0].From != "taya" {
		t.Fatalf("jargon filter = %#v", got)
	}
	if got := FilterReplacementsByCategory(rules, " REPAIR "); len(got) != 1 || got[0].From != "heiko" {
		t.Fatalf("repair filter = %#v", got)
	}
	if got := FilterReplacementsByCategory(rules); len(got) != 0 {
		t.Fatalf("no categories must select nothing, got %#v", got)
	}
	if got := FilterReplacementsByCategory(rules, ""); len(got) != 1 || got[0].From != "jewel" {
		t.Fatalf("empty category must select uncategorized, got %#v", got)
	}

	text := "taya heiko jewel"
	gotJargon, appliedJargon := ApplyReplacementsByCategory(text, rules, CategoryJargon)
	if gotJargon != "taia heiko jewel" || len(appliedJargon) != 1 || appliedJargon[0].From != "taya" {
		t.Errorf("jargon apply = %q, %+v", gotJargon, appliedJargon)
	}
	gotRepair, appliedRepair := ApplyReplacementsByCategory(text, rules, CategoryRepair)
	if gotRepair != "taya haiku jewel" || len(appliedRepair) != 1 || appliedRepair[0].From != "heiko" {
		t.Errorf("repair apply = %q, %+v", gotRepair, appliedRepair)
	}
	gotAll, appliedAll := ApplyReplacementsWithAudit(text, rules)
	if gotAll != "taia haiku jules" || len(appliedAll) != 3 {
		t.Errorf("live apply must keep all rules: %q, %+v", gotAll, appliedAll)
	}
}

func TestCategoryPreservedByDedupeAndImportMerge(t *testing.T) {
	kept, dropped := DedupeReplacements([]Replacement{
		{From: "Boxie", To: "Voxi", Category: CategoryRepair},
		{From: "boxie", To: "voxi"},
	})
	if len(kept) != 1 || kept[0].Category != CategoryRepair || len(dropped) != 1 {
		t.Fatalf("dedupe kept = %#v, dropped = %#v", kept, dropped)
	}

	dest := []Replacement{{From: "taia", To: "taia"}}
	src := []Replacement{{From: "taia", To: "taia", Category: CategoryJargon}, {From: "qwen", To: "qwen", Category: CategoryJargon}}
	merged, summary := mergeReplacements(dest, src, true)
	if summary.Imported != 2 || len(merged) != 2 {
		t.Fatalf("merge summary = %+v, merged = %#v", summary, merged)
	}
	for _, r := range merged {
		if r.Category != CategoryJargon {
			t.Errorf("merged rule lost category: %#v", r)
		}
	}
}

func TestReplacementCategoryCommands(t *testing.T) {
	home := t.TempDir()
	var out bytes.Buffer
	run := func(args ...string) error {
		out.Reset()
		cmd := NewCommand(&out, home, rules, 64, nil)
		cmd.SetArgs(args)
		return cmd.Execute()
	}
	if err := run("replacement", "add", "taia", "taia", "--category", " Jargon "); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "(category: jargon)") {
		t.Fatalf("add output = %q", got)
	}
	if err := run("replacement", "add", "heiko", "haiku", "--category", "typo"); err == nil {
		t.Fatal("add with invalid category accepted")
	}
	if err := run("replacement", "add", "ubonatic dot com", "ubunatic.com", "--fixed-case"); err != nil {
		t.Fatal(err)
	}
	if err := run("replacement", "add", "voxi dot com", "voxi.com", "--category", "jargon", "--fixed-case"); err != nil {
		t.Fatal(err)
	}
	wantAdd := "Added transcript replacement \"voxi dot com\" -> \"voxi.com\" (fixed case: always written exactly as given) (category: jargon). Remove it with: voxi feedback replacement remove \"voxi dot com\"\n"
	if got := out.String(); got != wantAdd {
		t.Fatalf("add with category and fixed-case output = %q, want %q", got, wantAdd)
	}
	if err := run("replacement", "add", "heiko", "haiku"); err != nil {
		t.Fatal(err)
	}
	if err := run("replacement", "set-category", "heiko", "repair"); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "Set category of transcript replacement \"heiko\" -> \"haiku\" to repair.\n" {
		t.Fatalf("set-category output = %q", got)
	}
	if err := run("replacement", "set-category", "missing", "repair"); err == nil {
		t.Error("set-category on missing rule accepted")
	}
	if err := run("replacement", "set-category", "heiko", "typo"); err == nil {
		t.Error("set-category with invalid category accepted")
	}
	if err := run("replacement", "list"); err != nil {
		t.Fatal(err)
	}
	want := "heiko\thaiku\t[repair]\ntaia\ttaia\t[jargon]\nubonatic dot com\tubunatic.com\t[fixed-case]\nvoxi dot com\tvoxi.com\t[jargon]\t[fixed-case]\n"
	if got := out.String(); got != want {
		t.Fatalf("list = %q, want %q", got, want)
	}
}
