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
		{".", false},
		{"!?", false},
		{" \t\u2026 ", false},
		{"\u2605", false},
		{"", false},
	}

	stopWords := testStopWords(t)
	for _, tc := range cases {
		if got := IsSafeToType(tc.text, stopWords); got != tc.safe {
			t.Errorf("IsSafeToType(%q) = %v, want %v", tc.text, got, tc.safe)
		}
	}
}

func TestHasRepeatedSentencePair(t *testing.T) {
	for _, text := range []string{
		"I'm going to the hospital. I'm going to the hospital.",
		"This is a complete sentence! This is a complete sentence!",
	} {
		if !HasRepeatedSentencePair(text) {
			t.Errorf("HasRepeatedSentencePair(%q) = false", text)
		}
	}
	for _, text := range []string{
		"Yes. Yes.",
		"very very good",
		"This is one sentence. This is different.",
	} {
		if HasRepeatedSentencePair(text) {
			t.Errorf("HasRepeatedSentencePair(%q) = true", text)
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

func TestStripLeadingHallucinations(t *testing.T) {
	stopWords := []string{"subs byuk"}

	got := StripLeadingHallucinations("Subs byuk, hello team, let's start the standup.", stopWords)
	want := "hello team, let's start the standup."
	if got != want {
		t.Fatalf("StripLeadingHallucinations() = %q, want %q", got, want)
	}

	// A stop-word-like substring occurring mid-sentence must not be
	// stripped -- only a true leading match should be removed.
	midSentence := "I was reading about subs byuk hallucinations yesterday."
	if got := StripLeadingHallucinations(midSentence, stopWords); got != midSentence {
		t.Fatalf("StripLeadingHallucinations() stripped a mid-sentence match: got %q, want unchanged %q", got, midSentence)
	}
}

func TestCleanWhisperTranscriptStripsLeadingHallucination(t *testing.T) {
	out := `
[2026-09-02T10:00:00Z INFO] Loading audio file: /tmp/utt_002.wav
[2026-09-02T10:00:01Z INFO] Model loaded in 0.2s
Transcription completed in 0.45s: "Subs byuk, hello team, let's start the standup."
`
	got := CleanWhisperTranscript(out, testStopWords(t))
	want := "hello team, let's start the standup."
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
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

func TestStripLeadingDashFragment(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			// Observed live: "-Transcribe." fused onto the real sentence.
			name:  "Transcribe artifact",
			input: "-Transcribe. I will now check if the transcribe immediately starts after I close the session.",
			want:  "I will now check if the transcribe immediately starts after I close the session.",
		},
		{
			// Observed in ring buffer chunk #186.
			name:  "H artifact",
			input: "-H. Also file a follow-up ticket that we need to handle this.",
			want:  "Also file a follow-up ticket that we need to handle this.",
		},
		{
			// Whole transcript is just the dash-fragment; no capitalized
			// continuation → no strip (IsSafeToType / caller decides fate).
			name:  "whole-transcript fragment not stripped",
			input: "-Trap.",
			want:  "-Trap.",
		},
		{
			// Legitimate hyphen-led list item: no capital-sentence continuation.
			name:  "legitimate hyphen-led list item not stripped",
			input: "- first item in list",
			want:  "- first item in list",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := StripLeadingDashFragment(tc.input)
			if got != tc.want {
				t.Errorf("StripLeadingDashFragment(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestCollapseRepeatedTrailingClause(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "repeated two-word clause after period",
			input: "Let's get started. get started",
			want:  "Let's get started.",
		},
		{
			name:  "mid-sentence repetition remains",
			input: "It was very very good",
			want:  "It was very very good",
		},
		{
			name:  "one-word suffix left as legitimate short answer",
			input: "Was your answer no? No",
			want:  "Was your answer no? No",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CollapseRepeatedTrailingClause(tc.input); got != tc.want {
				t.Errorf("CollapseRepeatedTrailingClause(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
