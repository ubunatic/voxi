package eager

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
)

func TestEagerAudioFailurePublisherTracksConsecutiveFailuresAndClears(t *testing.T) {
	runtimeDir := t.TempDir()
	publisher := newEagerAudioFailurePublisher(runtimeDir)
	statePath := EagerAudioFailureStatePath(runtimeDir)
	readState := func() EagerAudioFailureState {
		t.Helper()
		data, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatalf("read published failure state: %v", err)
		}
		var state EagerAudioFailureState
		if err := json.Unmarshal(data, &state); err != nil {
			t.Fatalf("decode published failure state: %v", err)
		}
		return state
	}

	publisher.Failure(errors.New("first chunk write failed"))
	if state := readState(); state.ConsecutiveFailures != 1 {
		t.Fatalf("after first failure, consecutive failures = %d, want 1", state.ConsecutiveFailures)
	}

	publisher.Failure(errors.New("second chunk write failed"))
	state := readState()
	if state.ConsecutiveFailures < EagerAudioFailureDegradedAfter {
		t.Fatalf("after second failure, consecutive failures = %d, want at least %d", state.ConsecutiveFailures, EagerAudioFailureDegradedAfter)
	}
	if state.LastError != "second chunk write failed" || state.LastFailureAt.IsZero() {
		t.Fatalf("published failure detail = %+v, want last error and timestamp", state)
	}

	publisher.Success()
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("failure state remains after next successful chunk: stat error = %v", err)
	}

	publisher.Failure(errors.New("failure after success"))
	if state := readState(); state.ConsecutiveFailures != 1 {
		t.Fatalf("after success then failure, consecutive failures = %d, want 1", state.ConsecutiveFailures)
	}
}
