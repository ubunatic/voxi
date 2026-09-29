package sample

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/deps"
)

func guardStore(t *testing.T, dataHome string, xs ...Sample) *Store {
	t.Helper()
	store, err := Open(Root(dataHome))
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range xs {
		src := filepath.Join(t.TempDir(), x.ID+".wav")
		if err := os.WriteFile(src, []byte("RIFF-"+x.ID), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := store.Put(x, src); err != nil {
			t.Fatal(err)
		}
	}
	return store
}

func runGuard(t *testing.T, dataHome string, d deps.Dependencies, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := testCommand(t, dataHome, t.TempDir(), &out, d)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestPutVoiceRequiresConsent(t *testing.T) {
	store := guardStore(t, t.TempDir())
	src := filepath.Join(t.TempDir(), "v.wav")
	if err := os.WriteFile(src, []byte("RIFF"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(Sample{ID: "v", Purpose: Voice, Transcript: "hi"}, src); err == nil {
		t.Fatal("voice sample without consent was stored")
	}
	if err := store.Add(Sample{ID: "w", Purpose: Voice, Transcript: "hi"}); err == nil {
		t.Fatal("voice sidecar without consent was added")
	}
	if err := RequireConsent([]Sample{{ID: "a", Purpose: Voice}, {ID: "b", Purpose: Dictation}}); err == nil || !strings.Contains(err.Error(), "a") || strings.Contains(err.Error(), "b") {
		t.Fatalf("RequireConsent = %v, want error naming only a", err)
	}
}

func TestMoveIntoVoiceAsksForConsent(t *testing.T) {
	dataHome := t.TempDir()
	store := guardStore(t, dataHome,
		Sample{ID: "mine", Purpose: Dictation, Transcript: "hello"},
		Sample{ID: "flagged", Purpose: Dictation, Transcript: "hello"},
		Sample{ID: "silent", Purpose: Noise})

	out, err := runGuard(t, dataHome, deps.Dependencies{Stdin: strings.NewReader("n\n")}, "move", "mine", "voice")
	if err == nil || !strings.Contains(out, "This is my own voice and I consent to cloning it? [y/N]") {
		t.Fatalf("declined consent: err=%v out=%q", err, out)
	}
	if x, _ := store.Get("mine"); x.Purpose != Dictation || x.Consent != nil {
		t.Fatalf("declined move changed sample: %+v", x)
	}

	if _, err := runGuard(t, dataHome, deps.Dependencies{Stdin: strings.NewReader("y\n")}, "move", "mine", "voice"); err != nil {
		t.Fatal(err)
	}
	if x, _ := store.Get("mine"); x.Purpose != Voice || x.Consent == nil || time.Since(*x.Consent) > time.Minute {
		t.Fatalf("confirmed move: %+v", x)
	}

	if _, err := runGuard(t, dataHome, deps.Dependencies{}, "move", "flagged", "voice", "--own-voice"); err != nil {
		t.Fatal(err)
	}
	if x, _ := store.Get("flagged"); x.Purpose != Voice || x.Consent == nil {
		t.Fatalf("--own-voice move: %+v", x)
	}

	if _, err := runGuard(t, dataHome, deps.Dependencies{}, "move", "silent", "voice", "--own-voice"); err == nil {
		t.Fatal("moved a sample without transcript into voice")
	}
}

func TestAddVoiceFromChunkStoresConsent(t *testing.T) {
	dataHome, runtime := t.TempDir(), t.TempDir()
	addChunkFixture(t, runtime, "my voice")
	var out bytes.Buffer
	cmd := testCommand(t, dataHome, runtime, &out, deps.Dependencies{Getenv: func(k string) string {
		if k == "VOXI_SAMPLE_EDITOR" {
			return "off"
		}
		return ""
	}, Stdin: strings.NewReader("\n")})
	cmd.SetArgs([]string{"add", "mine", "--last", "--purpose", "voice", "--own-voice"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v (%s)", err, out.String())
	}
	store, _ := Open(Root(dataHome))
	x, err := store.Get("mine")
	if err != nil || x.Purpose != Voice || x.Consent == nil {
		t.Fatalf("added voice sample %+v %v", x, err)
	}
}

func fakeFFmpeg(calls *[]string) func(context.Context, string, ...string) error {
	return func(_ context.Context, name string, args ...string) error {
		*calls = append(*calls, name+" "+strings.Join(args, " "))
		return os.WriteFile(args[len(args)-1], []byte("fLaC"), 0o600)
	}
}

func TestPublishOnlyNoiseAfterConfirmation(t *testing.T) {
	dataHome := t.TempDir()
	now := time.Now()
	guardStore(t, dataHome,
		Sample{ID: "said", Purpose: Dictation, Transcript: "hello"},
		Sample{ID: "me", Purpose: Voice, Transcript: "hello", Consent: &now},
		Sample{ID: "clack", Purpose: Noise, Source: "record"})
	public := filepath.Join(t.TempDir(), "samples")
	var calls []string
	d := deps.Dependencies{Run: fakeFFmpeg(&calls)}

	for _, id := range []string{"said", "me"} {
		if _, err := runGuard(t, dataHome, d, "publish", id, "--no-speech", "--public-store", public); err == nil {
			t.Errorf("published %s sample", id)
		}
	}
	d.Stdin = strings.NewReader("n\n")
	if _, err := runGuard(t, dataHome, d, "publish", "clack", "--public-store", public); err == nil {
		t.Error("published without no-speech confirmation")
	}
	if len(calls) != 0 {
		t.Fatalf("refused publishes ran ffmpeg: %v", calls)
	}
	if _, err := os.Stat(public); err == nil {
		t.Fatal("refused publishes created the public store")
	}

	d.Stdin = strings.NewReader("y\n")
	if _, err := runGuard(t, dataHome, d, "publish", "clack", "--public-store", public); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(public, "noise", "clack.flac"))
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("public flac: %v %v", info, err)
	}
	pub, _ := OpenReadOnly(public)
	x, err := pub.Get("clack")
	if err != nil || x.Purpose != Noise || x.Source != "published:record" {
		t.Fatalf("public sidecar %+v %v", x, err)
	}
	out, err := runGuard(t, dataHome, d, "list", "--public", "--public-store", public)
	if err != nil || !strings.Contains(out, "clack\tnoise") {
		t.Fatalf("list --public: %q %v", out, err)
	}
	if _, err := runGuard(t, dataHome, d, "publish", "clack", "--no-speech", "--public-store", public); err == nil {
		t.Fatal("republish overwrote public sample")
	}
}
