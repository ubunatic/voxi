package sample

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"ubunatic.com/voxi/internal/chunks"
	"ubunatic.com/voxi/internal/deps"
	"ubunatic.com/voxi/internal/feedback"
)

func TestCommandListsShowsDeletesStoreSamples(t *testing.T) {
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
	for _, args := range [][]string{{"list", "--purpose", "dictation"}, {"show", "one"}, {"delete", "one"}} {
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	if !strings.Contains(out.String(), "one\tdictation\thello") || !strings.Contains(out.String(), "transcript: hello") {
		t.Fatalf("command output: %s", out.String())
	}
	if _, err := store.Get("one"); err == nil {
		t.Fatal("delete left sample in store")
	}
}

type delayedReader struct {
	reader io.Reader
	delay  time.Duration
	first  bool
}

func (r *delayedReader) Read(p []byte) (int, error) {
	if !r.first {
		r.first = true
		time.Sleep(r.delay)
	}
	return r.reader.Read(p)
}

func TestRecordCapturesAndStoresWithPurpose(t *testing.T) {
	home, bin := t.TempDir(), t.TempDir()
	recorder := filepath.Join(bin, "pw-record")
	if err := os.WriteFile(recorder, []byte("#!/bin/sh\nsleep 0.1\ndd if=/dev/zero bs=32000 count=1 2>/dev/null\nexec sleep 5\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	d := deps.Dependencies{
		LookPath: func(name string) (string, error) {
			if name == "pw-record" {
				return recorder, nil
			}
			return "", os.ErrNotExist
		},
		Stdin:  &delayedReader{reader: strings.NewReader("\nhello there\n\n"), delay: 300 * time.Millisecond},
		Stdout: &bytes.Buffer{},
	}
	var out bytes.Buffer
	d.Stdout = &out
	cmd := testCommand(t, home, "", &out, d)
	cmd.SetArgs([]string{"record", "room-tone"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	store, err := Open(Root(home))
	if err != nil {
		t.Fatal(err)
	}
	x, err := store.Get("room-tone")
	if err != nil {
		t.Fatal(err)
	}
	if x.Purpose != Dictation || x.Transcript != "hello there" || x.Source != "record" {
		t.Fatalf("recorded sample = %+v", x)
	}
	audioInfo, err := os.Stat(store.AudioPath(x))
	if err != nil {
		t.Fatal(err)
	}
	if audioInfo.Mode().Perm() != 0o600 {
		t.Fatalf("recording mode = %o", audioInfo.Mode().Perm())
	}
}

func testCommand(t *testing.T, dataHome string, runtime string, out *bytes.Buffer, d deps.Dependencies) *cobra.Command {
	t.Helper()
	priorGetenv := d.Getenv
	d.Getenv = func(k string) string {
		switch k {
		case "XDG_DATA_HOME":
			return dataHome
		case "XDG_RUNTIME_DIR":
			return runtime
		case "HOME":
			return t.TempDir()
		}
		if priorGetenv != nil {
			return priorGetenv(k)
		}
		return ""
	}
	d.Stdout = out
	return NewCommand(d)
}

func addChunkFixture(t *testing.T, dir string, transcript string) chunks.Chunk {
	t.Helper()
	b := chunks.NewBuffer(chunks.StorageDir(dir, ""), 10)
	c, err := b.Add(chunks.Chunk{SessionID: "session-test", RawTranscript: transcript, CleanedTranscript: transcript, Accepted: true}, []byte{0, 1, 0, 2}, 16000)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAddChunkAndLastCopyAudioSidecarAndPermissions(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		text string
	}{
		{"index", []string{"add", "indexed", "--chunk", "1"}, "first text"},
		{"last", []string{"add", "recent", "--last"}, "last text"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dataHome, runtime := t.TempDir(), t.TempDir()
			c1 := addChunkFixture(t, runtime, "first text")
			c2 := addChunkFixture(t, runtime, tc.text)
			if tc.name == "index" {
				_ = c2
			}
			var out bytes.Buffer
			cmd := testCommand(t, dataHome, runtime, &out, deps.Dependencies{})
			cmd.SetArgs(tc.args)
			if err := cmd.ExecuteContext(context.Background()); err != nil {
				t.Fatal(err)
			}
			store, err := Open(Root(dataHome))
			if err != nil {
				t.Fatal(err)
			}
			id := "indexed"
			idx := 1
			if tc.name == "last" {
				id, idx = "recent", 2
			}
			x, err := store.Get(id)
			if err != nil {
				t.Fatal(err)
			}
			if x.Source != fmt.Sprintf("chunk:session-test/%d", idx) || x.Transcript != tc.text {
				t.Fatalf("sidecar = %+v", x)
			}
			input := filepath.Join(chunks.StorageDir(runtime, ""), c1.WAVFile)
			if tc.name == "last" {
				input = filepath.Join(chunks.StorageDir(runtime, ""), c2.WAVFile)
			}
			gotAudio, err := os.ReadFile(store.AudioPath(x))
			if err != nil {
				t.Fatal(err)
			}
			wantAudio, err := os.ReadFile(input)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(gotAudio, wantAudio) {
				t.Fatal("stored audio differs from source chunk")
			}
			info, err := os.Stat(store.AudioPath(x))
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o600 {
				t.Fatalf("audio mode = %o", info.Mode().Perm())
			}
		})
	}
}

func TestEveryCommandHasShortAndExample(t *testing.T) {
	cmd := NewCommand(deps.Dependencies{Getenv: func(string) string { return t.TempDir() }})
	want := []string{"add", "delete", "edit", "list", "move", "play", "record", "show"}
	got := make([]string, 0, len(cmd.Commands()))
	for _, child := range cmd.Commands() {
		got = append(got, child.Name())
		if strings.TrimSpace(child.Short) == "" || !strings.Contains(child.Long, "Example:") {
			t.Errorf("%s help missing Short or example: Short=%q Long=%q", child.Name(), child.Short, child.Long)
		}
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("sample commands = %v, want %v", got, want)
	}
}

func TestAddRejectsUnknownDuplicateAndVoicePurpose(t *testing.T) {
	dataHome, runtime := t.TempDir(), t.TempDir()
	addChunkFixture(t, runtime, "source")
	store, err := Open(Root(dataHome))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Add(Sample{ID: "dupe", Purpose: Dictation, Transcript: "kept"}); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "unknown", "--chunk", "7"}, {"add", "dupe", "--last"}, {"add", "voice", "--last", "--purpose", "voice"}} {
		var out bytes.Buffer
		cmd := testCommand(t, dataHome, runtime, &out, deps.Dependencies{})
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Errorf("%v unexpectedly succeeded", args)
		}
	}
	x, err := store.Get("dupe")
	if err != nil || x.Transcript != "kept" {
		t.Fatalf("duplicate altered existing sample: %+v, %v", x, err)
	}
}

