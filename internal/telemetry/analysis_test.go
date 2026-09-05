package telemetry

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testTime(ms int) time.Time {
	return time.Date(2026, 9, 5, 10, 0, 0, ms*int(time.Millisecond), time.UTC)
}
func boolp(v bool) *bool { return &v }
func intp(v int) *int    { return &v }

func lifecycleFixture() []Event {
	audio := &AudioMetrics{DurationSecs: 2, PCMBytes: 64000, MeanRMS: 80, PeakRMS: 200, VoicedRatio: .05, ProbableSilence: true}
	return []Event{
		{SchemaVersion: 1, Event: MicActivated, Timestamp: testTime(0), SessionID: "s1"},
		{SchemaVersion: 1, Event: CaptureStarted, Timestamp: testTime(10), SessionID: "s1"},
		{SchemaVersion: 1, Event: ChunkFinalized, Timestamp: testTime(100), SessionID: "s1", ChunkID: "c1", ChunkIndex: 1, Audio: audio},
		{SchemaVersion: 1, Event: TranscriptionStarted, Timestamp: testTime(150), SessionID: "s1", ChunkID: "c1", ChunkIndex: 1},
		{SchemaVersion: 1, Event: MicDeactivated, Timestamp: testTime(200), SessionID: "s1"},
		{SchemaVersion: 1, Event: CaptureStopped, Timestamp: testTime(220), SessionID: "s1"},
		{SchemaVersion: 1, Event: TranscriptionComplete, Timestamp: testTime(450), SessionID: "s1", ChunkID: "c1", ChunkIndex: 1, Success: boolp(true), TranscriptWordCount: intp(4)},
		{SchemaVersion: 1, Event: TypingStarted, Timestamp: testTime(500), SessionID: "s1", ChunkID: "c1", ChunkIndex: 1},
		{SchemaVersion: 1, Event: TypingComplete, Timestamp: testTime(530), SessionID: "s1", ChunkID: "c1", ChunkIndex: 1, Success: boolp(true)},
		// A partial failed chunk proves absent stages do not fabricate durations.
		{SchemaVersion: 1, Event: ChunkFinalized, Timestamp: testTime(600), SessionID: "s1", ChunkID: "c2", ChunkIndex: 2, Audio: &AudioMetrics{DurationSecs: 1, PCMBytes: 32000}},
		{SchemaVersion: 1, Event: TranscriptionStarted, Timestamp: testTime(700), SessionID: "s1", ChunkID: "c2", ChunkIndex: 2},
		{SchemaVersion: 1, Event: TranscriptionComplete, Timestamp: testTime(900), SessionID: "s1", ChunkID: "c2", ChunkIndex: 2, Success: boolp(false)},
	}
}

func TestCorrelateDerivedDurationsAndPartialLifecycle(t *testing.T) {
	sessions, chunks := Correlate(lifecycleFixture())
	if len(sessions) != 1 || sessions[0].ChunkCount != 2 || *sessions[0].CaptureStartupMS != 10 || *sessions[0].CaptureShutdownMS != 20 {
		t.Fatalf("session correlation = %+v", sessions)
	}
	if len(chunks) != 2 {
		t.Fatalf("chunks = %d", len(chunks))
	}
	c := chunks[0]
	if *c.QueueDelayMS != 50 || *c.TranscriptionMS != 300 || *c.TypingDelayMS != 50 || *c.TypingMS != 30 || *c.TotalToTypeMS != 430 {
		t.Fatalf("derived chunk durations = %+v", c)
	}
	if !c.PostDeactivation || c.Audio == nil || !c.Audio.ProbableSilence || *c.TranscriptWordCount != 4 {
		t.Fatalf("enriched chunk = %+v", c)
	}
	if chunks[1].TypingStarted != nil || chunks[1].TotalToTypeMS != nil || chunks[1].TypingSuccess != nil {
		t.Fatalf("partial chunk fabricated stage: %+v", chunks[1])
	}
}

