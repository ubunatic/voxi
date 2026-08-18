package asr

import (
	"testing"
)

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
		{"", false},
	}

	for _, tc := range cases {
		if got := IsSafeToType(tc.text); got != tc.safe {
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
	got := CleanWhisperTranscript(out)
	if got != "Testing the voice input extraction." {
		t.Fatalf("expected 'Testing the voice input extraction.', got %q", got)
	}
}
