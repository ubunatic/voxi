package telemetry

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
)

type queryOptions struct {
	path, format, view, sessionID, chunkID, eventType string
	since, until                                      string
	limit, maxEvents                                  int
	success, silence, postClose                       string
}

// NewCommand builds the read-only telemetry analytics command family.
func NewCommand(out io.Writer, getenv func(string) string) *cobra.Command {
	defaultPath := func() string { return Path(getenv("XDG_DATA_HOME"), getenv("HOME")) }
	root := &cobra.Command{Use: "telemetry", Short: "Query correlated Eager pipeline telemetry"}
	var q queryOptions
	query := &cobra.Command{Use: "query", Short: "List raw events or correlated lifecycle records", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error { return runQuery(out, defaultPath(), q) }}
	addCommonFlags(query, &q)
	query.Flags().StringVar(&q.view, "view", "chunks", "view: events, chunks, or sessions")
	query.Flags().StringVar(&q.eventType, "event", "", "filter raw events by event type")
	query.Flags().StringVar(&q.success, "success", "any", "filter events/chunks: any, true, or false")
	query.Flags().StringVar(&q.silence, "silence", "any", "filter chunks: any, true, or false")
	query.Flags().StringVar(&q.postClose, "post-deactivation", "any", "filter chunks: any, true, or false")
	query.Flags().IntVar(&q.limit, "limit", 100, "maximum rows to print (0 means all within --max-events)")
	var s queryOptions
	stats := &cobra.Command{Use: "stats", Short: "Summarize latency, backlog, audio, words, and stage outcomes", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error { return runStats(out, defaultPath(), s) }}
	addCommonFlags(stats, &s)
	root.AddCommand(query, stats)
	return root
}

func addCommonFlags(cmd *cobra.Command, o *queryOptions) {
	cmd.Flags().StringVar(&o.path, "path", "", "telemetry JSONL path (default: Voxi XDG data path)")
	cmd.Flags().StringVar(&o.format, "format", "text", "output format: text or json")
	cmd.Flags().StringVar(&o.since, "since", "", "include timestamps at/after RFC3339 time")
	cmd.Flags().StringVar(&o.until, "until", "", "include timestamps at/before RFC3339 time")
	cmd.Flags().StringVar(&o.sessionID, "session", "", "filter by session ID")
	cmd.Flags().StringVar(&o.chunkID, "chunk", "", "filter by chunk ID")
	cmd.Flags().IntVar(&o.maxEvents, "max-events", 100000, "hard memory bound on valid events in the selected time range")
}

func parseTimeRange(sinceText, untilText string) (*time.Time, *time.Time, error) {
	parse := func(name, v string) (*time.Time, error) {
		if v == "" {
			return nil, nil
		}
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			return nil, fmt.Errorf("invalid --%s %q (want RFC3339 with timezone): %w", name, v, err)
		}
		return &t, nil
	}
	since, err := parse("since", sinceText)
	if err != nil {
		return nil, nil, err
	}
	until, err := parse("until", untilText)
	if err != nil {
		return nil, nil, err
	}
	if since != nil && until != nil && until.Before(*since) {
		return nil, nil, fmt.Errorf("--until must not precede --since")
	}
	return since, until, nil
}
func parseTri(name, value string) (*bool, error) {
	switch value {
	case "any":
		return nil, nil
	case "true":
		v := true
		return &v, nil
	case "false":
		v := false
		return &v, nil
	default:
		return nil, fmt.Errorf("invalid --%s %q (want any, true, or false)", name, value)
	}
}
func load(defaultPath string, o queryOptions) (ScanResult, error) {
	if o.maxEvents <= 0 {
		return ScanResult{}, fmt.Errorf("--max-events must be positive")
	}
	since, until, err := parseTimeRange(o.since, o.until)
	if err != nil {
		return ScanResult{}, err
	}
	if o.path == "" {
		o.path = defaultPath
	}
	return ScanRange(o.path, o.maxEvents, since, until)
}

