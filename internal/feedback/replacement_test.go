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
		{"word and punctuation", "Voxy, then Voxy!", []Replacement{{"Voxy", "voxi"}}, "voxi, then voxi!"},
		{"embedded word rejected", "Voxyology myVoxy Voxy_2", []Replacement{{"Voxy", "voxi"}}, "Voxyology myVoxy Voxy_2"},
		{"case insensitive", "voxy VOXY Voxy", []Replacement{{"Voxy", "voxi"}}, "voxi voxi voxi"},
		{"unicode boundaries", "überVoxy Voxy—Voxy Voxy猫", []Replacement{{"Voxy", "voxi"}}, "überVoxy voxi—voxi Voxy猫"},
		{"phrase flexible whitespace", "Voxy\tproject and Voxy\nproject", []Replacement{{"Voxy project", "voxi project"}}, "voxi project and voxi project"},
		{"all aliases", "Voxy and Voxie", []Replacement{{"Voxy", "voxi"}, {"Voxie", "voxi"}}, "voxi and voxi"},
		{"longest overlap", "Voxy project", []Replacement{{"Voxy", "wrong"}, {"Voxy project", "voxi project"}}, "voxi project"},
		{"non cascading", "Voxy Voxi", []Replacement{{"Voxy", "Voxi"}, {"Voxi", "changed"}}, "Voxi changed"},
		{"literal target", "Voxy", []Replacement{{"Voxy", "voxi™"}}, "voxi™"},
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
	if got := out.String(); got != "Voxy\tvoxi\nVoxy project\tvoxi project\n" {
		t.Fatalf("list = %q", got)
	}
	if err := run("replacement", "remove", "voxy"); err != nil {
		t.Fatalf("case-insensitive remove error = %v", err)
	}
	loaded, err := LoadReplacements(path)
	if err != nil || len(loaded) != 1 || loaded[0].From != "Voxy project" {
		t.Fatalf("loaded = %#v, err = %v", loaded, err)
	}
	if _, _, err := AddReplacement(nil, "", "x"); err == nil {
		t.Error("empty source accepted")
	}
	if _, _, err := AddReplacement(nil, "x", " "); err == nil {
		t.Error("empty target accepted")
	}
	if _, _, err := AddReplacement(nil, "x\n", "y"); err != nil {
		t.Errorf("ordinary source whitespace should normalize: %v", err)
	}
	if _, _, err := AddReplacement(nil, "x", "y\x00"); err == nil {
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
