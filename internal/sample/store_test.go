package sample

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreRoundTripAndPrivatePermissions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	x := Sample{ID: "hello", Purpose: Dictation, Transcript: "hello there", Keyterms: []string{"hello"}, Created: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Source: "record"}
	if err = s.Add(x); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(x.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Transcript != x.Transcript || got.Source != x.Source || got.Purpose != Dictation {
		t.Fatalf("round trip: %+v", got)
	}
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("root mode %o", info.Mode().Perm())
	}
	info, err = os.Stat(filepath.Join(root, "dictation", "hello.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("sidecar mode %o", info.Mode().Perm())
	}
}

func TestOpenReadOnlyDoesNotChangeStorePermissions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := OpenReadOnly(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("read-only open changed store mode to %o", info.Mode().Perm())
	}
}

func TestDuplicateAcrossPurposesAndMoveDelete(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Add(Sample{ID: "same", Purpose: Noise}); err != nil {
		t.Fatal(err)
	}
	if err = s.Add(Sample{ID: "same", Purpose: Dictation}); err == nil {
		t.Fatal("duplicate id accepted")
	}
	if err = s.Move("same", Dictation); err != nil {
		t.Fatal(err)
	}
	if err = s.Delete("same"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get("same"); err == nil {
		t.Fatal("deleted sample remains")
	}
}

func TestMoveVoiceConsentAndAudio(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Add(Sample{ID: "voice-sample", Purpose: Dictation}); err != nil {
		t.Fatal(err)
	}
	oldAudio := filepath.Join(s.root, "dictation", "voice-sample.wav")
	if err = os.WriteFile(oldAudio, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = s.Move("voice-sample", Voice); err == nil {
		t.Fatal("move to voice without consent succeeded")
	}
	if _, err = os.Stat(oldAudio); err != nil {
		t.Fatalf("source audio changed after refused move: %v", err)
	}
	if _, err = os.Stat(filepath.Join(s.root, "dictation", "voice-sample.json")); err != nil {
		t.Fatalf("source sidecar changed after refused move: %v", err)
	}
	consent := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	got, err := s.Get("voice-sample")
	if err != nil {
		t.Fatal(err)
	}
	got.Consent = &consent
	if err = s.write(got); err != nil {
		t.Fatal(err)
	}
	if err = s.Move("voice-sample", Voice); err != nil {
		t.Fatal(err)
	}
	got, err = s.Get("voice-sample")
	if err != nil {
		t.Fatal(err)
	}
	if got.Purpose != Voice || got.Audio != "voice-sample.wav" {
		t.Fatalf("moved sample metadata: %+v", got)
	}
	if _, err = os.Stat(filepath.Join(s.root, "voice", got.Audio)); err != nil {
		t.Fatalf("audio did not follow sample: %v", err)
	}
	if _, err = os.Stat(filepath.Join(s.root, "dictation", "voice-sample.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source sidecar remains: %v", err)
	}
}

func TestMoveRollsBackWhenAudioRenameFails(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Add(Sample{ID: "blocked", Purpose: Noise}); err != nil {
		t.Fatal(err)
	}
	sourceAudio := filepath.Join(s.root, "noise", "blocked.wav")
	if err = os.WriteFile(sourceAudio, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	blockedTarget := filepath.Join(s.root, "dictation", "blocked.wav")
	if err = os.MkdirAll(filepath.Join(blockedTarget, "child"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err = s.Move("blocked", Dictation); err == nil {
		t.Fatal("move succeeded despite obstructed destination audio")
	}
	if _, err = os.Stat(sourceAudio); err != nil {
		t.Fatalf("source audio not restored: %v", err)
	}
	if _, err = os.Stat(filepath.Join(s.root, "noise", "blocked.json")); err != nil {
		t.Fatalf("source sidecar missing: %v", err)
	}
	if _, err = os.Stat(filepath.Join(s.root, "dictation", "blocked.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destination sidecar not rolled back: %v", err)
	}
}

func TestDeleteRemovesAudioAndSidecarAndUnknownGet(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get("missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Get unknown id error = %v", err)
	}
	if err = s.Add(Sample{ID: "remove-me", Purpose: Noise}); err != nil {
		t.Fatal(err)
	}
	audio := filepath.Join(s.root, "noise", "remove-me.flac")
	if err = os.WriteFile(audio, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = s.Delete("remove-me"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{audio, filepath.Join(s.root, "noise", "remove-me.json")} {
		if _, err = os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s remains after Delete: %v", path, err)
		}
	}
}

func TestMismatchedSidecarIDIsReported(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.root, "noise", "filename.json")
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte(`{"id":"different"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get("filename"); err == nil || !strings.Contains(err.Error(), "mismatched id") {
		t.Fatalf("Get mismatched sidecar error = %v", err)
	}
}

func TestCorruptSidecarDoesNotHideOtherSamples(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Add(Sample{ID: "valid", Purpose: Noise}); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(s.root, "noise", "broken.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	items, err := s.List(Noise)
	if err == nil || !strings.Contains(err.Error(), "broken.json") {
		t.Fatalf("List error = %v, want corrupt path", err)
	}
	if len(items) != 1 || items[0].ID != "valid" {
		t.Fatalf("valid items lost: %+v", items)
	}
}

func TestPublicPermissions(t *testing.T) {
	s, err := OpenPublic(filepath.Join(t.TempDir(), "public"))
	if err != nil {
		t.Fatal(err)
	}
	items := []Sample{{ID: "b", Purpose: Noise, Audio: "b.flac", Transcript: "a\tnoise", Keyterms: []string{"key", "term"}}, {ID: "a", Purpose: Dictation, Audio: "a.wav", Transcript: "speech"}}
	if err = s.Add(items[0]); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(s.root, "noise", "b.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("public sidecar mode %o", info.Mode().Perm())
	}
}

func TestNormalizeKeyterms(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []string
		want []string
	}{
		{"empty input", nil, nil},
		{"only blanks", []string{"", "  ", "\t"}, nil},
		{"trims and drops blanks", []string{"  kubectl ", "", "voxi"}, []string{"kubectl", "voxi"}},
		{"keeps first occurrence in order", []string{"b", "a", "b", " a ", "c"}, []string{"b", "a", "c"}},
		{"case sensitive match", []string{"Kube", "kube"}, []string{"Kube", "kube"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeKeyterms(tc.in)
			if strings.Join(got, "|") != strings.Join(tc.want, "|") || (got == nil) != (tc.want == nil) {
				t.Fatalf("NormalizeKeyterms(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestUpdateDetailsNormalizesAndWritesAtomically(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Add(Sample{ID: "jargon", Purpose: Dictation, Transcript: "old", Keyterms: []string{"stale"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateDetails("jargon", "  new transcript  ", []string{" kubectl ", "", "voxi", "kubectl"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("jargon")
	if err != nil {
		t.Fatal(err)
	}
	if got.Transcript != "new transcript" || strings.Join(got.Keyterms, "|") != "kubectl|voxi" {
		t.Fatalf("updated sample = %+v", got)
	}
	if got.Purpose != Dictation || got.Audio != "" {
		t.Fatalf("purpose or audio changed: %+v", got)
	}
	entries, err := os.ReadDir(filepath.Join(root, "dictation"))
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range entries {
		if strings.HasPrefix(ent.Name(), ".sample-") {
			t.Fatalf("temp sidecar left behind: %s", ent.Name())
		}
	}
	if err := s.UpdateDetails("jargon", "cleared", nil); err != nil {
		t.Fatal(err)
	}
	got, err = s.Get("jargon")
	if err != nil || got.Keyterms != nil {
		t.Fatalf("cleared keyterms = %+v, %v", got.Keyterms, err)
	}
}

func TestUpdateDetailsUnknownIDFails(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	err = s.UpdateDetails("missing", "text", []string{"term"})
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unknown id error = %v, want os.ErrNotExist", err)
	}
	if _, statErr := os.Stat(filepath.Join(s.Root(), "dictation", "missing.json")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("unknown id created a sidecar: %v", statErr)
	}
}
