package telemetry

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"time"
)

// ScanResult describes a bounded, read-only scan of a telemetry database.
type ScanResult struct {
	Events          []Event `json:"events"`
	MalformedRows   int     `json:"malformed_rows"`
	NewerSchemaRows int     `json:"newer_schema_rows"`
}

// Scan reads at most maxEvents valid rows. This hard bound prevents an
// unexpectedly large database from consuming unbounded memory.
func Scan(path string, maxEvents int) (ScanResult, error) {
	return ScanRange(path, maxEvents, nil, nil)
}

// ScanRange applies the time window while streaming, before the memory bound.
func ScanRange(path string, maxEvents int, since, until *time.Time) (ScanResult, error) {
	var result ScanResult
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("open telemetry database: %w", err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil || event.Event == "" || event.Timestamp.IsZero() || event.SessionID == "" {
			result.MalformedRows++
			continue
		}
		if event.SchemaVersion > SchemaVersion {
			result.NewerSchemaRows++
			continue
		}
		if since != nil && event.Timestamp.Before(*since) || until != nil && event.Timestamp.After(*until) {
			continue
		}
		if len(result.Events) >= maxEvents {
			return result, fmt.Errorf("telemetry scan exceeds --max-events=%d; narrow --since/--until or raise the bound", maxEvents)
		}
		result.Events = append(result.Events, event)
	}
	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("read telemetry database: %w", err)
	}
	sort.SliceStable(result.Events, func(i, j int) bool {
		if result.Events[i].Timestamp.Equal(result.Events[j].Timestamp) {
			if result.Events[i].SessionID == result.Events[j].SessionID {
				if result.Events[i].ChunkID == result.Events[j].ChunkID {
					return result.Events[i].Event < result.Events[j].Event
				}
				return result.Events[i].ChunkID < result.Events[j].ChunkID
			}
			return result.Events[i].SessionID < result.Events[j].SessionID
		}
		return result.Events[i].Timestamp.Before(result.Events[j].Timestamp)
	})
	return result, nil
}

// Session is a correlated microphone/capture lifecycle.
type Session struct {
	SessionID         string     `json:"session_id"`
	MicActivated      *time.Time `json:"mic_activated,omitempty"`
	CaptureStarted    *time.Time `json:"capture_started,omitempty"`
	MicDeactivated    *time.Time `json:"mic_deactivated,omitempty"`
	CaptureStopped    *time.Time `json:"capture_stopped,omitempty"`
	CaptureStartupMS  *float64   `json:"capture_startup_ms,omitempty"`
	CaptureShutdownMS *float64   `json:"capture_shutdown_ms,omitempty"`
	ChunkCount        int        `json:"chunk_count"`
}

// Chunk is a correlated pipeline lifecycle. Missing stages remain nil in JSON.
type Chunk struct {
	SessionID              string        `json:"session_id"`
	ChunkID                string        `json:"chunk_id"`
	ChunkIndex             int           `json:"chunk_index"`
	Finalized              *time.Time    `json:"finalized,omitempty"`
	TranscriptionStarted   *time.Time    `json:"transcription_started,omitempty"`
	TranscriptionCompleted *time.Time    `json:"transcription_completed,omitempty"`
	TypingStarted          *time.Time    `json:"typing_started,omitempty"`
	TypingCompleted        *time.Time    `json:"typing_completed,omitempty"`
	QueueDelayMS           *float64      `json:"queue_delay_ms,omitempty"`
	TranscriptionMS        *float64      `json:"transcription_ms,omitempty"`
	TypingDelayMS          *float64      `json:"typing_delay_ms,omitempty"`
	TypingMS               *float64      `json:"typing_ms,omitempty"`
	TotalToTypeMS          *float64      `json:"total_to_type_ms,omitempty"`
	PostDeactivation       bool          `json:"post_deactivation"`
	Audio                  *AudioMetrics `json:"audio,omitempty"`
	TranscriptWordCount    *int          `json:"transcript_word_count,omitempty"`
	TranscriptionSuccess   *bool         `json:"transcription_success,omitempty"`
	TypingSuccess          *bool         `json:"typing_success,omitempty"`
}

func durationMS(a, b *time.Time) *float64 {
	if a == nil || b == nil || b.Before(*a) {
		return nil
	}
	v := float64(b.Sub(*a)) / float64(time.Millisecond)
	return &v
}

