package sample

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/deps"
)

func writeLegacy(t *testing.T, dir, corpus, allowlist string, wavs map[string]string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "corpus.tsv"), []byte(corpus), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "voice-training.txt"), []byte(allowlist), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, content := range wavs {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMigrationPlanAssignsPurposesAndReportsProblems(t *testing.T) {
	dir := t.TempDir()
	writeLegacy(t, dir, "speech\tspeech.wav\thello\nnoise\tnoise.wav\t[typing]\nvoice\tvoice.wav\tread this\nmissing\tmissing.wav\tno file\n", "voice\nghost\n", map[string]string{"speech.wav": "s", "noise.wav": "n", "voice.wav": "v"})
	plan, err := PlanMigration(dir, time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 3 || len(plan.Problems) != 2 {
		t.Fatalf("plan = %+v", plan)
	}
	got := map[string]MigrationItem{}
	for _, item := range plan.Items {
		got[item.Sample.ID] = item
	}
	if got["speech"].Sample.Purpose != Dictation || got["noise"].Sample.Purpose != Noise {
		t.Fatalf("purposes = %+v", got)
	}
	if got["voice"].Sample.Purpose != Voice || got["voice"].Sample.Consent == nil || !strings.Contains(got["voice"].Reason, "consent") {
		t.Fatalf("voice migration = %+v", got["voice"])
	}
	if !strings.Contains(strings.Join(plan.Problems, "\n"), "ghost: allowlisted") || !strings.Contains(strings.Join(plan.Problems, "\n"), "missing: WAV") {
		t.Fatalf("problems = %v", plan.Problems)
	}
}

func TestMigrationDryRunWritesNothingAndRealRunVerifiesModes(t *testing.T) {
	legacy, dataHome := t.TempDir(), t.TempDir()
	writeLegacy(t, legacy, "voice\tvoice.wav\tread this\nnoise\tnoise.wav\t\ndict\tdict.wav\thello\n", "voice\n", map[string]string{"voice.wav": "voice", "noise.wav": "noise", "dict.wav": "dict"})
	var out bytes.Buffer
	cmd := NewCommand(deps.Dependencies{Getenv: func(k string) string {
		if k == "XDG_DATA_HOME" {
			return dataHome
		}
		return ""
	}, Stdout: &out})
	cmd.SetArgs([]string{"migrate", "--dry-run", "--from", legacy})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(Root(dataHome)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry run wrote store: %v", err)
	}
	if !strings.Contains(out.String(), "voice\tvoice") || !strings.Contains(out.String(), "total dictation: 1") {
		t.Fatalf("plan output = %s", out.String())
	}
	cmd = NewCommand(deps.Dependencies{Getenv: func(k string) string {
		if k == "XDG_DATA_HOME" {
			return dataHome
		}
		return ""
	}, Stdout: &out})
	cmd.SetArgs([]string{"migrate", "--from", legacy})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(Root(dataHome))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"voice", "noise", "dict"} {
		x, err := store.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{store.AudioPath(x), filepath.Join(store.dir(x.Purpose), id+".json")} {
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("%s mode: %v, %v", path, info, err)
			}
		}
	}
	info, err := os.Stat(Root(dataHome))
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("root mode: %v, %v", info, err)
	}
}

func TestMigrationHashConflictFailsWithoutOverwriting(t *testing.T) {
	legacy := t.TempDir()
	writeLegacy(t, legacy, "same\tsame.wav\thello\n", "", map[string]string{"same.wav": "source"})
	plan, err := PlanMigration(legacy, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(store.dir(Dictation), 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(store.dir(Dictation), "same.json"), []byte(`{"id":"same"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(store.dir(Dictation), "same.wav"), []byte("target"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = Migrate(store, legacy, plan); err == nil || !strings.Contains(err.Error(), "hash conflict") {
		t.Fatalf("conflict error = %v", err)
	}
}
