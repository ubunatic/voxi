package monitor

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/tts"
)

func TestPrintTTSPanelShowsCurrentQueueTimingAndControls(t *testing.T) {
	var out bytes.Buffer
	PrintTTSPanel(&out, tts.Snapshot{
		Status:           "playing",
		BackendStatus:    "Festival; player pw-play",
		Current:          "Now speaking this paragraph.",
		Queue:            []string{"Next paragraph."},
		TimeToFirstAudio: 1250 * time.Millisecond,
	})
	got := out.String()
	for _, want := range []string{"TTS · playing", "Backend: Festival; player pw-play", "Now speaking this paragraph.", "Queue: 1 chunk(s)", "Next paragraph.", "First audio: 1.25s", "[m]play/pause", "[b]previous", "[n]next", "[x]stop", "[k]clear"} {
		if !strings.Contains(got, want) {
			t.Errorf("panel missing %q in %q", want, got)
		}
	}
}

func TestPrintTTSPanelClearsEveryLineForShorterNextFrame(t *testing.T) {
	var out bytes.Buffer
	PrintTTSPanel(&out, tts.Snapshot{
		Status:  "synthesizing",
		Current: "A long paragraph remains on this line after the next frame.",
		Queue:   []string{"Another lengthy paragraph remains in the queue."},
	})
	PrintTTSPanel(&out, tts.Snapshot{Status: "idle"})

	frames := strings.Split(out.String(), "\n")
	for i, line := range frames {
		if strings.Contains(line, "TTS") || strings.Contains(line, "Backend:") || strings.Contains(line, "Now:") || strings.Contains(line, "Queue:") || strings.Contains(line, "[m]") {
			if !strings.Contains(line, "\x1b[K") {
				t.Errorf("TTS line %d lacks erase-to-end: %q", i+1, line)
			}
		}
	}
}
