package sample

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/audio"
	"ubunatic.com/voxi/internal/chunks"
	"ubunatic.com/voxi/internal/deps"
)

// pcmOf returns n 16-bit samples of value v.
func pcmOf(n int, v byte) []byte {
	b := make([]byte, 2*n)
	for i := 0; i < len(b); i += 2 {
		b[i] = v
	}
	return b
}

func putWAV(t *testing.T, store *Store, x Sample, pcm []byte, rate int) {
	t.Helper()
	src := filepath.Join(t.TempDir(), x.ID+".wav")
	if err := audio.WriteWAVAudio(src, pcm, rate); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(x, src); err != nil {
		t.Fatal(err)
	}
}

var editorOff = func(k string) string {
	if k == "VOXI_SAMPLE_EDITOR" {
		return "off"
	}
	return ""
}

func TestMergeSamplesJoinsAudioTranscriptAndProvenance(t *testing.T) {
	dataHome := t.TempDir()
	store := guardStore(t, dataHome)
	putWAV(t, store, Sample{ID: "b", Purpose: Dictation, Transcript: "second part.", Keyterms: []string{"Voxi", "Go"}}, pcmOf(1600, 9), 16000)
	putWAV(t, store, Sample{ID: "a", Purpose: Dictation, Transcript: " first part. ", Keyterms: []string{"Voxi"}}, pcmOf(800, 5), 16000)
	before, _ := os.ReadFile(store.AudioPath(Sample{Purpose: Dictation, Audio: "a.wav"}))

	// Argument order wins: b before a.
	out, err := runGuard(t, dataHome, deps.Dependencies{Getenv: editorOff}, "merge", "joined", "b", "a", "--gap", "100ms")
	if err != nil {
		t.Fatalf("%v (%s)", err, out)
	}
	x, err := store.Get("joined")
	if err != nil {
		t.Fatal(err)
	}
	if x.Purpose != Dictation || x.Transcript != "second part. first part." || x.Source != "merge:samples:b,a" || strings.Join(x.Keyterms, "|") != "Voxi|Go" {
		t.Fatalf("merged sidecar %+v", x)
	}
	w, err := readWAV(store.AudioPath(x))
	if err != nil {
		t.Fatal(err)
	}
	want := append(append(pcmOf(1600, 9), make([]byte, 2*1600)...), pcmOf(800, 5)...)
	if w.rate != 16000 || !bytes.Equal(w.pcm, want) {
		t.Fatalf("merged pcm: rate %d, %d bytes, want %d bytes b+100ms+a", w.rate, len(w.pcm), len(want))
	}
	after, _ := os.ReadFile(store.AudioPath(Sample{Purpose: Dictation, Audio: "a.wav"}))
	if !bytes.Equal(before, after) {
		t.Fatal("merge changed an input")
	}
	if _, err := runGuard(t, dataHome, deps.Dependencies{Getenv: editorOff}, "merge", "joined", "a", "b"); err == nil {
		t.Fatal("merge overwrote an existing sample")
	}
}

func TestMergeSamplesRefusals(t *testing.T) {
	dataHome := t.TempDir()
	store := guardStore(t, dataHome)
	putWAV(t, store, Sample{ID: "d1", Purpose: Dictation, Transcript: "one"}, pcmOf(100, 1), 16000)
	putWAV(t, store, Sample{ID: "d2", Purpose: Dictation, Transcript: "two"}, pcmOf(100, 1), 22050)
	putWAV(t, store, Sample{ID: "n1", Purpose: Noise}, pcmOf(100, 1), 16000)
	putWAV(t, store, Sample{ID: "long", Purpose: Dictation, Transcript: "long"}, pcmOf(16000*15, 1), 16000)
	for name, args := range map[string][]string{
		"rate mismatch":    {"merge", "x", "d1", "d2"},
		"purpose mismatch": {"merge", "x", "d1", "n1"},
		"duplicate source": {"merge", "x", "d1", "d1"},
		"too long":         {"merge", "x", "long", "long2"},
		"unknown source":   {"merge", "x", "d1", "nope"},
		"single source":    {"merge", "x", "d1"},
	} {
		if name == "too long" {
			putWAV(t, store, Sample{ID: "long2", Purpose: Dictation, Transcript: "long"}, pcmOf(16000*6, 1), 16000)
		}
		if _, err := runGuard(t, dataHome, deps.Dependencies{Getenv: editorOff}, args...); err == nil {
			t.Errorf("%s: merge succeeded", name)
		}
		if ok, _ := store.Has("x"); ok {
			t.Fatalf("%s: refused merge left sample x", name)
		}
	}
}

