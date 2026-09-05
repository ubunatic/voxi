package telemetry

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecorderPersistsCorrelatedEventsPrivately(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "events.jsonl")
	recorder := NewRecorder(path)
	at := time.Date(2026, 9, 5, 12, 34, 56, 789, time.UTC)
	sessionID := recorder.NewSessionID(at)
	words, success := 3, true
	events := []Event{
		{Event: MicActivated, Timestamp: at, SessionID: sessionID},
		{Event: ChunkFinalized, Timestamp: at.Add(time.Second), SessionID: sessionID, ChunkID: sessionID + "/1", ChunkIndex: 1, Audio: &AudioMetrics{DurationSecs: 1.5, PCMBytes: 48000, MeanRMS: 210, PeakRMS: 900, VoicedRatio: .4, ProbableSilence: false}},
		{Event: TranscriptionComplete, Timestamp: at.Add(2 * time.Second), SessionID: sessionID, ChunkID: sessionID + "/1", ChunkIndex: 1, TranscriptWordCount: &words, Success: &success},
	}
	for _, event := range events {
		if err := recorder.Record(event); err != nil {
			t.Fatalf("Record(%s): %v", event.Event, err)
		}
	}

	got, err := ReadAll(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(events) {
		t.Fatalf("got %d events, want %d", len(got), len(events))
	}
	if got[1].SessionID != sessionID || got[1].ChunkID != sessionID+"/1" {
		t.Fatalf("correlation lost: %+v", got[1])
	}
	if got[1].Audio == nil || got[1].Audio.PeakRMS != 900 || got[1].Audio.ProbableSilence {
		t.Fatalf("audio metrics lost: %+v", got[1].Audio)
	}
	if got[2].TranscriptWordCount == nil || *got[2].TranscriptWordCount != 3 || got[2].Success == nil || !*got[2].Success {
		t.Fatalf("completion metrics lost: %+v", got[2])
	}
	if got[0].SchemaVersion != SchemaVersion || !got[0].Timestamp.Equal(at) {
		t.Fatalf("schema/timestamp changed: %+v", got[0])
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if gotMode := info.Mode().Perm(); gotMode != 0600 {
		t.Fatalf("mode = %o, want 600", gotMode)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if gotMode := dirInfo.Mode().Perm(); gotMode != 0700 {
		t.Fatalf("directory mode = %o, want 700", gotMode)
	}
}

func TestSessionIDsAreUniqueAtSameTimestamp(t *testing.T) {
	recorder := NewRecorder("")
	at := time.Unix(100, 0)
	first := recorder.NewSessionID(at)
	second := recorder.NewSessionID(at)
	if first == second {
		t.Fatalf("duplicate session ID %q", first)
	}
}

func TestPathHonorsXDGDataHome(t *testing.T) {
	if got := Path("/data", "/home/me"); got != "/data/voxi/eager-telemetry.jsonl" {
		t.Fatalf("Path with XDG_DATA_HOME = %q", got)
	}
	if got := Path("", "/home/me"); got != "/home/me/.local/share/voxi/eager-telemetry.jsonl" {
		t.Fatalf("Path fallback = %q", got)
	}
}