// Correlate joins immutable events into deterministic session and chunk views.
func Correlate(events []Event) ([]Session, []Chunk) {
	sessions := map[string]*Session{}
	chunks := map[string]*Chunk{}
	for i := range events {
		e := events[i]
		s := sessions[e.SessionID]
		if s == nil {
			s = &Session{SessionID: e.SessionID}
			sessions[e.SessionID] = s
		}
		t := e.Timestamp
		switch e.Event {
		case MicActivated:
			if s.MicActivated == nil {
				s.MicActivated = &t
			}
		case CaptureStarted:
			if s.CaptureStarted == nil {
				s.CaptureStarted = &t
			}
		case MicDeactivated:
			if s.MicDeactivated == nil {
				s.MicDeactivated = &t
			}
		case CaptureStopped:
			if s.CaptureStopped == nil {
				s.CaptureStopped = &t
			}
		}
		if e.ChunkID == "" {
			continue
		}
		key := e.SessionID + "\x00" + e.ChunkID
		c := chunks[key]
		if c == nil {
			c = &Chunk{SessionID: e.SessionID, ChunkID: e.ChunkID, ChunkIndex: e.ChunkIndex}
			chunks[key] = c
			s.ChunkCount++
		}
		switch e.Event {
		case ChunkFinalized:
			if c.Finalized == nil {
				c.Finalized = &t
				c.Audio = e.Audio
			}
		case TranscriptionStarted:
			if c.TranscriptionStarted == nil {
				c.TranscriptionStarted = &t
			}
		case TranscriptionComplete:
			if c.TranscriptionCompleted == nil {
				c.TranscriptionCompleted = &t
				c.TranscriptWordCount = e.TranscriptWordCount
				c.TranscriptionSuccess = e.Success
			}
		case TypingStarted:
			if c.TypingStarted == nil {
				c.TypingStarted = &t
			}
		case TypingComplete:
			if c.TypingCompleted == nil {
				c.TypingCompleted = &t
				c.TypingSuccess = e.Success
			}
		}
	}
	ss := make([]Session, 0, len(sessions))
	for _, s := range sessions {
		s.CaptureStartupMS = durationMS(s.MicActivated, s.CaptureStarted)
		s.CaptureShutdownMS = durationMS(s.MicDeactivated, s.CaptureStopped)
		ss = append(ss, *s)
	}
	sort.Slice(ss, func(i, j int) bool {
		if ss[i].MicActivated != nil && ss[j].MicActivated != nil && !ss[i].MicActivated.Equal(*ss[j].MicActivated) {
			return ss[i].MicActivated.Before(*ss[j].MicActivated)
		}
		return ss[i].SessionID < ss[j].SessionID
	})
	cs := make([]Chunk, 0, len(chunks))
	for _, c := range chunks {
		c.QueueDelayMS = durationMS(c.Finalized, c.TranscriptionStarted)
		c.TranscriptionMS = durationMS(c.TranscriptionStarted, c.TranscriptionCompleted)
		c.TypingDelayMS = durationMS(c.TranscriptionCompleted, c.TypingStarted)
		c.TypingMS = durationMS(c.TypingStarted, c.TypingCompleted)
		c.TotalToTypeMS = durationMS(c.Finalized, c.TypingCompleted)
		if s := sessions[c.SessionID]; s != nil && s.MicDeactivated != nil {
			c.PostDeactivation = (c.TranscriptionStarted != nil && c.TranscriptionStarted.After(*s.MicDeactivated)) || (c.TranscriptionCompleted != nil && c.TranscriptionCompleted.After(*s.MicDeactivated)) || (c.TypingStarted != nil && c.TypingStarted.After(*s.MicDeactivated)) || (c.TypingCompleted != nil && c.TypingCompleted.After(*s.MicDeactivated))
		}
		cs = append(cs, *c)
	}
	sort.Slice(cs, func(i, j int) bool {
		if cs[i].Finalized != nil && cs[j].Finalized != nil && !cs[i].Finalized.Equal(*cs[j].Finalized) {
			return cs[i].Finalized.Before(*cs[j].Finalized)
		}
		if cs[i].SessionID != cs[j].SessionID {
			return cs[i].SessionID < cs[j].SessionID
		}
		return cs[i].ChunkID < cs[j].ChunkID
	})
	return ss, cs
}

// LatencySummary is a robust millisecond distribution.
type LatencySummary struct {
	Count     int     `json:"count"`
	AverageMS float64 `json:"average_ms"`
	P50MS     float64 `json:"p50_ms"`
	P95MS     float64 `json:"p95_ms"`
	MaxMS     float64 `json:"max_ms"`
}

