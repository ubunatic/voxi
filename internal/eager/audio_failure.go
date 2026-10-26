package eager

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// EagerAudioFailureDegradedAfter is the consecutive chunk-write failure count
// at which the monitor marks eager audio as degraded.
const EagerAudioFailureDegradedAfter = 2

// EagerAudioFailureState is the minimal runtime snapshot of recent WAV write failures.
type EagerAudioFailureState struct {
	ConsecutiveFailures int       `json:"consecutive_failures"`
	LastError           string    `json:"last_error"`
	LastFailureAt       time.Time `json:"last_failure_at"`
}

// EagerAudioFailureStatePath returns the runtime path shared by the eager daemon and monitor.
func EagerAudioFailureStatePath(runtimeDir string) string {
	return filepath.Join(runtimeDir, "voxi", "eager-audio-failures.json")
}

type eagerAudioFailurePublisher struct {
	mu          sync.Mutex
	path        string
	consecutive int
}

func newEagerAudioFailurePublisher(runtimeDir string) *eagerAudioFailurePublisher {
	return &eagerAudioFailurePublisher{path: EagerAudioFailureStatePath(runtimeDir)}
}

func (p *eagerAudioFailurePublisher) Failure(err error) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	p.consecutive++
	state := EagerAudioFailureState{
		ConsecutiveFailures: p.consecutive,
		LastFailureAt:       time.Now().UTC(),
	}
	if err != nil {
		state.LastError = err.Error()
	}
	data, marshalErr := json.Marshal(state)
	if marshalErr != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p.path), 0700); err != nil {
		return
	}
	_ = os.WriteFile(p.path, data, 0600)
}

func (p *eagerAudioFailurePublisher) Success() {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.consecutive = 0
	_ = os.Remove(p.path)
	p.mu.Unlock()
}