func TestEmptyTranscriptAllowedOnlyForNoiseAddAndEdit(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	dataHome, runtime := t.TempDir(), t.TempDir()
	addChunkFixture(t, runtime, "")
	var out bytes.Buffer
	cmd := testCommand(t, dataHome, runtime, &out, deps.Dependencies{})
	cmd.SetArgs([]string{"add", "silent-room", "--last", "--purpose", "noise"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("noise add with empty transcript: %v", err)
	}
	store, err := Open(Root(dataHome))
	if err != nil {
		t.Fatal(err)
	}
	noise, err := store.Get("silent-room")
	if err != nil || noise.Transcript != "" {
		t.Fatalf("noise sample = %+v, %v", noise, err)
	}

	addChunkFixture(t, runtime, "")
	cmd = testCommand(t, dataHome, runtime, &out, deps.Dependencies{})
	cmd.SetArgs([]string{"add", "empty-dictation", "--last"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("dictation add with empty transcript succeeded")
	}

	if err := store.Add(Sample{ID: "empty-noise", Purpose: Noise, Transcript: ""}); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(Sample{ID: "dictation", Purpose: Dictation, Transcript: "before"}); err != nil {
		t.Fatal(err)
	}
	editor := filepath.Join(t.TempDir(), "editor.sh")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\n: > \"$1\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	d := deps.Dependencies{Getenv: func(k string) string {
		if k == "XDG_DATA_HOME" {
			return dataHome
		}
		if k == "EDITOR" {
			return editor
		}
		return ""
	}}
	cmd = testCommand(t, dataHome, "", &out, d)
	cmd.SetArgs([]string{"edit", "dictation"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("dictation edit to empty transcript succeeded")
	}
	cmd = testCommand(t, dataHome, "", &out, d)
	cmd.SetArgs([]string{"edit", "empty-noise"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("noise edit to empty transcript: %v", err)
	}
	noise, err = store.Get("empty-noise")
	if err != nil || noise.Transcript != "" {
		t.Fatalf("edited noise sample = %+v, %v", noise, err)
	}
}

func TestRecordNoiseSkipsTranscriptPromptForEmptyASR(t *testing.T) {
	home, bin := t.TempDir(), t.TempDir()
	recorder := filepath.Join(bin, "pw-record")
	if err := os.WriteFile(recorder, []byte("#!/bin/sh\nsleep 0.1\ndd if=/dev/zero bs=32000 count=1 2>/dev/null\nexec sleep 5\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	d := deps.Dependencies{
		LookPath: func(name string) (string, error) {
			if name == "pw-record" {
				return recorder, nil
			}
			return "", os.ErrNotExist
		},
		Stdin: &delayedReader{reader: strings.NewReader("\n\n"), delay: 300 * time.Millisecond},
	}
	var out bytes.Buffer
	cmd := testCommand(t, home, "", &out, d)
	cmd.SetArgs([]string{"record", "--purpose", "noise", "silent-room"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "Enter the corrected transcript") {
		t.Fatalf("asked transcript question despite empty ASR result: %s", out.String())
	}
	if !strings.Contains(out.String(), "No ASR transcript detected; saving this noise sample with an empty transcript.") {
		t.Fatalf("missing empty-transcript confirmation line: %s", out.String())
	}
	store, err := Open(Root(home))
	if err != nil {
		t.Fatal(err)
	}
	x, err := store.Get("silent-room")
	if err != nil || x.Purpose != Noise || x.Transcript != "" {
		t.Fatalf("recorded sample = %+v, %v", x, err)
	}
}

func TestRecordDictationRejectsEmptyTranscript(t *testing.T) {
	home, bin := t.TempDir(), t.TempDir()
	recorder := filepath.Join(bin, "pw-record")
	if err := os.WriteFile(recorder, []byte("#!/bin/sh\nsleep 0.1\ndd if=/dev/zero bs=32000 count=1 2>/dev/null\nexec sleep 5\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	d := deps.Dependencies{
		LookPath: func(name string) (string, error) {
			if name == "pw-record" {
				return recorder, nil
			}
			return "", os.ErrNotExist
		},
		Stdin: &delayedReader{reader: strings.NewReader("\n\n"), delay: 300 * time.Millisecond},
	}
	var out bytes.Buffer
	cmd := testCommand(t, home, "", &out, d)
	cmd.SetArgs([]string{"record", "dictation-sample"})
	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("dictation record with empty transcript succeeded")
	}
	store, err := Open(Root(home))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("dictation-sample"); err == nil {
		t.Fatal("empty dictation sample was saved")
	}
}

func TestPlayUsesInjectedRunner(t *testing.T) {
	dataHome := t.TempDir()
	store, err := Open(Root(dataHome))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Add(Sample{ID: "playme", Purpose: Noise, Transcript: "", Source: "record"}); err != nil {
		t.Fatal(err)
	}
	wav := filepath.Join(store.Root(), "noise", "playme.wav")
	if err := os.WriteFile(wav, []byte("wav"), 0o600); err != nil {
		t.Fatal(err)
	}
	var gotName string
	var gotArgs []string
	d := deps.Dependencies{LookPath: func(name string) (string, error) {
		if name != "paplay" {
			t.Fatalf("lookpath %q", name)
		}
		return "/fake/paplay", nil
	}, Run: func(_ context.Context, name string, args ...string) error { gotName = name; gotArgs = args; return nil }}
	var out bytes.Buffer
	cmd := testCommand(t, dataHome, "", &out, d)
	cmd.SetArgs([]string{"play", "playme"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if gotName != "paplay" || len(gotArgs) != 1 || gotArgs[0] != wav {
		t.Fatalf("runner called %q %q", gotName, gotArgs)
	}
}

func TestDeleteUnknownIDIsAllOrNothing(t *testing.T) {
	dataHome := t.TempDir()
	store, err := Open(Root(dataHome))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"one", "two"} {
		if err := store.Add(Sample{ID: id, Purpose: Dictation, Transcript: id}); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	cmd := testCommand(t, dataHome, "", &out, deps.Dependencies{})
	cmd.SetArgs([]string{"delete", "one", "missing"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("unknown id accepted")
	}
	for _, id := range []string{"one", "two"} {
		if _, err := store.Get(id); err != nil {
			t.Fatalf("%s was removed: %v", id, err)
		}
	}
}

func TestFeedbackSampleCommandIsGone(t *testing.T) {
	cmd := feedback.NewCommand(&bytes.Buffer{}, t.TempDir(), nil, 64, nil)
	if _, _, err := cmd.Find([]string{"sample"}); err == nil {
		t.Fatal("feedback sample remains registered")
	}
}

func TestEditAndMoveCommands(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	dataHome := t.TempDir()
	store, err := Open(Root(dataHome))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Add(Sample{ID: "editme", Purpose: Dictation, Transcript: "before"}); err != nil {
		t.Fatal(err)
	}
	// Use the existing shared editor implementation through a temp script.
	script := filepath.Join(t.TempDir(), "editor.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'after\\n' > \"$1\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	d := deps.Dependencies{Getenv: func(k string) string {
		if k == "XDG_DATA_HOME" {
			return dataHome
		}
		if k == "EDITOR" {
			return script
		}
		return ""
	}}
	cmd := testCommand(t, dataHome, "", &out, d)
	cmd.SetArgs([]string{"edit", "editme"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	x, err := store.Get("editme")
	if err != nil || x.Transcript != "after" {
		t.Fatalf("edit result %+v %v", x, err)
	}
	cmd = testCommand(t, dataHome, "", &out, d)
	cmd.SetArgs([]string{"move", "editme", "noise"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	x, err = store.Get("editme")
	if err != nil || x.Purpose != Noise {
		t.Fatalf("move result %+v %v", x, err)
	}
	cmd = testCommand(t, dataHome, "", &out, d)
	cmd.SetArgs([]string{"move", "editme", "voice"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("move into voice unexpectedly succeeded")
	}
}