type queryJSON struct {
	View            string    `json:"view"`
	MalformedRows   int       `json:"malformed_rows"`
	NewerSchemaRows int       `json:"newer_schema_rows"`
	Events          []Event   `json:"events"`
	Sessions        []Session `json:"sessions"`
	Chunks          []Chunk   `json:"chunks"`
}

func runQuery(out io.Writer, defaultPath string, o queryOptions) error {
	if o.format != "text" && o.format != "json" {
		return fmt.Errorf("invalid --format %q (want text or json)", o.format)
	}
	if o.view != "events" && o.eventType != "" {
		return fmt.Errorf("--event is only valid with --view=events")
	}
	success, err := parseTri("success", o.success)
	if err != nil {
		return err
	}
	silence, err := parseTri("silence", o.silence)
	if err != nil {
		return err
	}
	post, err := parseTri("post-deactivation", o.postClose)
	if err != nil {
		return err
	}
	scan, err := load(defaultPath, o)
	if err != nil {
		return err
	}
	eventType := ""
	eventSuccess := (*bool)(nil)
	if o.view == "events" {
		eventType, eventSuccess = o.eventType, success
	}
	events := filterEvents(scan.Events, o.sessionID, o.chunkID, eventType, eventSuccess)
	sessions, chunks := Correlate(events)
	filteredChunks := chunks[:0]
	for _, c := range chunks {
		if silence != nil && (c.Audio == nil || c.Audio.ProbableSilence != *silence) {
			continue
		}
		if post != nil && c.PostDeactivation != *post {
			continue
		}
		if success != nil && !chunkHasSuccess(c, *success) {
			continue
		}
		filteredChunks = append(filteredChunks, c)
	}
	chunks = filteredChunks
	if o.limit < 0 {
		return fmt.Errorf("--limit must not be negative")
	}
	limit := func(n int) int {
		if o.limit > 0 && n > o.limit {
			return o.limit
		}
		return n
	}
	result := queryJSON{View: o.view, MalformedRows: scan.MalformedRows, NewerSchemaRows: scan.NewerSchemaRows, Events: []Event{}, Sessions: []Session{}, Chunks: []Chunk{}}
	switch o.view {
	case "events":
		result.Events = events[:limit(len(events))]
	case "sessions":
		result.Sessions = sessions[:limit(len(sessions))]
	case "chunks":
		result.Chunks = chunks[:limit(len(chunks))]
	default:
		return fmt.Errorf("invalid --view %q (want events, chunks, or sessions)", o.view)
	}
	if o.format == "json" {
		return EncodeJSON(out, result)
	}
	fmt.Fprintf(out, "Telemetry %s: %d malformed row(s), %d newer-schema row(s) skipped\n", o.view, scan.MalformedRows, scan.NewerSchemaRows)
	switch o.view {
	case "events":
		for _, e := range result.Events {
			fmt.Fprintf(out, "%s  %-25s session=%s", e.Timestamp.Format(time.RFC3339Nano), e.Event, e.SessionID)
			if e.ChunkID != "" {
				fmt.Fprintf(out, " chunk=%s", e.ChunkID)
			}
			if e.Success != nil {
				fmt.Fprintf(out, " success=%t", *e.Success)
			}
			fmt.Fprintln(out)
		}
	case "sessions":
		for _, s := range result.Sessions {
			fmt.Fprintf(out, "session=%s chunks=%d activated=%s deactivated=%s\n", s.SessionID, s.ChunkCount, timeText(s.MicActivated), timeText(s.MicDeactivated))
		}
	case "chunks":
		for _, c := range result.Chunks {
			fmt.Fprintf(out, "session=%s chunk=%s queue=%s transcription=%s typing-delay=%s total=%s post-deactivation=%t silence=%s words=%s\n", c.SessionID, c.ChunkID, msText(c.QueueDelayMS), msText(c.TranscriptionMS), msText(c.TypingDelayMS), msText(c.TotalToTypeMS), c.PostDeactivation, boolTextAudio(c.Audio), intText(c.TranscriptWordCount))
		}
	}
	return nil
}

