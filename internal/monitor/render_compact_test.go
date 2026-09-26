package monitor

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"ubunatic.com/voxi/internal/chunks"
	"ubunatic.com/voxi/internal/eager"
	"ubunatic.com/voxi/internal/tts"
)

func TestPrintCompactTwoBoxMatchesLayout(t *testing.T) {
	var out bytes.Buffer

	report := VoiceResourceReport{
		ActiveModel:    "r2t2-confucius4",
		RecordStatus:   "recording",
		MicLevel:       46.0,
		ModifierStatus: "L-Ctrl",
		ASRBackend: ASRBackendState{
			Applicable: true,
			Online:     true,
			Endpoint:   "127.0.0.1:18131",
		},
		ServiceMemory:  6764573491, // ~6.3 GB
		VRAMUsedBytes:  3006477107, // ~2.8 GB
		Processes: []ProcessResource{
			{Name: "agent"},
			{Name: "r2t2"},
			{Name: "dotoold"},
		},
		EagerMetrics: &eager.EagerMetrics{
			LastUtterance: &eager.UtteranceStat{
				RTF:            0.45, // 1/0.45 = 2.2x speed
				TranscribeSecs: 1.30,
			},
		},
	}

	recentChunks := []chunks.Chunk{
		{
			Index:             53,
			Timestamp:         time.Date(2026, 9, 26, 11, 51, 11, 0, time.Local),
			AudioDurationSecs: 0.4,
			Accepted:          false,
			RejectionReason:   "low_energy",
		},
		{
			Index:             54,
			Timestamp:         time.Date(2026, 9, 26, 11, 51, 14, 0, time.Local),
			AudioDurationSecs: 2.9,
			Accepted:          true,
			CleanedTranscript: "Chunk one...",
		},
	}

	ttsSnap := tts.Snapshot{
		History: []tts.ChunkRecord{
			{
				ID:        1,
				Text:      "Super+Y \"Voxi standalone\"",
				Status:    "1.4s",
				Timestamp: time.Date(2026, 9, 26, 11, 53, 10, 0, time.Local),
			},
			{
				ID:        2,
				Text:      "Dictating continuous eager",
				Status:    "0.8s",
				Timestamp: time.Date(2026, 9, 26, 11, 53, 45, 0, time.Local),
			},
		},
	}

	PrintCompactTwoBox(&out, report, recentChunks, ttsSnap, 105)
	got := out.String()

	// Verify required sections and keywords
	for _, want := range []string{
		"Voxi Monitor",
		"r2t2-confucius4",
		"Live Voice Stream",
		"Dictation In/Out & Health",
		"REC",
		"L-Ctrl active",
		"RTF 2.2x",
		"1.30s lag",
		"chunks timeline",
		"#53",
		"#54",
		"low_energy",
		"Chunk one...",
		":18131",
		"online",
		"ram 6.3G",
		"vram 2.8G",
		"agent",
		"r2t2",
		"dotoold",
		"[TTS]",
		"[OUT]",
		"Voxi standalone",
		"[c]ompact",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("compact view missing expected element %q; full output:\n%s", want, got)
		}
	}

	// Verify box lines formatting (all rendered box lines must have equal display width)
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) < 8 {
		t.Fatalf("expected at least 8 lines, got %d", len(lines))
	}

	for i, line := range lines {
		w := StringDisplayWidth(line)
		if strings.HasPrefix(line, "┌") || strings.HasPrefix(line, "│") || strings.HasPrefix(line, "└") {
			if w != 105 {
				t.Errorf("line %d has unexpected display width %d (expected 105): %q", i+1, w, line)
			}
		}
	}
}