func TestMergeVoiceSamplesKeepConsentOthersNeedIt(t *testing.T) {
	dataHome := t.TempDir()
	store := guardStore(t, dataHome)
	early, late := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	putWAV(t, store, Sample{ID: "v1", Purpose: Voice, Transcript: "one", Consent: &early}, pcmOf(100, 1), 16000)
	putWAV(t, store, Sample{ID: "v2", Purpose: Voice, Transcript: "two", Consent: &late}, pcmOf(100, 1), 16000)
	putWAV(t, store, Sample{ID: "d1", Purpose: Dictation, Transcript: "one"}, pcmOf(100, 1), 16000)
	putWAV(t, store, Sample{ID: "d2", Purpose: Dictation, Transcript: "two"}, pcmOf(100, 1), 16000)
	if _, err := runGuard(t, dataHome, deps.Dependencies{Getenv: editorOff}, "merge", "vv", "v1", "v2"); err != nil {
		t.Fatal(err)
	}
	if x, _ := store.Get("vv"); x.Purpose != Voice || x.Consent == nil || !x.Consent.Equal(late) {
		t.Fatalf("voice merge %+v", x)
	}
	if _, err := runGuard(t, dataHome, deps.Dependencies{Getenv: editorOff, Stdin: strings.NewReader("n\n")}, "merge", "dv", "d1", "d2", "--purpose", "voice"); err == nil {
		t.Fatal("dictation merged into voice without consent")
	}
	if _, err := runGuard(t, dataHome, deps.Dependencies{Getenv: editorOff}, "merge", "dv", "d1", "d2", "--purpose", "voice", "--own-voice"); err != nil {
		t.Fatal(err)
	}
	if x, _ := store.Get("dv"); x.Purpose != Voice || x.Consent == nil {
		t.Fatalf("dictation->voice merge %+v", x)
	}
}

func TestMergeChunksAdjacencyGuard(t *testing.T) {
	dataHome, runtime := t.TempDir(), t.TempDir()
	b := chunks.NewBuffer(chunks.StorageDir(runtime, ""), 10)
	now := time.Now()
	add := func(session string, at time.Time, accepted bool, reason, text string) int {
		c, err := b.Add(chunks.Chunk{SessionID: session, Timestamp: at, Accepted: accepted, RejectionReason: reason, CleanedTranscript: text}, pcmOf(160, 3), 16000)
		if err != nil {
			t.Fatal(err)
		}
		return c.Index
	}
	c1 := add("s1", now, true, "", "Hello")
	c2 := add("s1", now.Add(3*time.Second), true, "", "world.")
	other := add("s2", now.Add(4*time.Second), true, "", "other")
	late := add("s1", now.Add(2*time.Minute), true, "", "late")
	rejected := add("s1", now.Add(5*time.Second), false, "low_energy_transient", "")
	run := func(args ...string) error {
		var out bytes.Buffer
		cmd := testCommand(t, dataHome, runtime, &out, deps.Dependencies{Getenv: editorOff})
		cmd.SetArgs(args)
		return cmd.Execute()
	}
	for name, idx := range map[string]int{"cross-session": other, "time gap": late, "rejected": rejected} {
		if err := run("merge", "x", "--chunks", fmt.Sprint(c1), fmt.Sprint(idx)); err == nil {
			t.Errorf("%s chunk merged without --force", name)
		}
	}
	if err := run("merge", "para", "--chunks", fmt.Sprint(c1), fmt.Sprint(c2)); err != nil {
		t.Fatal(err)
	}
	store, _ := Open(Root(dataHome))
	x, err := store.Get("para")
	if err != nil || x.Purpose != Dictation || x.Transcript != "Hello world." || x.Source != fmt.Sprintf("merge:chunks:s1/%d,s1/%d", c1, c2) {
		t.Fatalf("chunk merge %+v %v", x, err)
	}
	if err := run("merge", "forced", "--chunks", fmt.Sprint(c1), fmt.Sprint(other), "--force"); err != nil {
		t.Fatalf("--force: %v", err)
	}
	if x, _ := store.Get("forced"); x.Source != fmt.Sprintf("merge:chunks:s1/%d,s2/%d", c1, other) {
		t.Fatalf("forced cross-session provenance %q", x.Source)
	}
}

func TestReadWAVOddDataRefusedByMerge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "odd.wav")
	if err := audio.WriteWAVAudio(path, []byte{1, 2, 3}, 16000); err != nil {
		t.Fatal(err)
	}
	ok := filepath.Join(dir, "ok.wav")
	if err := audio.WriteWAVAudio(ok, pcmOf(10, 1), 16000); err != nil {
		t.Fatal(err)
	}
	_, _, _, err := joinAudio([]mergePart{{label: "ok", wav: ok}, {label: "odd", wav: path}}, 0, time.Minute)
	if err == nil || !strings.Contains(err.Error(), "partial") {
		t.Fatalf("joinAudio = %v, want partial-sample error", err)
	}
}
