// Package telemetry persists a correlated, append-only timeline of Eager
// microphone, recording, transcription, and typing events.
package telemetry

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

const SchemaVersion = 1

const (
	MicActivated          = "mic_activated"
	CaptureStarted        = "capture_started"
	MicDeactivated        = "mic_deactivated"
	CaptureStopped        = "capture_stopped"
	ChunkFinalized        = "chunk_finalized"
	TranscriptionStarted  = "transcription_started"
	TranscriptionComplete = "transcription_completed"
	TypingStarted         = "typing_started"
	TypingComplete        = "typing_completed"
	DeliveryDuplicate     = "delivery_duplicate"
	DeliveryStale         = "delivery_stale"
	StopDrainTimeout      = "stop_drain_timeout"
	InjectorStarted       = "injector_started"
	InjectorComplete      = "injector_completed"
)

// AudioMetrics contains recording-side measurements derived from PCM without
// consulting the transcription model.
type AudioMetrics struct {
	DurationSecs    float64 `json:"duration_secs"`
	PCMBytes        int     `json:"pcm_bytes"`
	MeanRMS         int     `json:"mean_rms"`
	PeakRMS         int     `json:"peak_rms"`
	VoicedRatio     float64 `json:"voiced_ratio"`
	ProbableSilence bool    `json:"probable_silence"`
}

// Event is one immutable row in the local Eager telemetry event database.
type Event struct {
	SchemaVersion       int           `json:"schema_version"`
	Event               string        `json:"event"`
	Timestamp           time.Time     `json:"timestamp"`
	SessionID           string        `json:"session_id"`
	ChunkID             string        `json:"chunk_id,omitempty"`
	ChunkIndex          int           `json:"chunk_index,omitempty"`
	Audio               *AudioMetrics `json:"audio,omitempty"`
	TranscriptWordCount *int          `json:"transcript_word_count,omitempty"`
	Success             *bool         `json:"success,omitempty"`
	Error               string        `json:"error,omitempty"`
	DeliveryID          string        `json:"delivery_id,omitempty"`
	InjectorPath        string        `json:"injector_path,omitempty"`
	ProcessID           int           `json:"process_id,omitempty"`
	Attempt             int           `json:"attempt,omitempty"`
	DurationMS          *float64      `json:"duration_ms,omitempty"`
	CancelReason        string        `json:"cancel_reason,omitempty"`
}

// Recorder serializes event appends from overlapping Eager session drains.
type Recorder struct {
	path string
	now  func() time.Time
	mu   sync.Mutex
	seq  atomic.Uint64
}

// Path returns the private local telemetry database path.
func Path(xdgDataHome, home string) string {
	if xdgDataHome != "" {
		return filepath.Join(xdgDataHome, "voxi", "eager-telemetry.jsonl")
	}
	return filepath.Join(home, ".local", "share", "voxi", "eager-telemetry.jsonl")
}

// NewRecorder creates a recorder for path using the system clock.
func NewRecorder(path string) *Recorder {
	return &Recorder{path: path, now: time.Now}
}

// NewSessionID returns a process-unique, time-sortable session correlation ID.
func (r *Recorder) NewSessionID(at time.Time) string {
	return fmt.Sprintf("%s-%06d", at.UTC().Format("20060102T150405.000000000Z"), r.seq.Add(1))
}

// Record appends one event. Callers intentionally treat errors as non-fatal:
// observability must never interrupt voice input.
func (r *Recorder) Record(event Event) error {
	if r == nil || r.path == "" {
		return nil
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = r.now()
	}
	event.SchemaVersion = SchemaVersion
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode telemetry event: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(r.path), 0700); err != nil {
		return fmt.Errorf("create telemetry directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(r.path), 0700); err != nil {
		return fmt.Errorf("protect telemetry directory: %w", err)
	}
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open telemetry database: %w", err)
	}
	defer f.Close()
	if err := f.Chmod(0600); err != nil {
		return fmt.Errorf("protect telemetry database: %w", err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("append telemetry event: %w", err)
	}
	return nil
}

// ReadAll decodes all valid events in a database. It is primarily useful for
// diagnostics and tests; a malformed row does not hide later valid events.
func ReadAll(path string) ([]Event, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open telemetry database: %w", err)
	}
	defer f.Close()
	var events []Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var event Event
		if json.Unmarshal(scanner.Bytes(), &event) == nil {
			events = append(events, event)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read telemetry database: %w", err)
	}
	return events, nil
}
