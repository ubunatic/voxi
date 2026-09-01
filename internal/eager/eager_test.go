package eager

import (
	"slices"
	"testing"
)

func TestAcceptTranscriptRejectsIsolatedSilenceArtifactBeforeTypingAndHistory(t *testing.T) {
	artifacts := []string{"bye"}
	if acceptTranscript(nil, "Bye!", nil, artifacts) {
		t.Fatal("isolated artifact reached the eager typing/history gate")
	}
	for _, text := range []string{"goodbye", "hello bye", "bye for now", "say goodbye"} {
		if !acceptTranscript(nil, text, nil, artifacts) {
			t.Errorf("longer genuine transcript %q did not reach eager typing/history gate", text)
		}
	}
}

func TestVoxtypeTranscribeArgsPreserveDisabledBehavior(t *testing.T) {
	got := voxtypeTranscribeArgs("small.en", "/tmp/one.wav", "")
	want := []string{"--model", "small.en", "--threads", "6", "-q", "transcribe", "/tmp/one.wav"}
	if !slices.Equal(got, want) {
		t.Fatalf("voxtypeTranscribeArgs() = %#v, want %#v", got, want)
	}
}

func TestVoxtypeTranscribeArgsPutInitialPromptBeforeSubcommand(t *testing.T) {
	got := voxtypeTranscribeArgs("small.en", "/tmp/one.wav", "Terms: Voxi")
	want := []string{"--model", "small.en", "--threads", "6", "--initial-prompt", "Terms: Voxi", "-q", "transcribe", "/tmp/one.wav"}
	if !slices.Equal(got, want) {
		t.Fatalf("voxtypeTranscribeArgs() = %#v, want %#v", got, want)
	}
}

func TestSpeechContextIsOptInAndSmallEnOnly(t *testing.T) {
	for _, test := range []struct {
		model   string
		enabled bool
		want    bool
	}{
		{model: "small.en", enabled: false, want: false},
		{model: "small.en", enabled: true, want: true},
		{model: "base.en", enabled: true, want: false},
		{model: "large-v3-turbo", enabled: true, want: false},
	} {
		if got := shouldUseSpeechContext(test.model, test.enabled); got != test.want {
			t.Errorf("shouldUseSpeechContext(%q, %t) = %t, want %t", test.model, test.enabled, got, test.want)
		}
	}
}
