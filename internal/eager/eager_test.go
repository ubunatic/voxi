package eager

import "testing"

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