// Stats is an aggregate report. Counts make partial data and skipped rows visible.
type Stats struct {
	Sessions                   int            `json:"sessions"`
	Chunks                     int            `json:"chunks"`
	CompleteChunks             int            `json:"complete_chunks"`
	PartialChunks              int            `json:"partial_chunks"`
	PostDeactivationChunks     int            `json:"post_deactivation_chunks"`
	ProbableSilenceChunks      int            `json:"probable_silence_chunks"`
	AudioDurationSecs          float64        `json:"audio_duration_secs"`
	AudioBytes                 int64          `json:"audio_bytes"`
	TranscriptWords            int            `json:"transcript_words"`
	TranscriptWordsPerAudioSec *float64       `json:"transcript_words_per_audio_sec,omitempty"`
	TranscriptionSuccesses     int            `json:"transcription_successes"`
	TranscriptionFailures      int            `json:"transcription_failures"`
	TypingSuccesses            int            `json:"typing_successes"`
	TypingFailures             int            `json:"typing_failures"`
	MalformedRows              int            `json:"malformed_rows"`
	NewerSchemaRows            int            `json:"newer_schema_rows"`
	TranscriptionRTF           *float64       `json:"transcription_rtf,omitempty"`
	ProbableSilenceRate        *float64       `json:"probable_silence_rate,omitempty"`
	CaptureStartup             LatencySummary `json:"capture_startup"`
	CaptureShutdown            LatencySummary `json:"capture_shutdown"`
	QueueDelay                 LatencySummary `json:"queue_delay"`
	Transcription              LatencySummary `json:"transcription"`
	TypingDelay                LatencySummary `json:"typing_delay"`
	Typing                     LatencySummary `json:"typing"`
	TotalToType                LatencySummary `json:"total_to_type"`
}

func summarize(values []*float64) LatencySummary {
	v := make([]float64, 0, len(values))
	for _, p := range values {
		if p != nil {
			v = append(v, *p)
		}
	}
	sort.Float64s(v)
	if len(v) == 0 {
		return LatencySummary{}
	}
	sum := 0.0
	for _, x := range v {
		sum += x
	}
	percentile := func(p float64) float64 { idx := int(float64(len(v)-1)*p + 0.5); return v[idx] }
	return LatencySummary{Count: len(v), AverageMS: sum / float64(len(v)), P50MS: percentile(.50), P95MS: percentile(.95), MaxMS: v[len(v)-1]}
}

// Aggregate calculates statistics from correlated records.
func Aggregate(sessions []Session, chunks []Chunk, malformed, newer int) Stats {
	st := Stats{Sessions: len(sessions), Chunks: len(chunks), MalformedRows: malformed, NewerSchemaRows: newer}
	var q, tr, td, ty, tot []*float64
	var captureStart, captureStop []*float64
	for i := range sessions {
		captureStart = append(captureStart, sessions[i].CaptureStartupMS)
		captureStop = append(captureStop, sessions[i].CaptureShutdownMS)
	}
	transSecs, audioForRTF := 0.0, 0.0
	for i := range chunks {
		c := &chunks[i]
		complete := c.Finalized != nil && c.TranscriptionStarted != nil && c.TranscriptionCompleted != nil && c.TypingStarted != nil && c.TypingCompleted != nil
		if complete {
			st.CompleteChunks++
		} else {
			st.PartialChunks++
		}
		if c.PostDeactivation {
			st.PostDeactivationChunks++
		}
		if c.Audio != nil {
			if c.Audio.ProbableSilence {
				st.ProbableSilenceChunks++
			}
			st.AudioDurationSecs += c.Audio.DurationSecs
			st.AudioBytes += int64(c.Audio.PCMBytes)
			if c.TranscriptionMS != nil {
				audioForRTF += c.Audio.DurationSecs
				transSecs += *c.TranscriptionMS / 1000
			}
		}
		if c.TranscriptWordCount != nil {
			st.TranscriptWords += *c.TranscriptWordCount
		}
		if c.TranscriptionSuccess != nil {
			if *c.TranscriptionSuccess {
				st.TranscriptionSuccesses++
			} else {
				st.TranscriptionFailures++
			}
		}
		if c.TypingSuccess != nil {
			if *c.TypingSuccess {
				st.TypingSuccesses++
			} else {
				st.TypingFailures++
			}
		}
		q = append(q, c.QueueDelayMS)
		tr = append(tr, c.TranscriptionMS)
		td = append(td, c.TypingDelayMS)
		ty = append(ty, c.TypingMS)
		tot = append(tot, c.TotalToTypeMS)
	}
	if audioForRTF > 0 {
		v := transSecs / audioForRTF
		st.TranscriptionRTF = &v
	}
	if st.AudioDurationSecs > 0 {
		v := float64(st.TranscriptWords) / st.AudioDurationSecs
		st.TranscriptWordsPerAudioSec = &v
	}
	if st.Chunks > 0 {
		v := float64(st.ProbableSilenceChunks) / float64(st.Chunks)
		st.ProbableSilenceRate = &v
	}
	st.CaptureStartup = summarize(captureStart)
	st.CaptureShutdown = summarize(captureStop)
	st.QueueDelay = summarize(q)
	st.Transcription = summarize(tr)
	st.TypingDelay = summarize(td)
	st.Typing = summarize(ty)
	st.TotalToType = summarize(tot)
	return st
}

// EncodeJSON emits stable, indented JSON and normalizes nil slices to arrays.
func EncodeJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}
