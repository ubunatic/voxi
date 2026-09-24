package monitor

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/tts"
)

func TestPrintTTSBoxShowsStateQueueAndControls(t *testing.T) {
	var out bytes.Buffer
	PrintTTSBox(&out, tts.Snapshot{
		Status:           "playing",
		BackendStatus:    "Festival; player pw-play",
		Current:          "Now speaking this paragraph.",
		Queue:            []string{"Next paragraph."},
		TimeToFirstAudio: 1250 * time.Millisecond,
	})
	got := out.String()
	for _, want := range []string{"TTS playback", "playing", "Festival; player pw-play", "Now speaking this paragraph.", "queue: 1 chunk(s)", "first audio: 1.25s", "[m]play/pause", "[b]previous", "[n]next", "[x]stop", "[k]clear"} {
		if !strings.Contains(got, want) {
			t.Errorf("panel missing %q in %q", want, got)
		}
	}
}

func TestPrintTTSBoxClearsEveryLineForShorterNextFrame(t *testing.T) {
	var out bytes.Buffer
	PrintTTSBox(&out, tts.Snapshot{
		Status:  "synthesizing",
		Current: "A long paragraph remains on this line after the next frame.",
		Queue:   []string{"Another lengthy paragraph remains in the queue."},
	})
	PrintTTSBox(&out, tts.Snapshot{Status: "idle"})

	frames := strings.Split(out.String(), "\n")
	for i, line := range frames {
		if strings.Contains(line, "TTS") || strings.Contains(line, "Backend:") || strings.Contains(line, "Now:") || strings.Contains(line, "Queue:") || strings.Contains(line, "[m]") {
			if !strings.Contains(line, "\x1b[K") {
				t.Errorf("TTS line %d lacks erase-to-end: %q", i+1, line)
			}
		}
	}
}

func TestUnifiedFeedShowsInputOutputAndOutcome(t *testing.T) {
	var out bytes.Buffer
	PrintUnifiedFeed(&out, []FeedEntry{
		{Direction: FeedInput, Text: "typed words", Timestamp: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
		{Direction: FeedOutput, Text: "spoken words", Status: "played", Timestamp: time.Date(2026, 1, 2, 3, 4, 6, 0, time.UTC)},
		{Direction: FeedOutput, Text: "next words", Status: "synthesizing", Timestamp: time.Date(2026, 1, 2, 3, 4, 7, 0, time.UTC)},
	}, 100)
	got := out.String()
	for _, want := range []string{"[IN]", "typed words", "[OUT]", "spoken words", "played", "next words", "synthesizing"} {
		if !strings.Contains(got, want) {
			t.Errorf("feed missing %q: %q", want, got)
		}
	}
	if strings.Index(got, "typed words") > strings.Index(got, "spoken words") || strings.Index(got, "spoken words") > strings.Index(got, "next words") {
		t.Fatalf("feed rows are not ordered: %q", got)
	}
}

func TestUnifiedFeedTruncatesLongTextToDisplayWidth(t *testing.T) {
	var out bytes.Buffer
	PrintUnifiedFeed(&out, []FeedEntry{{Direction: FeedOutput, Text: strings.Repeat("long ", 30), Status: "playing"}}, 30)
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if StringDisplayWidth(line) > 30 {
			t.Fatalf("feed line exceeds limit: %q", line)
		}
	}
	if !strings.Contains(out.String(), "...") {
		t.Fatalf("truncated row has no ellipsis: %q", out.String())
	}
}