func TestAggregateExactCountsPercentilesAndRTF(t *testing.T) {
	sessions, chunks := Correlate(lifecycleFixture())
	stats := Aggregate(sessions, chunks, 2, 1)
	if stats.Chunks != 2 || stats.CompleteChunks != 1 || stats.PartialChunks != 1 || stats.PostDeactivationChunks != 2 || stats.ProbableSilenceChunks != 1 {
		t.Fatalf("counts = %+v", stats)
	}
	if stats.AudioDurationSecs != 3 || stats.AudioBytes != 96000 || stats.TranscriptWords != 4 || stats.TranscriptionSuccesses != 1 || stats.TranscriptionFailures != 1 || stats.TypingSuccesses != 1 {
		t.Fatalf("metrics = %+v", stats)
	}
	if stats.QueueDelay.Count != 2 || stats.QueueDelay.P50MS != 100 || stats.QueueDelay.P95MS != 100 || stats.QueueDelay.AverageMS != 75 {
		t.Fatalf("queue summary = %+v", stats.QueueDelay)
	}
	if stats.TranscriptionRTF == nil || math.Abs(*stats.TranscriptionRTF-(0.5/3)) > 1e-12 {
		t.Fatalf("RTF = %v", stats.TranscriptionRTF)
	}
	if stats.ProbableSilenceRate == nil || *stats.ProbableSilenceRate != .5 || stats.TranscriptWordsPerAudioSec == nil || math.Abs(*stats.TranscriptWordsPerAudioSec-(4.0/3)) > 1e-12 {
		t.Fatalf("rates = %+v", stats)
	}
	if stats.CaptureStartup.P50MS != 10 || stats.CaptureShutdown.P50MS != 20 {
		t.Fatalf("capture latency = %+v / %+v", stats.CaptureStartup, stats.CaptureShutdown)
	}
}

func TestScanRangeOrderingMalformedNewerAndBound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	rows := []string{`not-json`, `{"schema_version":2,"event":"future","timestamp":"2026-09-05T10:00:00Z","session_id":"s"}`}
	for _, e := range []Event{lifecycleFixture()[2], lifecycleFixture()[0]} {
		b, _ := json.Marshal(e)
		rows = append(rows, string(b))
	}
	if err := os.WriteFile(path, []byte(strings.Join(rows, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	since := testTime(0)
	until := testTime(100)
	got, err := ScanRange(path, 2, &since, &until)
	if err != nil {
		t.Fatal(err)
	}
	if got.MalformedRows != 1 || got.NewerSchemaRows != 1 || len(got.Events) != 2 || got.Events[0].Event != MicActivated || got.Events[1].Event != ChunkFinalized {
		t.Fatalf("scan = %+v", got)
	}
	if _, err := ScanRange(path, 1, &since, &until); err == nil || !strings.Contains(err.Error(), "exceeds --max-events=1") {
		t.Fatalf("bound error = %v", err)
	}
}

func TestQueryJSONFiltersAndPreservesAbsentStages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	var data bytes.Buffer
	for _, e := range lifecycleFixture() {
		b, _ := json.Marshal(e)
		data.Write(b)
		data.WriteByte('\n')
	}
	data.WriteString("bad\n")
	if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	o := queryOptions{path: path, format: "json", view: "chunks", sessionID: "s1", chunkID: "c2", success: "false", silence: "any", postClose: "any", limit: 100, maxEvents: 100}
	if err := runQuery(&out, "", o); err != nil {
		t.Fatal(err)
	}
	var got struct {
		MalformedRows int              `json:"malformed_rows"`
		Chunks        []map[string]any `json:"chunks"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.MalformedRows != 1 || len(got.Chunks) != 1 || got.Chunks[0]["chunk_id"] != "c2" {
		t.Fatalf("query JSON = %s", out.String())
	}
	if _, exists := got.Chunks[0]["typing_started"]; exists {
		t.Fatalf("absent typing stage should be omitted: %s", out.String())
	}
}

func TestParseTimeRangeRequiresTimezone(t *testing.T) {
	if _, _, err := parseTimeRange("2026-09-05T10:00:00", ""); err == nil {
		t.Fatal("timezone-free time accepted")
	}
	if _, _, err := parseTimeRange("2026-09-05T11:00:00Z", "2026-09-05T10:00:00Z"); err == nil {
		t.Fatal("reversed range accepted")
	}
}
