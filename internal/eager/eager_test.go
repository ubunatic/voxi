package eager

import (
	"fmt"
	"slices"
	"strings"
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

func TestSpeechContextIsSmallEnOnlyAndDisableable(t *testing.T) {
	for _, test := range []struct {
		model   string
		enabled bool
		want    bool
	}{
		// enabled=false models an explicit --speech-context=false: prompting
		// must be fully disabled regardless of model (issue 046).
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

// TestDefaultEagerOptionsEnableSpeechContext locks in issue 046's flipped
// default: a fresh `voxi eager` invocation with no --speech-context flag
// must have prompting active (small.en is also the default model, so the
// shouldUseSpeechContext gate above will fire). Explicit
// --speech-context=false still overrides this via the Cobra flag binding
// in cmd/voxi/main.go, which is exercised by TestSpeechContextIsSmallEnOnlyAndDisableable
// above (the enabled=false cases).
func TestDefaultEagerOptionsEnableSpeechContext(t *testing.T) {
	opts := DefaultEagerOptions()
	if !opts.SpeechContext {
		t.Fatal("DefaultEagerOptions().SpeechContext = false, want true (issue 046: default-on)")
	}
	if !shouldUseSpeechContext(opts.Model, opts.SpeechContext) {
		t.Fatalf("shouldUseSpeechContext(%q, true) = false, want true for the default model", opts.Model)
	}
}

func TestRejectionReason(t *testing.T) {
	artifacts := []string{"bye"}
	stopWords := []string{"thank you"}

	if r := rejectionReason(fmt.Errorf("fail"), "", "", nil, nil); !strings.HasPrefix(r, "transcribe_error") {
		t.Errorf("expected transcribe_error, got %q", r)
	}
	if r := rejectionReason(nil, "", "", nil, nil); r != "empty" {
		t.Errorf("expected empty, got %q", r)
	}
	if r := rejectionReason(nil, "bye.", "bye", nil, artifacts); r != "silence_artifact" {
		t.Errorf("expected silence_artifact, got %q", r)
	}
	if r := rejectionReason(nil, "Thank you.", "Thank you.", stopWords, nil); r != "stop_word" {
		t.Errorf("expected stop_word, got %q", r)
	}
	if r := rejectionReason(nil, "Hello world", "Hello world", stopWords, artifacts); r != "" {
		t.Errorf("expected empty reason for valid text, got %q", r)
	}
}
