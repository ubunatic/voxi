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
		Current:          "Now speaking this paragraph.",
		Queue:            []string{"Next paragraph."},
		TimeToFirstAudio: 1250 * time.Millisecond,
	})
	got := out.String()
	for _, want := range []string{"TTS · playing", "Now speaking this paragraph.", "Queue: 1 chunk(s)", "Next paragraph.", "First audio: 1.25s", "[m]play/pause", "[b]previous", "[n]next", "[x]stop", "[k]clear"} {
		if !strings.Contains(got, want) {
			t.Errorf("panel missing %q in %q", want, got)
		}
	}
}
