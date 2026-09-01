package feedback

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ubunatic.com/voxi/spec"
)

var rules = []spec.StopWord{{ID: "bye", Pattern: "bye"}, {ID: "thanks", Pattern: "thank you"}}

func TestSaveLoadAtomicPermissionsAndNormalize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "stop-words.json")
	o, err := Add(Overrides{}, " Bye ")
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(path, o); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("permissions = %o, want 0600", got)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.User) != 1 || got.User[0] != "Bye" {
		t.Fatalf("loaded %#v", got)
	}
}

func TestAddRemoveAndBuiltinState(t *testing.T) {
	o, err := Add(Overrides{}, "bye")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Add(o, "BYE"); err == nil {
		t.Fatal("duplicate add succeeded")
	}
	o, err = SetBuiltin(o, "bye", false, rules)
	if err != nil {
		t.Fatal(err)
	}
	if got := ActivePatterns(rules, o); len(got) != 2 || got[0] != "thank you" {
		t.Fatalf("active patterns %#v", got)
	}
	o, err = SetBuiltin(o, "bye", true, rules)
	if err != nil {
		t.Fatal(err)
	}
	o, err = Remove(o, "BYE")
	if err != nil {
		t.Fatal(err)
	}
	if len(o.User) != 0 || len(o.Disabled) != 0 {
		t.Fatalf("unexpected overrides %#v", o)
	}
}

func TestRejectInvalidAndMalformed(t *testing.T) {
	if _, err := Add(Overrides{}, "\x00"); err == nil {
		t.Fatal("control character accepted")
	}
	if _, err := Add(Overrides{}, strings.Repeat("x", 201)); err == nil {
		t.Fatal("long phrase accepted")
	}
	path := filepath.Join(t.TempDir(), "stop-words.json")
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("malformed file accepted")
	}
}

func TestLiteralPatternsAndCommand(t *testing.T) {
	p := literalPattern(`a.+(x)`)
	if !strings.Contains(p, `a\.\+\(x\)`) {
		t.Fatalf("pattern %q did not quote literal", p)
	}
	var out bytes.Buffer
	cmd := NewCommand(&out, t.TempDir(), rules)
	cmd.SetArgs([]string{"stop-word", "add", "bye"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "remove") {
		t.Fatalf("add output %q lacks reversal", out.String())
	}
	out.Reset()
	cmd.SetArgs([]string{"stop-word", "disable", "bye"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	cmd.SetArgs([]string{"stop-word", "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "disabled\tbye") || !strings.Contains(out.String(), "user\t-\tbye") {
		t.Fatalf("list output %q", out.String())
	}
}
