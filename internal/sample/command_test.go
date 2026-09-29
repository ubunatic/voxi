package sample

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/deps"
)

func TestCommandListsShowsDeletesAndExportsStoreSamples(t *testing.T) {
	dataHome := t.TempDir()
	store, err := Open(Root(dataHome))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Add(Sample{ID: "one", Purpose: Dictation, Transcript: "hello", Created: time.Now(), Source: "record"}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd := NewCommand(deps.Dependencies{Getenv: func(k string) string {
		if k == "XDG_DATA_HOME" {
			return dataHome
		}
		return ""
	}, Stdout: &out})
	for _, args := range [][]string{{"list", "--purpose", "dictation"}, {"show", "one"}, {"export", "--tsv"}, {"delete", "one"}} {
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	if !strings.Contains(out.String(), "one\tdictation\thello") || !strings.Contains(out.String(), "# id\twav file") {
		t.Fatalf("command output: %s", out.String())
	}
	if _, err := store.Get("one"); err == nil {
		t.Fatal("delete left sample in store")
	}
}
