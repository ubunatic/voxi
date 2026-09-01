package asr

import (
	"testing"

	spec "ubunatic.com/voxi/spec"
)

func testStopWords(t *testing.T) []string {
	t.Helper()
	s, err := spec.LoadModels()
	if err != nil {
		t.Fatalf("spec.LoadModels() error = %v", err)
	}
	return s.StopWords(s.DefaultModel)
}

func TestStripANSI(t *testing.T) {
	colored := "\x1b[32mhello\x1b[0m \x1b[1;31mworld\x1b[0m"
	if got := StripANSI(colored); got != "hello world" {
		t.Fatalf("expected 'hello world', got %q", got)
	}
}

func TestIsSafeToType(t *testing.T) {
	cases := []struct {
		text string
		safe bool
	}{
		{"Hello world", true},
		{"2026-08-18T12:00:00 [INFO] log line", false},
		{"INFO processing audio", false},
		{"whisper_init_state: kv self size", false},
		{"https://example.com/video", false},
		{"thank you for watching", false},
		{"Please subscribe to my channel.", false},
		{"Kathryn", false},
		{"kathryn.", false},
		{"", false},
	}

	stopWords := testStopWords(t)
	for _, tc := range cases {
		if got := IsSafeToType(tc.text, stopWords); got != tc.safe {
			t.Errorf("IsSafeToType(%q) = %v, want %v", tc.text, got, tc.safe)
		}
	}
}

func TestCleanWhisperTranscript(t *testing.T) {
	out := `
[2026-08-18T10:00:00Z INFO] Loading audio file: /tmp/utt_001.wav
[2026-08-18T10:00:01Z INFO] Model loaded in 0.2s
Transcription completed in 0.45s: "Testing the voice input extraction."
`
	got := CleanWhisperTranscript(out, testStopWords(t))
	if got != "Testing the voice input extraction." {
		t.Fatalf("expected 'Testing the voice input extraction.', got %q", got)
	}
}

func TestUserStopWordLiteralBoundaries(t *testing.T) {
	stopWords := []string{`(?:^|[\s\p{P}])bye(?:$|[\s\p{P}])`, `(?:^|[\s\p{P}])A\.\+\(x\)(?:$|[\s\p{P}])`}
	if IsSafeToType("bye", stopWords) {
		t.Fatal("whole user stop word was accepted")
	}
	if !IsSafeToType("goodbye", stopWords) {
		t.Fatal("substring was incorrectly rejected")
	}
	if got := StripTrailingHallucinations("hello bye", stopWords); got != "hello" {
		t.Fatalf("trailing word = %q", got)
	}
	if got := StripTrailingHallucinations("hello A.+(x)", stopWords); got != "hello" {
		t.Fatalf("literal punctuation phrase = %q", got)
	}
	if !IsSafeToType("hello ax", stopWords) {
		t.Fatal("regex-shaped phrase was interpreted as regex")
	}
}