func filterEvents(events []Event, session, chunk, eventType string, success *bool) []Event {
	out := make([]Event, 0, len(events))
	for _, e := range events {
		if session != "" && e.SessionID != session || chunk != "" && e.ChunkID != chunk || eventType != "" && e.Event != eventType {
			continue
		}
		if success != nil && (e.Success == nil || *e.Success != *success) {
			continue
		}
		out = append(out, e)
	}
	return out
}
func chunkHasSuccess(c Chunk, want bool) bool {
	return c.TranscriptionSuccess != nil && *c.TranscriptionSuccess == want || c.TypingSuccess != nil && *c.TypingSuccess == want
}
func timeText(v *time.Time) string {
	if v == nil {
		return "-"
	}
	return v.Format(time.RFC3339Nano)
}
func msText(v *float64) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%.1fms", *v)
}
func boolTextAudio(a *AudioMetrics) string {
	if a == nil {
		return "-"
	}
	return fmt.Sprintf("%t", a.ProbableSilence)
}
func intText(v *int) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%d", *v)
}

func runStats(out io.Writer, defaultPath string, o queryOptions) error {
	if o.format != "text" && o.format != "json" {
		return fmt.Errorf("invalid --format %q (want text or json)", o.format)
	}
	scan, err := load(defaultPath, o)
	if err != nil {
		return err
	}
	events := filterEvents(scan.Events, o.sessionID, o.chunkID, "", nil)
	sessions, chunks := Correlate(events)
	stats := Aggregate(sessions, chunks, scan.MalformedRows, scan.NewerSchemaRows)
	if o.format == "json" {
		return EncodeJSON(out, stats)
	}
	fmt.Fprintf(out, "Telemetry: %d sessions, %d chunks (%d complete, %d partial)\n", stats.Sessions, stats.Chunks, stats.CompleteChunks, stats.PartialChunks)
	fmt.Fprintf(out, "Backlog: %d chunks processed after mic deactivation\n", stats.PostDeactivationChunks)
	fmt.Fprintf(out, "Audio: %.2fs, %d bytes, %d probable-silence chunks, %d transcript words", stats.AudioDurationSecs, stats.AudioBytes, stats.ProbableSilenceChunks, stats.TranscriptWords)
	if stats.ProbableSilenceRate != nil {
		fmt.Fprintf(out, " (silence rate %.1f%%)", *stats.ProbableSilenceRate*100)
	}
	if stats.TranscriptWordsPerAudioSec != nil {
		fmt.Fprintf(out, ", %.2f words/audio-second", *stats.TranscriptWordsPerAudioSec)
	}
	if stats.TranscriptionRTF != nil {
		fmt.Fprintf(out, ", transcription RTF %.3f", *stats.TranscriptionRTF)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Outcomes: transcription %d ok/%d failed; typing %d ok/%d failed\n", stats.TranscriptionSuccesses, stats.TranscriptionFailures, stats.TypingSuccesses, stats.TypingFailures)
	for _, row := range []struct {
		name string
		s    LatencySummary
	}{{"capture startup", stats.CaptureStartup}, {"capture shutdown", stats.CaptureShutdown}, {"queue", stats.QueueDelay}, {"transcription", stats.Transcription}, {"typing delay", stats.TypingDelay}, {"typing", stats.Typing}, {"total chunk-to-type", stats.TotalToType}} {
		fmt.Fprintf(out, "Latency %-19s n=%d avg=%.1fms p50=%.1fms p95=%.1fms max=%.1fms\n", row.name, row.s.Count, row.s.AverageMS, row.s.P50MS, row.s.P95MS, row.s.MaxMS)
	}
	fmt.Fprintf(out, "Input quality: %d malformed row(s), %d newer-schema row(s) skipped\n", stats.MalformedRows, stats.NewerSchemaRows)
	return nil
}
